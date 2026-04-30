package libvirt

import (
	"fmt"

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
	OS        string
	CPU       uint64
	Memory    uint64
	MaxMemory uint64
	VCPUs     uint
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

		// OS type
		osType, _ := d.GetOSType()

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

		domains = append(domains, DomainInfo{
			Name:      name,
			Status:    status,
			UUID:      uuid,
			OS:        osType,
			CPU:       info.CpuTime,
			Memory:    info.Memory,
			MaxMemory: info.MaxMem,
			VCPUs:     info.NrVirtCpu,
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