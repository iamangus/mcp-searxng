package aia_test

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/iamangus/mcp-searxng/internal/aia"
)

func TestNewCertCache_DefaultTTL(t *testing.T) {
	cache := aia.NewCertCache(0)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	cache.Stop()
}

func TestNewCertCache_CustomTTL(t *testing.T) {
	cache := aia.NewCertCache(5 * time.Minute)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	cache.Stop()
}

func TestCertCache_StoreAndGet(t *testing.T) {
	cache := aia.NewCertCache(1 * time.Hour)
	defer cache.Stop()

	template := &x509.Certificate{}
	cache.StoreCache("http://example.com/aia.cer", []*x509.Certificate{template})

	certs, ok := cache.GetFromCache("http://example.com/aia.cer")
	if !ok {
		t.Fatal("expected to find cached entry")
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
}

func TestCertCache_ExpiredEntry(t *testing.T) {
	cache := aia.NewCertCache(50 * time.Millisecond)
	defer cache.Stop()

	template := &x509.Certificate{}
	cache.StoreCache("http://example.com/aia.cer", []*x509.Certificate{template})

	time.Sleep(100 * time.Millisecond)

	_, ok := cache.GetFromCache("http://example.com/aia.cer")
	if ok {
		t.Fatal("expected expired entry to be gone")
	}
}

func TestCertCache_MissingKey(t *testing.T) {
	cache := aia.NewCertCache(1 * time.Hour)
	defer cache.Stop()

	_, ok := cache.GetFromCache("http://example.com/nonexistent.cer")
	if ok {
		t.Fatal("expected missing key to return false")
	}
}

func TestCertCache_Cleanup(t *testing.T) {
	cache := aia.NewCertCache(50 * time.Millisecond)
	defer cache.Stop()

	template := &x509.Certificate{}
	cache.StoreCache("http://example.com/aia1.cer", []*x509.Certificate{template})
	cache.StoreCache("http://example.com/aia2.cer", []*x509.Certificate{template})

	time.Sleep(100 * time.Millisecond)

	_, ok1 := cache.GetFromCache("http://example.com/aia1.cer")
	_, ok2 := cache.GetFromCache("http://example.com/aia2.cer")
	if ok1 || ok2 {
		t.Fatal("expected expired entries to be cleaned up")
	}
}

func TestVerifyPeerCertificate_NoCerts(t *testing.T) {
	cache := aia.NewCertCache(1 * time.Hour)
	defer cache.Stop()

	err := cache.VerifyPeerCertificate(nil, nil)
	if err == nil {
		t.Fatal("expected error for no certificates")
	}
}

func TestVerifyPeerCertificate_EmptyCerts(t *testing.T) {
	cache := aia.NewCertCache(1 * time.Hour)
	defer cache.Stop()

	err := cache.VerifyPeerCertificate([][]byte{}, nil)
	if err == nil {
		t.Fatal("expected error for empty certificates")
	}
}

func TestFetchIntermediates_NoAIAURL(t *testing.T) {
	cache := aia.NewCertCache(1 * time.Hour)
	defer cache.Stop()

	template := &x509.Certificate{}
	_, err := cache.FetchIntermediates(template)
	if err == nil {
		t.Fatal("expected error for certificate with no AIA URL")
	}
}
