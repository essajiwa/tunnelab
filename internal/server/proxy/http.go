package proxy

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/essajiwa/tunnelab/internal/server/registry"
)

type HTTPProxy struct {
	registry *registry.Registry
	domain   string
}

func NewHTTPProxy(registry *registry.Registry, domain string) *HTTPProxy {
	return &HTTPProxy{
		registry: registry,
		domain:   domain,
	}
}

func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	subdomain := p.extractSubdomain(r.Host)
	if subdomain == "" {
		http.Error(w, "Invalid subdomain", http.StatusBadRequest)
		return
	}

	_, exists := p.registry.GetBySubdomain(subdomain)
	if !exists {
		http.Error(w, "Tunnel not found", http.StatusNotFound)
		log.Printf("Tunnel not found for subdomain: %s", subdomain)
		return
	}

	stream, err := p.registry.OpenStream(subdomain)
	if err != nil {
		http.Error(w, "Failed to connect to tunnel", http.StatusBadGateway)
		log.Printf("Failed to open stream for %s: %v", subdomain, err)
		return
	}
	defer stream.Close()

	if err := r.Write(stream); err != nil {
		http.Error(w, "Failed to forward request", http.StatusBadGateway)
		log.Printf("Failed to write request to stream: %v", err)
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(stream), r)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusBadGateway)
		log.Printf("Failed to read response from stream: %v", err)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Check if response supports flushing (for SSE and streaming)
	flusher, canFlush := w.(http.Flusher)
	
	// For streaming responses (SSE, chunked, etc.), flush immediately
	isStreaming := resp.Header.Get("Content-Type") == "text/event-stream" ||
		resp.Header.Get("Transfer-Encoding") == "chunked" ||
		resp.Header.Get("X-Accel-Buffering") == "no"

	var written int64
	if isStreaming && canFlush {
		// Stream with immediate flushing for SSE
		buf := make([]byte, 32*1024) // 32KB buffer
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				nw, ew := w.Write(buf[:n])
				written += int64(nw)
				if ew != nil {
					log.Printf("Error writing streaming response: %v", ew)
					break
				}
				flusher.Flush() // Flush immediately for streaming
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading streaming response: %v", err)
				}
				break
			}
		}
	} else {
		// Regular buffered copy for normal responses
		var err error
		written, err = io.Copy(w, resp.Body)
		if err != nil {
			log.Printf("Error copying response body: %v", err)
		}
	}

	duration := time.Since(start)
	log.Printf("[%s] %s %s -> %d (%d bytes, %v)",
		subdomain, r.Method, r.URL.Path, resp.StatusCode, written, duration)
}

func (p *HTTPProxy) extractSubdomain(host string) string {
	host = strings.Split(host, ":")[0]

	if !strings.HasSuffix(host, "."+p.domain) {
		if host == p.domain {
			return ""
		}
		return ""
	}

	subdomain := strings.TrimSuffix(host, "."+p.domain)
	return subdomain
}

func (p *HTTPProxy) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","tunnels":%d}`, p.registry.Count())
}
