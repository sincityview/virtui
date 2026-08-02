// virtui/internal/libvirt/client.go
package libvirt

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	lv "libvirt.org/go/libvirt"
	"virtui/internal/config"
)

type Client struct {
	conn       *lv.Connect
	IPv4Only   bool
	LibvirtDir string
}

func NewClient(cfg *config.Config) (*Client, error) {
	conn, err := lv.NewConnect(cfg.URI)
	if err != nil {
		return nil, fmt.Errorf("cant connect to libvirt: %w", err)
	}
	return &Client{
		conn:       conn, 
		IPv4Only:   cfg.IPv4Only,
		LibvirtDir: cfg.LibvirtDir,
	}, nil
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
	Warnings  []string
}

type domainXML struct {
	XMLName xml.Name   `xml:"domain"`
	Disks   []diskXML  `xml:"disk"`
}

type diskXML struct {
	Target struct {
		Dev string `xml:"dev,attr"`
	} `xml:"target"`
	Source struct {
		File string `xml:"file,attr"`
	} `xml:"source"`
}

type storageVolXML struct {
	XMLName  xml.Name `xml:"volume"`
	Capacity struct {
		Text string `xml:",chardata"`
		Unit string `xml:"unit,attr"`
	} `xml:"capacity"`
	Target struct {
		Format struct {
			Type string `xml:"type,attr"`
		} `xml:"format"`
	} `xml:"target"`
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

		xmlDesc, err := d.GetXMLDesc(0)
		var disks []string
		var warnings []string
		if err != nil {
			warnings = append(warnings, "failed to get XML: "+err.Error())
		} else {
			var dx domainXML
			if err := xml.Unmarshal([]byte(xmlDesc), &dx); err != nil {
				warnings = append(warnings, "failed to parse XML: "+err.Error())
			} else {
				for _, disk := range dx.Disks {
					if disk.Target.Dev == "" {
						continue
					}
					if disk.Source.File != "" {
						disks = append(disks, fmt.Sprintf("%s [%s]", disk.Target.Dev, disk.Source.File))
					} else {
						disks = append(disks, disk.Target.Dev)
					}
				}
			}
		}

		var ips []string
		if status == "Running" {
			ifaces, err := d.ListAllInterfaceAddresses(lv.DOMAIN_INTERFACE_ADDRESSES_SRC_AGENT)
			if err != nil {
				warnings = append(warnings, "failed to get IPs: "+err.Error())
			} else {
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
			Warnings:  warnings,
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

	xmlDesc, err := dom.GetXMLDesc(0)
	if err != nil {
		return fmt.Errorf("failed to get XML: %w", err)
	}

	var dx domainXML
	if err := xml.Unmarshal([]byte(xmlDesc), &dx); err != nil {
		return fmt.Errorf("failed to parse XML: %w", err)
	}

	var diskPaths []string
	for _, disk := range dx.Disks {
		if disk.Source.File != "" {
			path := strings.ReplaceAll(disk.Source.File, "/var/lib/libvirt/", c.LibvirtDir)
			diskPaths = append(diskPaths, path)
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

	cloneXML, disks, err := c.prepareCloneXML(xml, name, cloneName)
	if err != nil {
		return err
	}

	var clonedVols []string
	for _, d := range disks {
		srcPath := strings.ReplaceAll(d.srcPath, "/var/lib/libvirt/", c.LibvirtDir)
		clonePath := cloneDiskPath(srcPath, name, cloneName)
		if err := c.cloneStorageVol(srcPath, clonePath); err != nil {
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

func (c *Client) prepareCloneXML(xmlDesc, name, cloneName string) (string, []diskInfo, error) {
	var dx domainXML
	if err := xml.Unmarshal([]byte(xmlDesc), &dx); err != nil {
		return "", nil, fmt.Errorf("failed to parse source XML: %w", err)
	}

	disks := make([]diskInfo, 0, len(dx.Disks))
	for _, d := range dx.Disks {
		if d.Source.File != "" {
			disks = append(disks, diskInfo{srcPath: d.Source.File})
		}
	}

	s := xmlDesc

	s = strings.ReplaceAll(s, "/var/lib/libvirt/", c.LibvirtDir)

	s = regexp.MustCompile(`<name>[^<]+</name>`).ReplaceAllString(s, fmt.Sprintf("<name>%s</name>", cloneName))
	s = regexp.MustCompile(`\s*<uuid>[^<]+</uuid>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s*<mac address='[^']*'\s*/>`).ReplaceAllString(s, "")

	for _, d := range disks {
		newSrcPath := strings.ReplaceAll(d.srcPath, "/var/lib/libvirt/", c.LibvirtDir)
		clonePath := cloneDiskPath(newSrcPath, name, cloneName)
		s = strings.ReplaceAll(s, newSrcPath, clonePath)
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
		return fmt.Errorf("source volume not found at '%s': %w", srcPath, err)
	}
	defer srcVol.Free()

	srcXML, err := srcVol.GetXMLDesc(0)
	var format string = "raw"
	if err == nil {
		formatMatch := regexp.MustCompile(`<format type='([^']+)'`).FindStringSubmatch(srcXML)
		if len(formatMatch) > 1 {
			format = formatMatch[1]
		}
	}

	return c.cloneVolWithFormat(srcVol, cloneFileName, format)
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
