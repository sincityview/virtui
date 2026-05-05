// virtui/internal/libvirt/client.go
package libvirt

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	lv "libvirt.org/go/libvirt"
)

type Client struct {
	conn     *lv.Connect
	IPv4Only bool
}

func NewClient() (*Client, error) {
	conn, err := lv.NewConnect("qemu:///system")
	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к libvirt: %w", err)
	}
	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	if c.conn != nil {
		_, err := c.conn.Close()
		return err
	}
	return nil
}

type DomainInfo struct {
	Name      string
	Status    string
	UUID      string
	CPU       uint64
	Memory    uint64
	MaxMemory uint64
	VCPUs     uint
	Disks     []string
	IPs       []string
}

func (c *Client) ListDomains() ([]DomainInfo, error) {
	doms, err := c.conn.ListAllDomains(0)
	if err != nil {
		return nil, err
	}

	var domains []DomainInfo
	for _, d := range doms {
		name, _ := d.GetName()
		uuid, _ := d.GetUUIDString()

		state, _, _ := d.GetState()
		status := "Unknown"
		switch state {
		case lv.DOMAIN_RUNNING:
			status = "Running"
		case lv.DOMAIN_SHUTOFF:
			status = "Shutoff"
		case lv.DOMAIN_PAUSED:
			status = "Paused"
		case lv.DOMAIN_SHUTDOWN:
			status = "Shutting down"
		case lv.DOMAIN_CRASHED:
			status = "Crashed"
		}

		info, _ := d.GetInfo()

		xml, _ := d.GetXMLDesc(0)
		var disks []string
		targetRe := regexp.MustCompile(`<target dev=['"]([^'"]+)['"]`)
		sourceRe := regexp.MustCompile(`<source file=['"]([^'"]+)['"]`)
		diskParts := strings.Split(xml, "<disk")
		for _, part := range diskParts[1:] {
			block := "<disk" + strings.Split(part, "</disk>")[0]

			target := targetRe.FindStringSubmatch(block)
			if len(target) < 2 {
				continue
			}
			dev := target[1]

			entry := dev
			source := sourceRe.FindStringSubmatch(block)
			if len(source) > 1 {
				entry = fmt.Sprintf("%s [%s]", dev, source[1])
			}
			disks = append(disks, entry)
		}

		var ips []string
		if status == "Running" {
			ifaces, err := d.ListAllInterfaceAddresses(lv.DOMAIN_INTERFACE_ADDRESSES_SRC_AGENT)
			if err == nil {
				for _, iface := range ifaces {
					for _, addr := range iface.Addrs {
						if addr.Addr == "127.0.0.1" || addr.Addr == "::1" {
							continue
						}
						if addr.Type == lv.IP_ADDR_TYPE_IPV4 || (!c.IPv4Only && addr.Type == lv.IP_ADDR_TYPE_IPV6) {
							ips = append(ips, addr.Addr)
						}
					}
				}
			}
		}
		sort.Strings(ips)

		domains = append(domains, DomainInfo{
			Name:      name,
			Status:    status,
			UUID:      uuid,
			CPU:       info.CpuTime,
			Memory:    info.Memory,
			MaxMemory: info.MaxMem,
			VCPUs:     info.NrVirtCpu,
			Disks:     disks,
			IPs:       ips,
		})
		d.Free()
	}
	return domains, nil
}

func (c *Client) Start(name string) error {
	dom, err := c.conn.LookupDomainByName(name)
	if err != nil {
		return err
	}
	defer dom.Free()
	return dom.Create()
}

func (c *Client) Shutdown(name string) error {
	dom, err := c.conn.LookupDomainByName(name)
	if err != nil {
		return err
	}
	defer dom.Free()
	return dom.Shutdown()
}

func (c *Client) Reboot(name string) error {
	dom, err := c.conn.LookupDomainByName(name)
	if err != nil {
		return err
	}
	defer dom.Free()
	return dom.Reboot(lv.DOMAIN_REBOOT_DEFAULT)
}

func (c *Client) Destroy(name string) error {
	dom, err := c.conn.LookupDomainByName(name)
	if err != nil {
		return err
	}
	defer dom.Free()
	return dom.Destroy()
}

func (c *Client) GetXML(name string) (string, error) {
	dom, err := c.conn.LookupDomainByName(name)
	if err != nil {
		return "", err
	}
	defer dom.Free()
	return dom.GetXMLDesc(0)
}

func (c *Client) DefineXML(xml string) error {
	dom, err := c.conn.DomainDefineXML(xml)
	if err != nil {
		return err
	}
	defer dom.Free()
	return nil
}

type diskInfo struct {
	srcPath string
	format  string
}

