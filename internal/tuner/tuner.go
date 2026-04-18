package tuner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Setting struct {
	Key   string
	Value string
	Note  string
}

var settings = []Setting{
	{"fs.file-max", "2097152", "max open file descriptors"},
	{"fs.nr_open", "2097152", "max open files per process"},
	{"net.core.somaxconn", "65535", "TCP accept backlog"},
	{"net.ipv4.tcp_max_syn_backlog", "65535", "SYN queue depth"},
	{"net.ipv4.ip_local_port_range", "1024 65535", "outbound port range"},
	{"net.core.rmem_default", "262144", "default socket receive buffer"},
	{"net.core.wmem_default", "262144", "default socket send buffer"},
	{"net.core.rmem_max", "134217728", "max socket receive buffer (128MB)"},
	{"net.core.wmem_max", "134217728", "max socket send buffer (128MB)"},
	{"net.ipv4.tcp_rmem", "4096 87380 134217728", "TCP receive buffer min/default/max"},
	{"net.ipv4.tcp_wmem", "4096 65536 134217728", "TCP send buffer min/default/max"},
	{"net.ipv4.tcp_fastopen", "3", "TCP Fast Open (client+server)"},
	{"net.ipv4.tcp_window_scaling", "1", "TCP window scaling"},
	{"net.ipv4.tcp_tw_reuse", "1", "reuse TIME_WAIT sockets"},
	{"net.ipv4.tcp_fin_timeout", "15", "FIN timeout in seconds"},
	{"net.ipv4.tcp_max_tw_buckets", "1440000", "max TIME_WAIT sockets"},
	{"net.ipv4.tcp_keepalive_time", "60", "keepalive idle time"},
	{"net.ipv4.tcp_keepalive_intvl", "10", "keepalive probe interval"},
	{"net.ipv4.tcp_keepalive_probes", "6", "keepalive probe count"},
	{"net.core.netdev_max_backlog", "65536", "network device receive queue"},
	{"net.ipv4.conf.all.rp_filter", "0", "allow injected packets (required)"},
	{"net.ipv4.conf.default.rp_filter", "0", "allow injected packets (default)"},
	{"vm.max_map_count", "262144", "virtual memory map count"},
}

type Result struct {
	Key     string
	Before  string
	After   string
	Changed bool
	Error   string
}

func Apply(dryRun bool) ([]Result, error) {
	results := make([]Result, 0, len(settings))

	for _, s := range settings {
		before := readSysctl(s.Key)
		r := Result{Key: s.Key, Before: before}

		if dryRun {
			r.After = s.Value
			r.Changed = before != s.Value
			results = append(results, r)
			continue
		}

		cmd := exec.Command("sysctl", "-w", s.Key+"="+s.Value)
		out, err := cmd.CombinedOutput()
		if err != nil {
			r.Error = strings.TrimSpace(string(out))
		} else {
			r.After = s.Value
			r.Changed = before != s.Value
		}
		results = append(results, r)
	}

	if !dryRun {
		if err := persistSysctl(); err != nil {
			return results, fmt.Errorf("persist sysctl: %w", err)
		}
		if err := applyUlimit(); err != nil {
			return results, fmt.Errorf("apply ulimit: %w", err)
		}
		loadBBR()
	}

	return results, nil
}

func readSysctl(key string) string {
	path := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func persistSysctl() error {
	path := "/etc/sysctl.d/99-ghostnet.conf"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "# GhostNet kernel tuning — auto-generated")
	for _, s := range settings {
		fmt.Fprintf(w, "%s = %s\n", s.Key, s.Value)
	}
	return w.Flush()
}

func applyUlimit() error {
	path := "/etc/security/limits.d/99-ghostnet.conf"
	content := "# GhostNet ulimit\n*    soft nofile 1048576\n*    hard nofile 1048576\nroot soft nofile 1048576\nroot hard nofile 1048576\n"
	return os.WriteFile(path, []byte(content), 0644)
}

func loadBBR() {
	exec.Command("modprobe", "tcp_bbr").Run()
	exec.Command("sysctl", "-w", "net.core.default_qdisc=fq").Run()
	exec.Command("sysctl", "-w", "net.ipv4.tcp_congestion_control=bbr").Run()
}

func IsRoot() bool {
	return os.Getuid() == 0
}

func CurrentUlimit() int {
	data, err := os.ReadFile("/proc/self/limits")
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Max open files") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				n, err := strconv.Atoi(fields[3])
				if err == nil {
					return n
				}
			}
		}
	}
	return -1
}
