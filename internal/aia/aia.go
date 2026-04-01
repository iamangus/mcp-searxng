package aia

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type cacheEntry struct {
	certs     []*x509.Certificate
	fetchedAt time.Time
}

type CertCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
	client  *http.Client
	stopCh  chan struct{}
}

func NewCertCache(ttl time.Duration) *CertCache {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	c := &CertCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		stopCh: make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

func (c *CertCache) Stop() {
	close(c.stopCh)
}

func (c *CertCache) VerifyPeerCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("aia: no certificates presented")
	}

	leaf, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("aia: failed to parse leaf certificate: %w", err)
	}

	intermediates := x509.NewCertPool()
	for _, raw := range rawCerts[1:] {
		cert, err := x509.ParseCertificate(raw)
		if err != nil {
			continue
		}
		intermediates.AddCert(cert)
	}

	pool, err := x509.SystemCertPool()
	if err != nil {
		return fmt.Errorf("aia: failed to load system cert pool: %w", err)
	}

	opts := x509.VerifyOptions{
		Intermediates: intermediates,
		Roots:         pool,
	}

	if _, err := leaf.Verify(opts); err == nil {
		return nil
	}

	fetched, fetchErr := c.FetchIntermediates(leaf)
	if fetchErr != nil {
		return fmt.Errorf("aia: certificate verification failed and AIA fetch failed: %w (original verify: %v)", fetchErr, err)
	}

	for _, cert := range fetched {
		intermediates.AddCert(cert)
	}

	if _, retryErr := leaf.Verify(opts); retryErr != nil {
		return fmt.Errorf("aia: certificate verification failed even after fetching intermediates: %w", retryErr)
	}

	return nil
}

func (c *CertCache) FetchIntermediates(leaf *x509.Certificate) ([]*x509.Certificate, error) {
	return c.fetchChain(leaf, 3)
}

func (c *CertCache) fetchChain(cert *x509.Certificate, depth int) ([]*x509.Certificate, error) {
	if depth <= 0 {
		return nil, fmt.Errorf("aia: max recursion depth exceeded")
	}

	aiaURLs := cert.IssuingCertificateURL
	if len(aiaURLs) == 0 {
		return nil, fmt.Errorf("aia: no AIA URL in certificate")
	}

	var allFetched []*x509.Certificate

	for _, aiaURL := range aiaURLs {
		cached, ok := c.getFromCache(aiaURL)
		if ok {
			allFetched = append(allFetched, cached...)
			continue
		}

		intermediate, err := c.fetchSingle(aiaURL)
		if err != nil {
			continue
		}

		allFetched = append(allFetched, intermediate)

		pool, _ := x509.SystemCertPool()
		verifyOpts := x509.VerifyOptions{Roots: pool}
		if _, verifyErr := intermediate.Verify(verifyOpts); verifyErr == nil {
			c.storeCache(aiaURL, []*x509.Certificate{intermediate})
			continue
		}

		chained, chainErr := c.fetchChain(intermediate, depth-1)
		if chainErr != nil {
			c.storeCache(aiaURL, []*x509.Certificate{intermediate})
			continue
		}

		result := append([]*x509.Certificate{intermediate}, chained...)
		c.storeCache(aiaURL, result)
		allFetched = append(allFetched, chained...)
	}

	if len(allFetched) == 0 {
		return nil, fmt.Errorf("aia: failed to fetch any intermediate from AIA URLs %v", aiaURLs)
	}

	return allFetched, nil
}

func (c *CertCache) fetchSingle(aiaURL string) (*x509.Certificate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, aiaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("aia: failed to create request for %s: %w", aiaURL, err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aia: failed to fetch %s: %w", aiaURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aia: unexpected status %d fetching %s", resp.StatusCode, aiaURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("aia: failed to read response from %s: %w", aiaURL, err)
	}

	cert, err := x509.ParseCertificate(body)
	if err != nil {
		return nil, fmt.Errorf("aia: failed to parse certificate from %s: %w", aiaURL, err)
	}

	return cert, nil
}

func (c *CertCache) getFromCache(aiaURL string) ([]*x509.Certificate, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[aiaURL]
	if !ok {
		return nil, false
	}

	if time.Since(entry.fetchedAt) > c.ttl {
		return nil, false
	}

	return entry.certs, true
}

func (c *CertCache) storeCache(aiaURL string, certs []*x509.Certificate) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[aiaURL] = cacheEntry{
		certs:     certs,
		fetchedAt: time.Now(),
	}
}

func (c *CertCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

func (c *CertCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.fetchedAt) > c.ttl {
			delete(c.entries, key)
		}
	}
}
