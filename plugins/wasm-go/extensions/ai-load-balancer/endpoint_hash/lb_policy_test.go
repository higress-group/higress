package endpoint_hash

import (
	"strings"
	"testing"
)

// Note: NewEndpointHashLoadBalancer and the request/response handlers depend on
// the wasm host (proxywasm log, redis, properties) and cannot run under native
// `go test`; they are exercised via build-image + gateway integration. Here we
// only unit-test the pure stateKey derivation.

func TestStateKey_Deterministic(t *testing.T) {
	k1 := stateKey("route-a", "cluster-a", "alice")
	k2 := stateKey("route-a", "cluster-a", "alice")
	if k1 != k2 {
		t.Errorf("stateKey must be deterministic: %q != %q", k1, k2)
	}
	if !strings.HasPrefix(k1, "higress:endpoint_hash_table:route-a:cluster-a:") {
		t.Errorf("unexpected stateKey format: %q", k1)
	}
}

func TestStateKey_ScopedByRouteAndCluster(t *testing.T) {
	base := stateKey("route-a", "cluster-a", "alice")
	if got := stateKey("route-b", "cluster-a", "alice"); got == base {
		t.Error("different routes must yield different state keys")
	}
	if got := stateKey("route-a", "cluster-b", "alice"); got == base {
		t.Error("different clusters must yield different state keys")
	}
	if got := stateKey("route-a", "cluster-a", "bob"); got == base {
		t.Error("different hash keys must yield different state keys")
	}
}
