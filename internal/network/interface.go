package network

import (
	"fmt"
	"net"
)

func LocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("local IP: %w", err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

func InterfaceForIP(ip string) (*net.Interface, error) {
	target := net.ParseIP(ip)
	if target == nil {
		return nil, fmt.Errorf("invalid IP: %s", ip)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ifIP net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ifIP = v.IP
			case *net.IPAddr:
				ifIP = v.IP
			}
			if ifIP != nil && ifIP.Equal(target) {
				return &iface, nil
			}
		}
	}
	return nil, fmt.Errorf("no interface for IP %s", ip)
}

func SetSocketBuffers(conn net.Conn, size int) {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetReadBuffer(size)
		tc.SetWriteBuffer(size)
	}
}