func (c *Client) RemoveDomain(name string) error {
	dom, err := c.conn.LookupDomainByName(name)
	if err != nil {
		return fmt.Errorf("domain %s not found: %w", name, err)
	}
	defer dom.Free()

	xml, err := dom.GetXMLDesc(0)
	if err != nil {
		return fmt.Errorf("failed to get XML: %w", err)
	}

	re := regexp.MustCompile(`<source file='([^']+)'`)
	matches := re.FindAllStringSubmatch(xml, -1)
	var diskPaths []string
	for _, m := range matches {
		if len(m) > 1 {
			diskPaths = append(diskPaths, m[1])
		}
	}

	if err := dom.Destroy(); err != nil {
		// already shutoff — ignore
	}

	if err := dom.Undefine(); err != nil {
		return fmt.Errorf("failed to undefine domain: %w", err)
	}

	for _, path := range diskPaths {
		c.removeStorageVol(path)
	}

	return nil
}

func (c *Client) CloneDomain(name, cloneName string) error {
	srcDom, err := c.conn.LookupDomainByName(name)
	if err != nil {
		return fmt.Errorf("domain %s not found: %w", name, err)
	}
	defer srcDom.Free()

	state, _, _ := srcDom.GetState()
	if state != lv.DOMAIN_SHUTOFF {
		return fmt.Errorf("domain %s must be shutoff to clone", name)
	}

	exists, err := c.DomainExists(cloneName)
	if err != nil {
		return fmt.Errorf("failed to check domain existence: %w", err)
	}
	if exists {
		return fmt.Errorf("domain %s already exists", cloneName)
	}

	xml, err := srcDom.GetXMLDesc(0)
	if err != nil {
		return fmt.Errorf("failed to get XML: %w", err)
	}

	cloneXML, disks, err := prepareCloneXML(xml, name, cloneName)
	if err != nil {
		return err
	}

	var clonedVols []string
	for _, d := range disks {
		clonePath := cloneDiskPath(d.srcPath, name, cloneName)
		if err := c.cloneStorageVol(d.srcPath, clonePath); err != nil {
			for _, p := range clonedVols {
				c.removeStorageVol(p)
			}
			return fmt.Errorf("failed to clone disk %s: %w", d.srcPath, err)
		}
		clonedVols = append(clonedVols, clonePath)
	}

	if err := c.DefineXML(cloneXML); err != nil {
		for _, p := range clonedVols {
			c.removeStorageVol(p)
		}
		return fmt.Errorf("failed to define clone domain: %w", err)
	}

	return nil
}

func prepareCloneXML(xml, name, cloneName string) (string, []diskInfo, error) {
	s := xml

	s = regexp.MustCompile(`<name>[^<]+</name>`).ReplaceAllString(s, fmt.Sprintf("<name>%s</name>", cloneName))
	s = regexp.MustCompile(`\s*<uuid>[^<]+</uuid>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s*<mac address='[^']*'\s*/>`).ReplaceAllString(s, "")

	re := regexp.MustCompile(`<source file='([^']+)'`)
	matches := re.FindAllStringSubmatch(s, -1)
	disks := make([]diskInfo, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			disks = append(disks, diskInfo{srcPath: m[1]})
		}
	}

	for _, d := range disks {
		clonePath := cloneDiskPath(d.srcPath, name, cloneName)
		s = strings.ReplaceAll(s, d.srcPath, clonePath)
	}

	return s, disks, nil
}

func cloneDiskPath(srcPath, name, cloneName string) string {
	dir, file := filepath.Split(srcPath)
	newFile := strings.Replace(file, name, cloneName, 1)
	if newFile == file {
		ext := filepath.Ext(file)
		base := file[:len(file)-len(ext)]
		newFile = base + "-" + cloneName + ext
	}
	return filepath.Join(dir, newFile)
}

func (c *Client) cloneStorageVol(srcPath, clonePath string) error {
	cloneFileName := filepath.Base(clonePath)

	srcVol, err := c.conn.LookupStorageVolByPath(srcPath)
	if err != nil {
		return fmt.Errorf("source volume not found: %w", err)
	}
	defer srcVol.Free()

	srcXML, err := srcVol.GetXMLDesc(0)
	if err == nil {
		formatMatch := regexp.MustCompile(`<format type='([^']+)'`).FindStringSubmatch(srcXML)
		if len(formatMatch) > 1 {
			return c.cloneVolWithFormat(srcVol, cloneFileName, formatMatch[1])
		}
	}

	return c.cloneVolWithFormat(srcVol, cloneFileName, "raw")
}

func (c *Client) cloneVolWithFormat(srcVol *lv.StorageVol, name, format string) error {
	pool, err := srcVol.LookupPoolByVolume()
	if err != nil {
		return fmt.Errorf("failed to find pool for volume: %w", err)
	}
	defer pool.Free()

	volXML := fmt.Sprintf(`<volume><name>%s</name><target><format type='%s'/></target></volume>`, name, format)
	_, err = pool.StorageVolCreateXMLFrom(volXML, srcVol, 0)
	return err
}

func (c *Client) removeStorageVol(path string) {
	vol, err := c.conn.LookupStorageVolByPath(path)
	if err != nil {
		return
	}
	defer vol.Free()
	vol.Delete(lv.STORAGE_VOL_DELETE_NORMAL)
}

func (c *Client) DomainExists(name string) (bool, error) {
	dom, err := c.conn.LookupDomainByName(name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	dom.Free()
	return true, nil
}
