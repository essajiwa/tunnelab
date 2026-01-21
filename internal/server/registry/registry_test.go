package registry

import "testing"

func TestRegistryGetByPortLifecycle(t *testing.T) {
	reg := NewRegistry()

	tunnel := &TunnelInfo{
		ID:         "abc",
		ClientID:   "client",
		Subdomain:  "demo",
		Protocol:   "tcp",
		LocalPort:  9000,
		PublicPort: 31001,
	}

	if err := reg.Register(tunnel); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	retrieved, ok := reg.GetByPort(31001)
	if !ok {
		t.Fatalf("expected tunnel on port 31001")
	}
	if retrieved.Subdomain != tunnel.Subdomain {
		t.Fatalf("unexpected tunnel retrieved: %+v", retrieved)
	}

	reg.Unregister("demo")
	if _, ok := reg.GetByPort(31001); ok {
		t.Fatalf("expected port mapping to be removed after unregister")
	}
}

func TestRegistryRejectsDuplicatePorts(t *testing.T) {
	reg := NewRegistry()

	base := &TunnelInfo{ID: "base", ClientID: "client", Subdomain: "base", Protocol: "tcp", LocalPort: 8000, PublicPort: 32000}
	if err := reg.Register(base); err != nil {
		t.Fatalf("register base failed: %v", err)
	}

	dup := &TunnelInfo{ID: "dup", ClientID: "client", Subdomain: "dup", Protocol: "tcp", LocalPort: 8001, PublicPort: 32000}
	if err := reg.Register(dup); err == nil {
		t.Fatal("expected duplicate port registration to fail")
	}
}
