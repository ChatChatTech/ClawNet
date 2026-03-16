//go:build linux

package overlay

// Linux TUN device for ClawNet overlay IPv6 traffic.
// Creates a system TUN interface, assigns the overlay IPv6 address,
// and routes 200::/7 through it.

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	tunDevPath = "/dev/net/tun"
	tunSetIFF  = 0x400454ca // _IOW('T', 202, int)
	iffTUN     = 0x0001
	iffNoPi    = 0x1000
	ifNameSize = 16
)

type ifReq struct {
	Name  [ifNameSize]byte
	Flags uint16
	_     [22]byte
}

// TUNDevice manages a Linux TUN interface for overlay IPv6 traffic.
type TUNDevice struct {
	file *os.File
	name string
	addr net.IP
	mtu  int
}

// NewTUNDevice creates and configures a TUN device with the given IPv6 address.
// Requires root or CAP_NET_ADMIN.
func NewTUNDevice(name string, addr net.IP, mtu int) (*TUNDevice, error) {
	fd, err := syscall.Open(tunDevPath, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", tunDevPath, err)
	}

	var req ifReq
	copy(req.Name[:], name)
	req.Flags = iffTUN | iffNoPi

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), tunSetIFF, uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("ioctl TUNSETIFF: %v", errno)
	}

	file := os.NewFile(uintptr(fd), tunDevPath)
	dev := &TUNDevice{
		file: file,
		name: name,
		addr: addr,
		mtu:  mtu,
	}

	if err := dev.configure(); err != nil {
		file.Close()
		return nil, fmt.Errorf("configure %s: %w", name, err)
	}

	return dev, nil
}

func (d *TUNDevice) configure() error {
	// Set MTU
	if out, err := exec.Command("ip", "link", "set", d.name, "mtu", fmt.Sprintf("%d", d.mtu)).CombinedOutput(); err != nil {
		return fmt.Errorf("set mtu: %s: %w", string(out), err)
	}
	// Bring interface up
	if out, err := exec.Command("ip", "link", "set", d.name, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("link up: %s: %w", string(out), err)
	}
	// Add IPv6 address with /7 prefix (covers entire 200::/7 overlay address space)
	if out, err := exec.Command("ip", "-6", "addr", "add", d.addr.String()+"/7", "dev", d.name).CombinedOutput(); err != nil {
		return fmt.Errorf("add addr: %s: %w", string(out), err)
	}
	return nil
}

// Read reads a raw IPv6 packet from the TUN device.
func (d *TUNDevice) Read(p []byte) (int, error) { return d.file.Read(p) }

// Write writes a raw IPv6 packet to the TUN device.
func (d *TUNDevice) Write(p []byte) (int, error) { return d.file.Write(p) }

// Close tears down the TUN device.
func (d *TUNDevice) Close() error {
	_ = exec.Command("ip", "link", "set", d.name, "down").Run()
	return d.file.Close()
}

// Name returns the TUN interface name.
func (d *TUNDevice) Name() string { return d.name }
