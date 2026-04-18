//go:build !linux

package proxy

import (
	"fmt"
	"net"
	"time"
)

func getTCPSeqNums(conn net.Conn) (synSeq, synAckSeq uint32, synTime time.Time, err error) {
	synTime = time.Now()
	return pseudoISN(conn), 0, synTime, fmt.Errorf("raw socket bypass not supported on this platform")
}

func pseudoISN(conn net.Conn) uint32 {
	h := uint32(2166136261)
	for _, c := range conn.LocalAddr().String() + conn.RemoteAddr().String() {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}
