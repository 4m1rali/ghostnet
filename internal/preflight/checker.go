package preflight

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
)

var KnownSNIDomains = []string{
	"hcaptcha.com",
	"newassets.hcaptcha.com",
	"js.hcaptcha.com",
	"imgs.hcaptcha.com",
	"imgs2.hcaptcha.com",
	"imgs3.hcaptcha.com",
	"assets.hcaptcha.com",
	"api.hcaptcha.com",
	"analytics.hcaptcha.com",
	"stats.hcaptcha.com",
	"three-cust.hcaptcha.com",
	"tg.hcaptcha.com",
	"primary.hcaptcha.com",
	"dashboard.hcaptcha.com",
	"billing.hcaptcha.com",
	"accounts.hcaptcha.com",
	"proxy.hcaptcha.com",
	"loader.hcaptcha.com",
	"challenge-tasks.hcaptcha.com",
	"serverless.hcaptcha.com",
	"health-check.hcaptcha.com",
	"email.hcaptcha.com",
	"admin.vercel.com",
	"assets.vercel.com",
	"auth.vercel.com",
	"cdn1.vercel.com",
	"checkout.vercel.com",
	"conf-feature-flags.vercel.com",
	"data.vercel.com",
	"datarequest.vercel.com",
	"edge-config.vercel.com",
	"examples.vercel.com",
	"go.vercel.com",
	"image.ship.vercel.com",
	"import.vercel.com",
	"info.vercel.com",
	"kms.vercel.com",
	"links.vercel.com",
	"partners-classic.vercel.com",
	"partners.vercel.com",
	"pcosti.vercel.com",
	"pyra.vercel.com",
	"ri2.vercel.com",
	"saml-authentication.vercel.com",
	"security.vercel.com",
	"test.test0001.vercel.com",
	"test.test0002.vercel.com",
	"test.vertex.vercel.com",
	"test0000001.vercel.com",
	"test000002.vercel.com",
	"upflow-email.billing.vercel.com",
}

type Result struct {
	Domain  string
	IP      string
	Latency time.Duration
	Alive   bool
}

type Report struct {
	Best    Result
	Reachable []Result
	Dead    []Result
}

func Run(port int, timeout time.Duration, logger func(string)) Report {
	return RunDomains(KnownSNIDomains, port, timeout, logger)
}

func RunDomains(domains []string, port int, timeout time.Duration, logger func(string)) Report {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	results := make([]Result, len(domains))
	var wg sync.WaitGroup

	for i, domain := range domains {
		wg.Add(1)
		go func(idx int, d string) {
			defer wg.Done()
			r := probe(d, port, timeout)
			results[idx] = r
			if r.Alive {
				logger(fmt.Sprintf("  ✓  %-45s  %s  ip=%s", d, r.Latency.Round(time.Millisecond), r.IP))
			} else {
				logger(fmt.Sprintf("  ✗  %-45s  unreachable", d))
			}
		}(i, domain)
	}
	wg.Wait()

	var reachable, dead []Result
	for _, r := range results {
		if r.Alive {
			reachable = append(reachable, r)
		} else {
			dead = append(dead, r)
		}
	}

	sort.Slice(reachable, func(i, j int) bool {
		return reachable[i].Latency < reachable[j].Latency
	})

	report := Report{Reachable: reachable, Dead: dead}
	if len(reachable) > 0 {
		report.Best = reachable[0]
	}
	return report
}

func probe(domain string, port int, timeout time.Duration) Result {
	addr := net.JoinHostPort(domain, fmt.Sprintf("%d", port))

	ips, err := net.LookupHost(domain)
	resolvedIP := ""
	if err == nil && len(ips) > 0 {
		resolvedIP = ips[0]
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return Result{Domain: domain, IP: resolvedIP, Alive: false}
	}
	conn.Close()
	return Result{
		Domain:  domain,
		IP:      resolvedIP,
		Latency: time.Since(start),
		Alive:   true,
	}
}
