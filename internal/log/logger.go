package log

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Level int32

const (
	LevelDebug   Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelStealth
)

const (
	cReset   = "\033[0m"
	cGray    = "\033[90m"
	cCyan    = "\033[36m"
	cYellow  = "\033[33m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cBlue    = "\033[34m"
	cMagenta = "\033[35m"
	cWhite   = "\033[97m"
)

type Logger struct {
	level   atomic.Int32
	writers []io.Writer
	closers []io.Closer
	noColor bool
	connID  string

	ch   chan string
	done chan struct{}
	once sync.Once
}

func ParseLevel(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "stealth":
		return LevelStealth
	default:
		return LevelInfo
	}
}

func New(level Level, filePath string) (*Logger, error) {
	writers := []io.Writer{os.Stdout}
	var closers []io.Closer

	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("log: open %q: %w", filePath, err)
		}
		writers = append(writers, f)
		closers = append(closers, f)
	}

	l := &Logger{
		writers: writers,
		closers: closers,
		ch:      make(chan string, 4096),
		done:    make(chan struct{}),
	}
	l.level.Store(int32(level))
	go l.writeLoop()
	return l, nil
}

func (l *Logger) writeLoop() {
	defer close(l.done)
	for line := range l.ch {
		for _, w := range l.writers {
			w.Write([]byte(line))
		}
	}
	for _, c := range l.closers {
		c.Close()
	}
}

func (l *Logger) Close() {
	l.once.Do(func() {
		close(l.ch)
		<-l.done
	})
}

func (l *Logger) WithConnID(id string) *Logger {
	child := &Logger{
		writers: l.writers,
		closers: nil,
		noColor: l.noColor,
		connID:  id,
		ch:      l.ch,
		done:    l.done,
	}
	child.level.Store(l.level.Load())
	return child
}

func (l *Logger) SetLevel(v Level)  { l.level.Store(int32(v)) }
func (l *Logger) GetLevel() Level   { return Level(l.level.Load()) }
func (l *Logger) SetNoColor(v bool) { l.noColor = v }

func (l *Logger) Debug(f string, a ...interface{}) { l.emit(LevelDebug, f, a...) }
func (l *Logger) Info(f string, a ...interface{})  { l.emit(LevelInfo, f, a...) }
func (l *Logger) Warn(f string, a ...interface{})  { l.emit(LevelWarn, f, a...) }
func (l *Logger) Error(f string, a ...interface{}) { l.emit(LevelError, f, a...) }

func (l *Logger) Fatal(f string, a ...interface{}) {
	l.emit(LevelError, f, a...)
	l.Close()
	os.Exit(1)
}

func (l *Logger) ConnOpen(connID, remote string, active, total int64) {
	l.emit(LevelInfo, "%s▶ OPEN%s    id=%-14s remote=%-24s active=%-5d total=%d",
		cGreen, cReset, connID, remote, active, total)
}

func (l *Logger) ConnClose(connID, remote string, in, out int64, dur time.Duration) {
	l.emit(LevelInfo, "%s■ CLOSE%s   id=%-14s remote=%-24s in=%-12s out=%-12s dur=%s",
		cGray, cReset, connID, remote, fmtBytes(in), fmtBytes(out), dur.Round(time.Millisecond))
}

func (l *Logger) ConnFail(connID, remote, reason string) {
	l.emit(LevelWarn, "%s✖ FAIL%s    id=%-14s remote=%-24s reason=%s",
		cYellow, cReset, connID, remote, reason)
}

func (l *Logger) ConnRetry(connID string, attempt, max int, delay time.Duration, reason string) {
	l.emit(LevelDebug, "%s↺ RETRY%s   id=%-14s attempt=%d/%d delay=%s reason=%s",
		cGray, cReset, connID, attempt, max, delay.Round(time.Millisecond), reason)
}

func (l *Logger) BypassOK(connID, strategy, sni string, latency time.Duration) {
	l.emit(LevelInfo, "%s✔ BYPASS%s  id=%-14s strategy=%-20s sni=%-32s latency=%s",
		cGreen, cReset, connID, strategy, sni, latency.Round(time.Microsecond))
}

func (l *Logger) BypassFail(connID, strategy, reason string) {
	l.emit(LevelWarn, "%s✘ BYPASS%s  id=%-14s strategy=%-20s reason=%s",
		cYellow, cReset, connID, strategy, reason)
}

func (l *Logger) SNI(connID, sni, remoteIP string) {
	l.emit(LevelInfo, "%s◈ SNI%s     id=%-14s sni=%-40s from=%s",
		cCyan, cReset, connID, sni, remoteIP)
}

func (l *Logger) Relay(connID, local, remote string) {
	l.emit(LevelDebug, "%s⇄ RELAY%s   id=%-14s %s ↔ %s",
		cBlue, cReset, connID, local, remote)
}

func (l *Logger) Health(addr string, alive bool, latency time.Duration, failures int) {
	if alive {
		l.emit(LevelDebug, "%s● HEALTH%s  addr=%-24s alive=true  latency=%s",
			cGreen, cReset, addr, latency.Round(time.Millisecond))
	} else {
		l.emit(LevelWarn, "%s● HEALTH%s  addr=%-24s alive=false failures=%d",
			cRed, cReset, addr, failures)
	}
}

func (l *Logger) Circuit(addr, state string) {
	l.emit(LevelWarn, "%s⚡ CIRCUIT%s addr=%-24s state=%s",
		cMagenta, cReset, addr, state)
}

func (l *Logger) RateLimit(ip string, limit int) {
	l.emit(LevelWarn, "%s⊘ RATELIM%s ip=%-22s limit=%d/s",
		cYellow, cReset, ip, limit)
}

func (l *Logger) Stats(s string) {
	l.emit(LevelInfo, "%s📊 STATS%s   %s", cBlue, cReset, s)
}

func (l *Logger) Startup(key, val string) {
	l.emit(LevelInfo, "%s  %-22s %s%s%s", cWhite+"⚙"+cReset, key, cCyan, val, cReset)
}

func (l *Logger) Section(title string) {
	l.emit(LevelInfo, "%s%s─── %s %s───%s",
		cGray, "", title, "", cReset)
}

func (l *Logger) emit(level Level, format string, args ...interface{}) {
	if level < Level(l.level.Load()) {
		return
	}

	ts := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)

	var lbl, col string
	switch level {
	case LevelDebug:
		lbl, col = "DBG", cGray
	case LevelInfo:
		lbl, col = "INF", cCyan
	case LevelWarn:
		lbl, col = "WRN", cYellow
	case LevelError:
		lbl, col = "ERR", cRed
	default:
		lbl, col = "   ", cReset
	}

	var line string
	if l.noColor {
		if l.connID != "" {
			line = fmt.Sprintf("[%s] [%s] [%s] %s\n", ts, lbl, l.connID, msg)
		} else {
			line = fmt.Sprintf("[%s] [%s] %s\n", ts, lbl, msg)
		}
	} else {
		if l.connID != "" {
			line = fmt.Sprintf("%s[%s]%s %s[%s]%s %s[%s]%s %s\n",
				cGray, ts, cReset,
				col, lbl, cReset,
				cMagenta, l.connID, cReset,
				msg)
		} else {
			line = fmt.Sprintf("%s[%s]%s %s[%s]%s %s\n",
				cGray, ts, cReset,
				col, lbl, cReset,
				msg)
		}
	}

	select {
	case l.ch <- line:
	default:
	}
}

func fmtBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.2fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
