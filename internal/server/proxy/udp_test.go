package proxy

import (
	"io"
	"net"
	"testing"

	"github.com/essajiwa/tunnelab/internal/server/registry"
	"github.com/essajiwa/tunnelab/pkg/protocol"
)

func TestLengthPrefixedRoundtrip(t *testing.T) {
	payload := []byte("hello udp")

	r, w := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- protocol.WriteLengthPrefixed(w, payload)
		w.Close()
	}()

	got, err := protocol.ReadLengthPrefixed(r)
	if err != nil {
		t.Fatalf("ReadLengthPrefixed error: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("WriteLengthPrefixed error: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("expected %q, got %q", payload, got)
	}
}

func TestLengthPrefixedEmptyPayload(t *testing.T) {
	r, w := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- protocol.WriteLengthPrefixed(w, []byte{})
		w.Close()
	}()

	got, err := protocol.ReadLengthPrefixed(r)
	if err != nil {
		t.Fatalf("ReadLengthPrefixed error: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("WriteLengthPrefixed error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty payload, got %q", got)
	}
}

func TestReadLengthPrefixedEOF(t *testing.T) {
	r, w := io.Pipe()
	w.Close()
	_, err := protocol.ReadLengthPrefixed(r)
	if err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Fatalf("expected EOF-style error, got %v", err)
	}
}

func TestReadLengthPrefixedTruncatedPayload(t *testing.T) {
	r, w := io.Pipe()
	go func() {
		hdr := make([]byte, 4)
		// big-endian length of 10
		hdr[3] = 10
		w.Write(hdr)
		w.Write([]byte("hi!"))
		w.Close()
	}()
	_, err := protocol.ReadLengthPrefixed(r)
	if err == nil {
		t.Fatal("expected error for truncated payload, got nil")
	}
}

func TestUDPListenAndForwardBinds(t *testing.T) {
	pc, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatalf("could not find free port: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	pc.Close()

	reg := registry.NewRegistry()
	p := NewUDPProxy(reg)
	if err := p.ListenAndForward(port, "test-subdomain"); err != nil {
		t.Fatalf("ListenAndForward failed: %v", err)
	}
	t.Cleanup(func() {
		p.StopForwarding("test-subdomain")
	})
}
