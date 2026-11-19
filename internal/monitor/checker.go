package monitor

import (
	"context"
	"errors"
	"example.com/site-observatory/internal/storage"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var ErrBlocked = errors.New("target IP address is not allowed")

type Checker struct {
	AllowPrivate bool
	Resolver     *net.Resolver
}

func (c Checker) ValidateURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil {
		return e
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return errors.New("only http(s) URLs without embedded credentials are accepted")
	}
	if strings.HasSuffix(strings.ToLower(u.Hostname()), ".local") || strings.EqualFold(u.Hostname(), "localhost") {
		if !c.AllowPrivate {
			return ErrBlocked
		}
	}
	return nil
}
func restricted(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	a = a.Unmap()
	return !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsMulticast() || a.IsUnspecified() || a.Is4In6() || a.IsInterfaceLocalMulticast() || a.Is6() && a.IsPrivate() || a.Is4() && (a.String() == "169.254.169.254" || strings.HasPrefix(a.String(), "100.") || strings.HasPrefix(a.String(), "198.18.") || strings.HasPrefix(a.String(), "198.19."))
}
func (c Checker) resolve(ctx context.Context, host string) ([]net.IP, error) {
	r := c.Resolver
	if r == nil {
		r = net.DefaultResolver
	}
	ips, e := r.LookupIP(ctx, "ip", host)
	if e != nil {
		return nil, e
	}
	if len(ips) == 0 {
		return nil, errors.New("target has no addresses")
	}
	if !c.AllowPrivate {
		for _, ip := range ips {
			if restricted(ip) {
				return nil, ErrBlocked
			}
		}
	}
	return ips, nil
}
func (c Checker) Check(ctx context.Context, raw string, timeout int) storage.Check {
	result := storage.Check{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	start := time.Now()
	if e := c.ValidateURL(raw); e != nil {
		result.Error = e.Error()
		return result
	}
	if timeout < 1 || timeout > 60 {
		timeout = 10
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	transport := &http.Transport{DisableKeepAlives: true, Proxy: nil, MaxIdleConns: 0, TLSHandshakeTimeout: time.Duration(timeout) * time.Second}
	defer transport.CloseIdleConnections()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := c.resolve(ctx, host)
		if e != nil {
			return nil, e
		}
		var last error
		for _, ip := range ips {
			dialer := net.Dialer{Timeout: 5 * time.Second}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
	client := &http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return c.ValidateURL(req.URL.String())
	}}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		result.Error = e.Error()
		return result
	}
	req.Header.Set("User-Agent", "SiteObservatory/0.1 (+self-hosted monitor)")
	resp, e := client.Do(req)
	result.Latency = time.Since(start).Milliseconds()
	if e != nil {
		result.Error = e.Error()
		return result
	}
	defer resp.Body.Close()
	result.Code = resp.StatusCode
	result.Success = resp.StatusCode >= 200 && resp.StatusCode < 400
	if !result.Success {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return result
}
