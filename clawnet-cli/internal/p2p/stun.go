package p2p

import (
	"fmt"
	"net"
	"time"

	"github.com/pion/stun"
)

// STUN servers to try in order.
var stunServers = []string{
	"stun.l.google.com:19302",
	"stun.cloudflare.com:3478",
	"stun.stunprotocol.org:3478",
}

// DetectExternalIP uses STUN to discover the node's external (public) IP address.
// Returns the IP string or empty string if detection fails.
func DetectExternalIP() string {
	for _, server := range stunServers {
		ip, err := stunQuery(server)
		if err == nil && ip != "" {
			return ip
		}
	}
	return ""
}

func stunQuery(server string) (string, error) {
	conn, err := net.DialTimeout("udp", server, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	c, err := stun.NewClient(conn)
	if err != nil {
		return "", err
	}
	defer c.Close()

	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	var externalIP string
	err = c.Do(msg, func(res stun.Event) {
		if res.Error != nil {
			return
		}
		var xorAddr stun.XORMappedAddress
		if err := xorAddr.GetFrom(res.Message); err == nil {
			externalIP = xorAddr.IP.String()
			return
		}
		var mappedAddr stun.MappedAddress
		if err := mappedAddr.GetFrom(res.Message); err == nil {
			externalIP = mappedAddr.IP.String()
		}
	})
	if err != nil {
		return "", fmt.Errorf("stun %s: %w", server, err)
	}
	return externalIP, nil
}
