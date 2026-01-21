package control

import (
	"fmt"
	"testing"

	"github.com/essajiwa/tunnelab/internal/server/registry"
)

func TestPortAllocatorAllocateSkipsUsedPorts(t *testing.T) {
	reg := registry.NewRegistry()
	if err := reg.Register(&registry.TunnelInfo{
		ID:         "t-used",
		ClientID:   "client",
		Subdomain:  "used",
		PublicPort: 30001,
	}); err != nil {
		t.Fatalf("failed to register existing port: %v", err)
	}

	allocator := &portAllocator{start: 30000, end: 30002, next: 30000}

	got := make(map[int]bool)
	for i := 0; i < 2; i++ {
		port, err := allocator.allocate(reg)
		if err != nil {
			t.Fatalf("allocate failed on iteration %d: %v", i, err)
		}
		if port < 30000 || port > 30002 {
			t.Fatalf("allocated port %d outside expected range", port)
		}
		if got[port] {
			t.Fatalf("port %d allocated twice", port)
		}
		got[port] = true

		// Mark the newly allocated port as used inside the registry so the allocator advances.
		if err := reg.Register(&registry.TunnelInfo{
			ID:         fmt.Sprintf("tunnel-%d", i),
			ClientID:   "client",
			Subdomain:  fmt.Sprintf("sub-%d", i),
			PublicPort: port,
		}); err != nil {
			t.Fatalf("failed to register allocated port: %v", err)
		}
	}

	if !got[30000] || !got[30002] {
		t.Fatalf("expected allocator to cover remaining free ports, got %+v", got)
	}
}

func TestPortAllocatorAllocateExhaustedRange(t *testing.T) {
	reg := registry.NewRegistry()
	if err := reg.Register(&registry.TunnelInfo{
		ID:         "only",
		ClientID:   "client",
		Subdomain:  "only",
		PublicPort: 40000,
	}); err != nil {
		t.Fatalf("failed to reserve only port: %v", err)
	}

	allocator := &portAllocator{start: 40000, end: 40000, next: 40000}
	if _, err := allocator.allocate(reg); err == nil {
		t.Fatal("expected allocation to fail when range is exhausted")
	}
}
