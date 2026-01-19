// Package tls provides TLS certificate management for TunneLab.
//
// This package supports automatic certificate generation using Let's Encrypt
// as well as manual certificate loading. It handles certificate caching,
// renewal, and provides secure TLS configurations.
//
// Features:
//   - Automatic Let's Encrypt certificate generation
//   - Certificate caching and renewal
//   - Manual certificate support
//   - Secure TLS configuration with modern ciphers
//
// Usage:
//
//   // Automatic certificate management
//   certManager, err := NewCertManager(&Config{
//       Domain:  "example.com",
//       Email:   "admin@example.com",
//       Staging: false,
//   })
//   if err != nil {
//       log.Fatal(err)
//   }
//
//   // Use with HTTP server
//   server := &http.Server{
//       Addr:      ":443",
//       TLSConfig: certManager.TLSConfig(),
//   }
package tls

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/acme/autocert"
)

// CertManager manages TLS certificates using Let's Encrypt.
type CertManager struct {
	manager *autocert.Manager // Let's Encrypt manager
	config  *tls.Config       // TLS configuration
}

// Config contains certificate manager configuration.
type Config struct {
	Domain   string // Domain for certificates (e.g., "example.com")
	Email    string // Email for Let's Encrypt notifications
	CacheDir string // Directory to cache certificates
	Staging  bool   // Use Let's Encrypt staging environment
}

// NewCertManager creates a new certificate manager with Let's Encrypt support.
//
// It sets up automatic certificate generation, caching, and renewal.
// The host policy allows the main domain and all subdomains.
//
// Parameters:
//   - cfg: Configuration for the certificate manager
//
// Returns:
//   - *CertManager: Certificate manager ready to use
//   - error: Error if setup fails
func NewCertManager(cfg *Config) (*CertManager, error) {
	if cfg.CacheDir == "" {
		cfg.CacheDir = "./certs"
	}

	if err := os.MkdirAll(cfg.CacheDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cert cache directory: %w", err)
	}

	// Allow the main domain and all subdomains
	hostPolicy := func(ctx context.Context, host string) error {
		// Allow exact domain match
		if host == cfg.Domain {
			return nil
		}
		// Allow control subdomain
		if host == "control."+cfg.Domain {
			return nil
		}
		// Allow any subdomain of the main domain
		if strings.HasSuffix(host, "."+cfg.Domain) {
			return nil
		}
		return fmt.Errorf("host %q not configured", host)
	}

	manager := &autocert.Manager{
		Prompt:      autocert.AcceptTOS,
		HostPolicy:  hostPolicy,
		Cache:       autocert.DirCache(cfg.CacheDir),
		Email:       cfg.Email,
	}

	if cfg.Staging {
		log.Println("Using Let's Encrypt STAGING environment")
	}

	tlsConfig := &tls.Config{
		GetCertificate: manager.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519,
		},
	}

	return &CertManager{
		manager: manager,
		config:  tlsConfig,
	}, nil
}

func (cm *CertManager) TLSConfig() *tls.Config {
	return cm.config
}

func (cm *CertManager) HTTPHandler() http.Handler {
	return cm.manager.HTTPHandler(nil)
}

func LoadManualCerts(certPath, keyPath string) (*tls.Config, error) {
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("cert_path and key_path are required for manual TLS mode")
	}

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("certificate file not found: %s", certPath)
	}

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("key file not found: %s", keyPath)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func GetCertCachePath(domain string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".tunnelab", "certs", domain)
	}
	return filepath.Join(home, ".tunnelab", "certs", domain)
}
