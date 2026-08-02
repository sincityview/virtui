### virtui - tui for libvirt

------

Install:
* sudo apt install -y libvirt-dev pkg-config gcc
* go mod tidy
* go build -o virtui cmd/tui/main.go
* sudo cp ./virtui /usr/local/bin/vtui
* sudo usermod -aG libvirt,libvirt-qemu,kvm $USER
* echo 'export LIBVIRT_DEFAULT_URI="qemu:///system"' >> ~/.bashrc
* relogin (or newgrp libvirt)
* vtui

Config is created automatically on first run at ~/.local/virtui/config:

```json
{
  "max_log_lines": 50,
  "ipv4_only": false,
  "libvirt_dir": "/var/lib/libvirt/"
}
```

Modify as needed, then restart vtui.

<br>

Hotkeys:
* jk - up / down for machines
* Shift +
  * s - start unactive machine
  * p - stop active machine
  * r - restart active machine
  * e - edit xml of unactive machine
  * c - connect to active machine
  * k - clone unactive machine with custom name [new uuid, mac]
  * d - destroy and undefine unactive machine
  * q - exit programm

<br>

![virtui](./img/virtui.png)
