package profilesrv

import (
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

// limiter allows n events per window for each key (fixed windows; old keys
// are dropped as windows pass).
type limiter struct {
	n      int
	window time.Duration
	now    func() time.Time

	mu    sync.Mutex
	start time.Time
	count map[string]int
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, now: time.Now, count: map[string]int{}}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := l.now(); now.Sub(l.start) >= l.window {
		l.start, l.count = now, map[string]int{}
	}
	if l.count[key] >= l.n {
		return false
	}
	l.count[key]++
	return true
}

// cloudflare are Cloudflare's published edge ranges
// (https://www.cloudflare.com/ips/). Only a request from one of them may tell
// the visitor's address in CF-Connecting-IP.
var cloudflare = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
		"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
		"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
		"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
		"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// ClientIP is the visitor's address: CF-Connecting-IP when the request came
// through Cloudflare, otherwise the connection's own address.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	addr = addr.Unmap()
	for _, p := range cloudflare {
		if p.Contains(addr) {
			if v, err := netip.ParseAddr(r.Header.Get("CF-Connecting-IP")); err == nil {
				return v.Unmap().String()
			}
			break
		}
	}
	return addr.String()
}
