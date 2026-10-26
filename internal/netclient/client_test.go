package netclient

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

func TestPublicAddresses(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "::1", "10.2.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.100.100.200", "0.0.0.0", "224.0.0.1", "240.0.0.1", "198.18.0.1", "192.0.2.1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "64:ff9b::7f00:1", "2002:7f00:1::", "2001:db8::1"} {
		if IsPublic(netip.MustParseAddr(raw)) {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !IsPublic(netip.MustParseAddr(raw)) {
			t.Errorf("rejected %s", raw)
		}
	}
}

func TestParseURL(t *testing.T) {
	for _, raw := range []string{"http:", "httpfoo://example.com", "file:///etc/passwd", "http://user:pass@example.com", "http://example.com:8080", "http://[fe80::1%25eth0]", "https:///path"} {
		if _, err := ParseURL(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestPrivateAndMixedDNSRejectedBeforeProxy(t *testing.T) {
	client := NewPublic(func(*http.Request) (*url.URL, error) {
		t.Fatal("proxy consulted for private destination")
		return nil, nil
	})
	p := client.Transport.(*publicTransport)
	for _, ips := range [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}, {netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.1")}} {
		p.lookup = func(context.Context, string) ([]netip.Addr, error) { return ips, nil }
		if _, err := client.Get("http://evil.example"); err == nil {
			t.Fatal("private DNS accepted")
		}
	}
}

func TestProxyReceivesPinnedIPAndOriginalHost(t *testing.T) {
	proxy := tunnelProxy(t, func(r *http.Request) string {
		if r.Host != "public.example" {
			t.Errorf("Host=%s", r.Host)
		}
		return "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"
	})
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	client := NewPublic(http.ProxyURL(u))
	defer client.CloseIdleConnections()
	client.Transport.(*publicTransport).lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	resp, err := client.Get("http://public.example/path")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Request.URL.Host != "public.example" {
		t.Fatal("leaked pinned address into response URL")
	}
}

func TestRedirectToPrivateDestinationRejected(t *testing.T) {
	calls := 0
	proxy := tunnelProxy(t, func(r *http.Request) string {
		calls++
		return "HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1/secret\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
	})
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	client := NewPublic(http.ProxyURL(u))
	defer client.CloseIdleConnections()
	client.Transport.(*publicTransport).lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "public.example" {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}
		return []netip.Addr{netip.MustParseAddr(host)}, nil
	}
	if _, err := client.Get("http://public.example"); err == nil {
		t.Fatal("private redirect accepted")
	}
	if calls != 1 {
		t.Fatal("private redirect reached proxy")
	}
}

func tunnelProxy(t *testing.T, respond func(*http.Request) string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != "8.8.8.8:80" {
			t.Errorf("not pinned: %s %s", r.Method, r.Host)
			http.Error(w, "bad target", 400)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			t.Error(err)
			return
		}
		defer req.Body.Close()
		fmt.Fprint(conn, respond(req))
	}))
}

func TestTrustedTLSAndHostnameVerification(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer upstream.Close()
	client := NewTrusted(nil)
	defer client.CloseIdleConnections()
	if _, err := client.Get(upstream.URL); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	pool := x509.NewCertPool()
	pool.AddCert(upstream.Certificate())
	tr := client.Transport.(*http.Transport)
	tr.TLSClientConfig = &tls.Config{RootCAs: pool}
	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	tr.CloseIdleConnections()
	// Even a trusted leaf must match the requested hostname.
	tr.TLSClientConfig = &tls.Config{RootCAs: pool, ServerName: "wrong.example"}
	if _, err := client.Get(upstream.URL); err == nil {
		t.Fatal("hostname mismatch accepted")
	}
}

func TestBodyLimit(t *testing.T) {
	if _, err := ReadBody(strings.NewReader(strings.Repeat("x", MaxBodyBytes+1))); err == nil {
		t.Fatal("oversize response accepted")
	}
}

func TestPublicTLSHostnameThroughProxy(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != "8.8.8.8:443" {
			t.Errorf("unpinned CONNECT: %s %s", r.Method, r.Host)
			http.Error(w, "bad target", 400)
			return
		}
		remote, err := net.Dial("tcp", upstreamURL.Host)
		if err != nil {
			t.Error(err)
			return
		}
		defer remote.Close()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		done := make(chan struct{})
		go func() { io.Copy(remote, conn); remote.Close(); close(done) }()
		io.Copy(conn, remote)
		conn.Close()
		<-done
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	c := NewPublic(http.ProxyURL(proxyURL))
	defer c.CloseIdleConnections()
	p := c.Transport.(*publicTransport)
	p.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	pool := x509.NewCertPool()
	pool.AddCert(upstream.Certificate())
	validHost := upstream.Certificate().DNSNames[0]
	for _, host := range []string{validHost, "wrong.example"} {
		if _, err := c.Get("https://" + host); err == nil {
			t.Fatal("untrusted certificate accepted")
		}
		tr := p.transports[host+"|"+proxyURL.String()]
		if tr.TLSClientConfig.ServerName != host || tr.TLSClientConfig.InsecureSkipVerify {
			t.Fatal("normal hostname verification not configured")
		}
		tr.CloseIdleConnections()
		cfg := tr.TLSClientConfig.Clone()
		cfg.RootCAs = pool
		tr.TLSClientConfig = cfg
		resp, err := c.Get("https://" + host)
		if host == validHost {
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
		} else if err == nil {
			resp.Body.Close()
			t.Fatal("trusted certificate with wrong hostname accepted")
		}
	}
}

func TestDNSRebindingOnSameOriginRedirect(t *testing.T) {
	proxy := tunnelProxy(t, func(*http.Request) string {
		return "HTTP/1.1 302 Found\r\nLocation: /next\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
	})
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	client := NewPublic(http.ProxyURL(u))
	defer client.CloseIdleConnections()
	calls := 0
	client.Transport.(*publicTransport).lookup = func(context.Context, string) ([]netip.Addr, error) {
		calls++
		if calls == 1 {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
	}
	if _, err := client.Get("http://public.example"); err == nil {
		t.Fatal("DNS rebinding accepted")
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
