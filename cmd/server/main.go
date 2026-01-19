// TunneLab Server - A secure tunneling server that exposes local HTTP servers to the internet.
//
// This server provides:
//   - WebSocket control connections for tunnel management
//   - HTTP/HTTPS proxy with subdomain routing
//   - Automatic Let's Encrypt certificate generation
//   - Token-based client authentication
//   - Yamux multiplexed connections for efficient data transfer
//
// Usage:
//
//	./tunnelab-server -config configs/server.yaml
//
// Flags:
//
//	-config: Path to configuration file (default: configs/server.yaml)
//	-version: Show version information
//
// Configuration:
//
//	The server is configured via YAML file. See configs/server.example.yaml
//	for a complete example.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/essajiwa/tunnelab/internal/database"
	"github.com/essajiwa/tunnelab/internal/server/config"
	"github.com/essajiwa/tunnelab/internal/server/control"
	"github.com/essajiwa/tunnelab/internal/server/proxy"
	"github.com/essajiwa/tunnelab/internal/server/registry"
	tlsmanager "github.com/essajiwa/tunnelab/internal/server/tls"
)

var (
	version = "dev" // Server version, set during build
)

// main is the entry point for TunneLab server.
func main() {
	configPath := flag.String("config", "configs/server.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("TunneLab Server %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	repo, err := database.NewRepository(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer repo.Close()

	reg := registry.NewRegistry()

	controlHandler := control.NewHandler(reg, repo, cfg.Server.Domain)
	httpProxy := proxy.NewHTTPProxy(reg, cfg.Server.Domain)

	controlMux := http.NewServeMux()
	controlMux.HandleFunc("/", controlHandler.HandleWebSocket)

	proxyMux := http.NewServeMux()
	proxyMux.Handle("/", httpProxy)
	proxyMux.HandleFunc("/health", httpProxy.HandleHealthCheck)

	var certManager *tlsmanager.CertManager
	if cfg.TLS.Mode == "auto" {
		var err error
		certManager, err = tlsmanager.NewCertManager(&tlsmanager.Config{
			Domain:   cfg.Server.Domain,
			Email:    cfg.TLS.Email,
			CacheDir: cfg.TLS.CacheDir,
			Staging:  cfg.TLS.Staging,
		})
		if err != nil {
			log.Fatalf("Failed to create certificate manager: %v", err)
		}
		log.Printf("Let's Encrypt autocert enabled for domain: %s", cfg.Server.Domain)
		if cfg.TLS.Staging {
			log.Printf("WARNING: Using Let's Encrypt STAGING environment")
		}

		proxyMux.Handle("/.well-known/acme-challenge/", certManager.HTTPHandler())
	}

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.ControlPort)
		log.Printf("Starting control server on %s", addr)
		if err := http.ListenAndServe(addr, controlMux); err != nil {
			log.Fatalf("Control server failed: %v", err)
		}
	}()

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
		log.Printf("Starting HTTP proxy on %s", addr)
		if err := http.ListenAndServe(addr, proxyMux); err != nil {
			log.Fatalf("HTTP proxy failed: %v", err)
		}
	}()

	if cfg.TLS.Mode == "auto" {
		go func() {
			addr := fmt.Sprintf(":%d", cfg.Server.HTTPSPort)
			log.Printf("Starting HTTPS proxy on %s (Let's Encrypt)", addr)
			server := &http.Server{
				Addr:      addr,
				Handler:   proxyMux,
				TLSConfig: certManager.TLSConfig(),
			}
			if err := server.ListenAndServeTLS("", ""); err != nil {
				log.Fatalf("HTTPS proxy failed: %v", err)
			}
		}()
	} else if cfg.TLS.Mode == "manual" {
		tlsConfig, err := tlsmanager.LoadManualCerts(cfg.TLS.CertPath, cfg.TLS.KeyPath)
		if err != nil {
			log.Fatalf("Failed to load manual certificates: %v", err)
		}
		go func() {
			addr := fmt.Sprintf(":%d", cfg.Server.HTTPSPort)
			log.Printf("Starting HTTPS proxy on %s (manual certs)", addr)
			server := &http.Server{
				Addr:      addr,
				Handler:   proxyMux,
				TLSConfig: tlsConfig,
			}
			if err := server.ListenAndServeTLS(cfg.TLS.CertPath, cfg.TLS.KeyPath); err != nil {
				log.Fatalf("HTTPS proxy failed: %v", err)
			}
		}()
	}

	log.Printf("TunneLab Server %s started", version)
	log.Printf("Domain: %s", cfg.Server.Domain)
	log.Printf("Control: :%d", cfg.Server.ControlPort)
	log.Printf("HTTP: :%d", cfg.Server.HTTPPort)
	if cfg.TLS.Mode != "disabled" {
		log.Printf("HTTPS: :%d (%s)", cfg.Server.HTTPSPort, cfg.TLS.Mode)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down gracefully...")
}
