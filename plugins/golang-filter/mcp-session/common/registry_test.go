package common

import (
	"errors"
	"testing"
)

type testRegistryServer struct {
	value string
}

func (s *testRegistryServer) ParseConfig(map[string]any) error {
	return nil
}

func (s *testRegistryServer) NewServer(serverName string) (*MCPServer, error) {
	return NewMCPServer(serverName, "test"), nil
}

type testCloningServer struct {
	*testRegistryServer
	cloneCount *int
}

func (s *testCloningServer) Clone() Server {
	(*s.cloneCount)++
	return &testCloningServer{
		testRegistryServer: &testRegistryServer{value: s.value},
		cloneCount:         s.cloneCount,
	}
}

type testValueServer struct{}

func (s testValueServer) ParseConfig(map[string]any) error {
	return nil
}

func (s testValueServer) NewServer(string) (*MCPServer, error) {
	return nil, errors.New("value server should not be cloned")
}

func TestNewServerConfigUsesServerCloner(t *testing.T) {
	registry := NewServerRegistry()
	cloneCount := 0
	registry.RegisterServer("cloner", &testCloningServer{
		testRegistryServer: &testRegistryServer{value: "template"},
		cloneCount:         &cloneCount,
	})

	cloned := registry.NewServerConfig("cloner")
	if cloned == nil {
		t.Fatal("expected cloned server")
	}
	if cloneCount != 1 {
		t.Fatalf("expected Clone to be called once, got %d", cloneCount)
	}
	if cloned == registry.GetServer("cloner") {
		t.Fatal("expected Clone to return an independent instance")
	}
}

func TestNewServerConfigClonesPointerServer(t *testing.T) {
	registry := NewServerRegistry()
	original := &testRegistryServer{value: "template"}
	registry.RegisterServer("pointer", original)

	cloned := registry.NewServerConfig("pointer")
	if cloned == nil {
		t.Fatal("expected cloned server")
	}
	if cloned == original {
		t.Fatal("expected pointer server to be shallow-cloned")
	}

	cloned.(*testRegistryServer).value = "changed"
	if original.value != "template" {
		t.Fatalf("expected original to remain unchanged, got %q", original.value)
	}
}

func TestNewServerConfigRejectsValueServer(t *testing.T) {
	registry := NewServerRegistry()
	registry.RegisterServer("value", testValueServer{})

	if cloned := registry.NewServerConfig("value"); cloned != nil {
		t.Fatalf("expected value server to be rejected, got %#v", cloned)
	}
}

func TestNewServerConfigHandlesNilPointer(t *testing.T) {
	registry := NewServerRegistry()
	var server *testRegistryServer
	registry.RegisterServer("nil", server)

	if cloned := registry.NewServerConfig("nil"); cloned != nil {
		t.Fatalf("expected nil pointer server to be rejected, got %#v", cloned)
	}
}

func TestNewServerConfigReturnsIndependentInstances(t *testing.T) {
	registry := NewServerRegistry()
	registry.RegisterServer("pointer", &testRegistryServer{value: "template"})

	first := registry.NewServerConfig("pointer").(*testRegistryServer)
	second := registry.NewServerConfig("pointer").(*testRegistryServer)
	first.value = "first"
	second.value = "second"

	if first == second {
		t.Fatal("expected each NewServerConfig call to return a distinct instance")
	}
	if registry.GetServer("pointer").(*testRegistryServer).value != "template" {
		t.Fatal("expected registered template to remain unchanged")
	}
}
