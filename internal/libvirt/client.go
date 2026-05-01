// virtui/internal/libvirt/client.go
package libvirt

import (
	"fmt"
	"regexp"
	"strings"

	lv "libvirt.org/go/libvirt"
)

type Client struct {
	conn *lv.Connect
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
		diskMatches := regexp.MustCompile(`<target dev=['"]([^'"]+)['"]`).FindAllStringSubmatch(xml, -1)
		for _, m := range diskMatches {
			if len(m) > 1 {
				dev := m[1]
				if !strings.HasPrefix(dev, "vnet") {
					disks = append(disks, dev)
				}
			}
		}

		var ips []string
		if status == "Running" {
			ifaces, err := d.ListAllInterfaceAddresses(lv.DOMAIN_INTERFACE_ADDRESSES_SRC_AGENT)
			if err == nil {
				for _, iface := range ifaces {
					for _, addr := range iface.Addrs {
						if addr.Type == lv.IP_ADDR_TYPE_IPV4 || addr.Type == lv.IP_ADDR_TYPE_IPV6 {
							ips = append(ips, addr.Addr)
						}
					}
				}
			}
		}

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
