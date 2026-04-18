//go:build linux

package proxy

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func getTCPSeqNums(conn net.Conn) (synSeq, synAckSeq uint32, synTime time.Time, err error) {
	synTime = time.Now()
	local, ok1 := conn.LocalAddr().(*net.TCPAddr)
	remote, ok2 := conn.RemoteAddr().(*net.TCPAddr)
	if !ok1 || !ok2 {
		return pseudoISN(conn), 0, synTime, fmt.Errorf("not a TCPConn")
	}
	tx, rx, readErr := procNetTCP(local, remote)
	if readErr != nil {
		return pseudoISN(conn), 0, synTime, readErr
	}
	return tx, rx, synTime, nil
}

func procNetTCP(local, remote *net.TCPAddr) (tx, rx uint32, err error) {
	data, err := os.ReadFile("/proc/net/tcp")
	if err != nil {
		return 0, 0, fmt.Errorf("proc/net/tcp: %w", err)
	}
	lh := addrHex(local)
	rh := addrHex(remote)
	for _, line := range strings.Split(string(data), "\n")[1:] {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		if f[1] == lh && f[2] == rh {
			tx, rx = parseQ(f[4])
			return tx, rx, nil
		}
	}
	return 0, 0, fmt.Errorf("proc/net/tcp: not found (%s->%s)", lh, rh)
}

func addrHex(a *net.TCPAddr) string {
	ip4 := a.IP.To4()
	if ip4 == nil {
		return ""
	}
	return fmt.Sprintf("%02X%02X%02X%02X:%04X", ip4[3], ip4[2], ip4[1], ip4[0], a.Port)
}

func parseQ(s string) (tx, rx uint32) {
	p := strings.SplitN(s, ":", 2)
	if len(p) != 2 {
		return
	}
	fmt.Sscanf(p[0], "%X", &tx)
	fmt.Sscanf(p[1], "%X", &rx)
	return
}

func pseudoISN(conn net.Conn) uint32 {
	h := uint32(2166136261)
	for _, c := range conn.LocalAddr().String() + conn.RemoteAddr().String() {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}
