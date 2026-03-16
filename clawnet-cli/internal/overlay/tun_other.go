//go:build !linux

package overlay

import (
	"errors"
	"net"
)

// TUNDevice stub for non-Linux platforms.
type TUNDevice struct{}

func NewTUNDevice(name string, addr net.IP, mtu int) (*TUNDevice, error) {
	return nil, errors.New("TUN not supported on this platform")
}

func (d *TUNDevice) Read(p []byte) (int, error)  { return 0, errors.New("unsupported") }
func (d *TUNDevice) Write(p []byte) (int, error) { return 0, errors.New("unsupported") }
func (d *TUNDevice) Close() error                { return nil }
func (d *TUNDevice) Name() string                { return "" }
