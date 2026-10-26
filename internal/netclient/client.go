// Package netclient owns bounded, reusable outbound HTTP transports.
package netclient

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	Timeout        = 15 * time.Second
	MaxBodyBytes   = 2 << 20
	MaxOutputChars = 50000
)

func ReadBody(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read upstream response")
	}
	if len(b) > MaxBodyBytes {
		return nil, fmt.Errorf("upstream response exceeds %d bytes", MaxBodyBytes)
	}
	return b, nil
}

func transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.MaxIdleConns = 16
	t.MaxIdleConnsPerHost = 4
	t.MaxConnsPerHost = 8
	t.IdleConnTimeout = 30 * time.Second
	t.ResponseHeaderTimeout = 10 * time.Second
	t.TLSHandshakeTimeout = 5 * time.Second
	t.MaxResponseHeaderBytes = 64 << 10
	return t
}

func NewTrusted(proxy func(*http.Request) (*url.URL, error)) *http.Client {
	t := transport()
	t.Proxy = proxy
	return &http.Client{Transport: t, Timeout: Timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		// Search credentials and query parameters must never leave the configured origin.
		if req.URL.Scheme != via[0].URL.Scheme || !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			return fmt.Errorf("cross-origin search redirect rejected")
		}
		return nil
	}}
}

// ParseURL rejects ambiguous URLs, credentials, non-web schemes and scoped IPv6.
func ParseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" || strings.Contains(u.Hostname(), "%") {
		return nil, fmt.Errorf("URL must be an absolute HTTP(S) URL without credentials")
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		return nil, fmt.Errorf("only web ports 80 and 443 are allowed")
	}
	u.Fragment = ""
	return u, nil
}

// Exclude special-purpose space as well as RFC1918/ULA. IsGlobalUnicast alone
// also accepts private, documentation, benchmarking and shared address space.
var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("168.63.129.16/32"), // Azure platform virtual IP.
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/3"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func IsPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// Only currently allocated IPv6 global unicast; this excludes NAT64 and
	// other translation forms which could encode a private IPv4 destination.
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, p := range blocked {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

type lookupFunc func(context.Context, string) ([]netip.Addr, error)

// publicTransport pins each request to a validated numeric destination. Even
// when using an HTTP(S) proxy, the proxy receives that IP, not an unresolved
// hostname. Original HTTP Host and TLS SNI/hostname verification are retained.
// Proxies are administrator-configured trusted infrastructure.
type publicTransport struct {
	mu         sync.Mutex
	transports map[string]*http.Transport
	proxy      func(*http.Request) (*url.URL, error)
	lookup     lookupFunc
}

func NewPublic(proxy func(*http.Request) (*url.URL, error)) *http.Client {
	p := &publicTransport{transports: make(map[string]*http.Transport), proxy: proxy,
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}}
	return &http.Client{Transport: p, Timeout: Timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return fmt.Errorf("HTTPS downgrade redirect rejected")
		}
		_, err := ParseURL(req.URL.String())
		return err // RoundTrip revalidates and pins DNS for every redirect.
	}}
}

func (p *publicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := ParseURL(req.URL.String())
	if err != nil {
		return nil, err
	}
	ips, err := p.lookup(req.Context(), u.Hostname())
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("cannot resolve public destination")
	}
	for _, ip := range ips {
		if !IsPublic(ip) {
			return nil, fmt.Errorf("destination is not a public website")
		}
	}
	var proxyURL *url.URL
	if p.proxy != nil {
		proxyURL, err = p.proxy(req)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy configuration")
		}
	}
	key := strings.ToLower(u.Hostname())
	if proxyURL != nil {
		key += "|" + proxyURL.String()
	}
	p.mu.Lock()
	t := p.transports[key]
	if t == nil {
		if len(p.transports) >= 32 {
			for k, old := range p.transports {
				old.CloseIdleConnections()
				delete(p.transports, k)
				break
			}
		}
		t = transport()
		t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
		if proxyURL != nil {
			// Tunnel HTTP as well as HTTPS. Forward-proxy serialization in
			// net/http derives its absolute request URI from Request.Host,
			// which would undo IP pinning when preserving virtual-host routing.
			t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialProxy(ctx, network, addr, proxyURL)
			}
		}
		p.transports[key] = t
	}
	p.mu.Unlock()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	copyReq := req.Clone(req.Context())
	copyReq.URL = new(url.URL)
	*copyReq.URL = *u
	copyReq.URL.Host = net.JoinHostPort(ips[0].Unmap().String(), port)
	copyReq.Host = u.Host
	resp, err := t.RoundTrip(copyReq)
	if resp != nil {
		resp.Request = req
	}
	return resp, err
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) { return c.reader.Read(b) }

func dialProxy(ctx context.Context, network, target string, proxyURL *url.URL) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	port := proxyURL.Port()
	if port == "" {
		if proxyURL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	conn, err := d.DialContext(ctx, network, net.JoinHostPort(proxyURL.Hostname(), port))
	if err != nil {
		return nil, fmt.Errorf("cannot connect to configured proxy")
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { rawConn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	if proxyURL.Scheme == "https" {
		secure := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: proxyURL.Hostname()})
		if err := secure.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("proxy TLS verification failed")
		}
		conn = secure
	}
	header := ""
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		header = "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username()+":"+password)) + "\r\n"
	}
	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n%s\r\n", target, target, header)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT failed")
	}
	limited := &io.LimitedReader{R: conn, N: 32 << 10}
	reader := bufio.NewReader(limited)
	resp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil || resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT rejected")
	}
	// Preserve any tunnel bytes buffered while reading the bounded headers.
	limited.N = 1<<63 - 1
	conn.SetDeadline(time.Time{})
	return &bufferedConn{Conn: conn, reader: reader}, nil
}

func (p *publicTransport) CloseIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range p.transports {
		t.CloseIdleConnections()
	}
}
