package proxy

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 65536)
		return &b
	},
}

type RelayResult struct {
	BytesIn  int64
	BytesOut int64
	Err      error
}

func Relay(
	ctx context.Context,
	client, server net.Conn,
	idleTimeout time.Duration,
	onBytesIn, onBytesOut func(int64),
) RelayResult {
	var (
		wg       sync.WaitGroup
		bytesIn  int64
		bytesOut int64
		firstErr error
		errMu    sync.Mutex
	)

	setErr := func(err error) {
		errMu.Lock()
		if firstErr == nil && err != nil && err != io.EOF {
			firstErr = err
		}
		errMu.Unlock()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-ctx.Done()
		client.Close()
		server.Close()
	}()

	wg.Add(2)

	go func() {
		defer wg.Done()
		n, err := copyLoop(server, client, idleTimeout, onBytesIn)
		bytesIn = n
		setErr(err)
		cancel()
	}()

	go func() {
		defer wg.Done()
		n, err := copyLoop(client, server, idleTimeout, onBytesOut)
		bytesOut = n
		setErr(err)
		cancel()
	}()

	wg.Wait()
	return RelayResult{BytesIn: bytesIn, BytesOut: bytesOut, Err: firstErr}
}

func copyLoop(dst, src net.Conn, idleTimeout time.Duration, onBytes func(int64)) (int64, error) {
	bufPtr := bufPool.Get().(*[]byte)
	buf := *bufPtr
	defer func() {
		for i := range buf {
			buf[i] = 0
		}
		bufPool.Put(bufPtr)
	}()

	var total int64
	for {
		if idleTimeout > 0 {
			if err := src.SetReadDeadline(time.Now().Add(idleTimeout)); err != nil {
				return total, err
			}
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			if idleTimeout > 0 {
				dst.SetWriteDeadline(time.Now().Add(idleTimeout))
			}
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)
			if onBytes != nil {
				onBytes(int64(written))
			}
			if writeErr != nil {
				return total, writeErr
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return total, nil
			}
			if ne, ok := readErr.(net.Error); ok && ne.Timeout() {
				return total, nil
			}
			return total, readErr
		}
	}
}

func RelayWithSNI(
	ctx context.Context,
	client, server net.Conn,
	idleTimeout time.Duration,
	maxSNIBuf int,
	onBytesIn, onBytesOut func(int64),
) (sni string, result RelayResult) {
	sni, accumulated := readSNI(client, idleTimeout, maxSNIBuf)

	if len(accumulated) > 0 {
		if idleTimeout > 0 {
			server.SetWriteDeadline(time.Now().Add(idleTimeout))
		}
		if _, err := server.Write(accumulated); err != nil {
			result.Err = err
			return sni, result
		}
		if onBytesIn != nil {
			onBytesIn(int64(len(accumulated)))
		}
	}

	result = Relay(ctx, client, server, idleTimeout, onBytesIn, onBytesOut)
	return sni, result
}

func readSNI(conn net.Conn, idleTimeout time.Duration, maxBuf int) (string, []byte) {
	if maxBuf <= 0 {
		maxBuf = 16384
	}

	acc := make([]byte, 0, 1024)
	tmp := make([]byte, 4096)

	for len(acc) < maxBuf {
		if idleTimeout > 0 {
			conn.SetReadDeadline(time.Now().Add(idleTimeout))
		}
		n, err := conn.Read(tmp)
		if n > 0 {
			acc = append(acc, tmp[:n]...)
			if sni, done := parseSNI(acc); done {
				return sni, acc
			}
		}
		if err != nil {
			break
		}
	}
	return "", acc
}

func parseSNI(data []byte) (string, bool) {
	if len(data) < 5 {
		return "", false
	}
	if data[0] != 0x16 {
		return "", true
	}

	recLen := int(data[3])<<8 | int(data[4])
	if recLen > 16384 || len(data) < 5+recLen {
		return "", len(data) >= 5+recLen
	}

	payload := data[5 : 5+recLen]
	if len(payload) < 4 || payload[0] != 0x01 {
		return "", true
	}

	hsLen := int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	if hsLen > 16384 || len(payload) < 4+hsLen {
		return "", len(payload) >= 4+hsLen
	}

	hello := payload[4 : 4+hsLen]
	if len(hello) < 38 {
		return "", true
	}

	pos := 34
	if pos >= len(hello) {
		return "", true
	}
	sessLen := int(hello[pos])
	pos++
	if pos+sessLen > len(hello) {
		return "", true
	}
	pos += sessLen

	if pos+2 > len(hello) {
		return "", true
	}
	csLen := int(hello[pos])<<8 | int(hello[pos+1])
	pos += 2
	if pos+csLen > len(hello) {
		return "", true
	}
	pos += csLen

	if pos >= len(hello) {
		return "", true
	}
	compLen := int(hello[pos])
	pos++
	if pos+compLen > len(hello) {
		return "", true
	}
	pos += compLen

	if pos+2 > len(hello) {
		return "", true
	}
	extTotalLen := int(hello[pos])<<8 | int(hello[pos+1])
	pos += 2
	if pos+extTotalLen > len(hello) {
		return "", true
	}

	ext := hello[pos : pos+extTotalLen]
	for len(ext) >= 4 {
		et := int(ext[0])<<8 | int(ext[1])
		el := int(ext[2])<<8 | int(ext[3])
		ext = ext[4:]
		if el > len(ext) {
			break
		}
		body := ext[:el]
		ext = ext[el:]

		if et != 0x0000 {
			continue
		}
		if len(body) < 5 {
			return "", true
		}
		nameLen := int(body[3])<<8 | int(body[4])
		if 5+nameLen > len(body) {
			return "", true
		}
		return string(body[5 : 5+nameLen]), true
	}
	return "", true
}
