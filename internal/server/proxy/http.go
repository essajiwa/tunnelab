package proxy

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
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

	if !p.handleTunnelLookup(w, subdomain) {
		return
	}

	stream, err := p.registry.OpenStream(subdomain)
	if err != nil {
		http.Error(w, "Failed to connect to tunnel", http.StatusBadGateway)
		log.Printf("Failed to open stream for %s: %v", subdomain, err)
		return
	}
	defer stream.Close()

	if !p.handleRequestForwarding(w, r, stream) {
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(stream), r)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusBadGateway)
		log.Printf("Failed to read response from stream: %v", err)
		return
	}
	defer resp.Body.Close()

	p.copyResponse(w, resp, subdomain, r, start)
}

func (p *HTTPProxy) handleTunnelLookup(w http.ResponseWriter, subdomain string) bool {
	_, exists := p.registry.GetBySubdomain(subdomain)
	if !exists {
		http.Error(w, "Tunnel not found", http.StatusNotFound)
		log.Printf("Tunnel not found for subdomain: %s", subdomain)
		return false
	}
	return true
}

func (p *HTTPProxy) handleRequestForwarding(w http.ResponseWriter, r *http.Request, stream net.Conn) bool {
	if err := r.Write(stream); err != nil {
		http.Error(w, "Failed to forward request", http.StatusBadGateway)
		log.Printf("Failed to write request to stream: %v", err)
		return false
	}
	return true
}

func (p *HTTPProxy) copyResponse(w http.ResponseWriter, resp *http.Response, subdomain string, r *http.Request, start time.Time) {
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	flusher, canFlush := w.(http.Flusher)
	isStreaming := p.isStreamingResponse(resp)

	var written int64
	if isStreaming && canFlush {
		written = p.copyStreamingResponse(w, resp.Body, flusher)
	} else {
		written, _ = io.Copy(w, resp.Body)
	}

	duration := time.Since(start)
	log.Printf("[%s] %s %s -> %d (%d bytes, %v)",
		subdomain, r.Method, r.URL.Path, resp.StatusCode, written, duration)
}

func (p *HTTPProxy) isStreamingResponse(resp *http.Response) bool {
	return resp.Header.Get("Content-Type") == "text/event-stream" ||
		resp.Header.Get("Transfer-Encoding") == "chunked" ||
		resp.Header.Get("X-Accel-Buffering") == "no"
}

func (p *HTTPProxy) copyStreamingResponse(w http.ResponseWriter, body io.ReadCloser, flusher http.Flusher) int64 {
	buf := make([]byte, 32*1024) // 32KB buffer
	var written int64
	for {
		n, err := body.Read(buf)
		if n > 0 {
			nw, ew := w.Write(buf[:n])
			written += int64(nw)
			if ew != nil {
				log.Printf("Error writing streaming response: %v", ew)
				break
			}
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading streaming response: %v", err)
			}
			break
		}
	}
	return written
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
