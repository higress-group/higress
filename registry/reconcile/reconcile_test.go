// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reconcile

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	v1 "github.com/alibaba/higress/v2/client/pkg/apis/networking/v1"
	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	"github.com/alibaba/higress/v2/pkg/kube"
	"github.com/alibaba/higress/v2/registry/memory"
)

func staticRegistry(name, domain string) *apiv1.RegistryConfig {
	return &apiv1.RegistryConfig{Type: "static", Name: name, Domain: domain, Port: 80}
}

func bridge(registries ...*apiv1.RegistryConfig) *v1.McpBridge {
	return &v1.McpBridge{Spec: apiv1.McpBridge{Registries: registries}}
}

func TestReconcileFailedReplacementInvalidatesOldServices(t *testing.T) {
	var updates atomic.Int32
	r := NewReconciler(func() { updates.Add(1) }, kube.NewFakeClient(), "higress-system", "test")
	initial := staticRegistry("backend", "192.0.2.1:8080")
	require.NoError(t, r.Reconcile(bridge(initial)))
	require.Len(t, r.GetAllServiceEntry(), 1)
	oldWatcher := r.watchers["static/backend"]
	updates.Store(0)

	replacement := staticRegistry("backend", "192.0.2.2:8080")
	replacement.AuthSecretName = "missing-secret"
	err := r.Reconcile(bridge(replacement))
	require.ErrorContains(t, err, "Init Watchers failed")
	require.False(t, oldWatcher.IsReady(), "the old watcher must be stopped")
	require.NotContains(t, r.watchers, "static/backend")
	require.NotContains(t, r.registries, "static/backend")
	require.Empty(t, r.GetAllServiceEntry(), "stopped registry data must not remain published")
	require.EqualValues(t, 1, updates.Load(), "purging old data must notify consumers")
	require.False(t, r.PurgeStaleItems(), "reconciliation must finish its deferred deletions")
}

func TestReconcileFailedReplacementCanRetry(t *testing.T) {
	var updates atomic.Int32
	client := kube.NewFakeClient()
	r := NewReconciler(func() { updates.Add(1) }, client, "higress-system", "test")
	require.NoError(t, r.Reconcile(bridge(staticRegistry("backend", "192.0.2.1:8080"))))
	replacement := staticRegistry("backend", "192.0.2.2:8080")
	replacement.AuthSecretName = "eventual-secret"
	require.ErrorContains(t, r.Reconcile(bridge(replacement)), "Init Watchers failed")
	updates.Store(0)
	for i := 0; i < 3; i++ {
		require.ErrorContains(t, r.Reconcile(bridge(replacement)), "Init Watchers failed")
		require.Empty(t, r.watchers)
		require.Empty(t, r.registries)
		require.Empty(t, r.watcherReady)
		require.Empty(t, r.GetAllServiceEntry())
	}
	require.Zero(t, updates.Load(), "repeated failure must not manufacture cache changes")

	_, err := client.Kube().CoreV1().Secrets("higress-system").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "eventual-secret", Namespace: "higress-system"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NoError(t, r.Reconcile(bridge(replacement)))
	require.Equal(t, map[string]string{"backend.static": "192.0.2.2"}, serviceAddresses(r.Cache))
	watcher := r.watchers["static/backend"]
	updates.Store(0)
	require.NoError(t, r.Reconcile(bridge(replacement)))
	require.Same(t, watcher, r.watchers["static/backend"])
	require.Zero(t, updates.Load())
	require.NoError(t, r.Reconcile(nil))
	require.Empty(t, r.GetAllServiceEntry())
	require.Empty(t, r.watcherReady)
}

func TestReconcileConstructionFailuresFinishMixedChanges(t *testing.T) {
	var snapshotsMu sync.Mutex
	var snapshots []map[string]string
	var r *Reconciler
	r = NewReconciler(func() {
		snapshotsMu.Lock()
		defer snapshotsMu.Unlock()
		snapshots = append(snapshots, serviceAddresses(r.Cache))
	}, kube.NewFakeClient(), "higress-system", "test")
	keep := staticRegistry("keep", "192.0.2.1:8080")
	initial := bridge(keep, staticRegistry("bad", "192.0.2.2:8080"),
		staticRegistry("changed", "192.0.2.3:8080"), staticRegistry("removed", "192.0.2.4:8080"))
	initial.Spec.Proxies = []*apiv1.ProxyConfig{httpProxy("old-proxy", "192.0.2.10")}
	require.NoError(t, r.Reconcile(initial))
	keepWatcher := r.watchers["static/keep"]
	bad := staticRegistry("bad", "192.0.2.20:8080")
	bad.AuthSecretName = "missing-secret"
	badNew := staticRegistry("bad-new", "192.0.2.21:8080")
	badNew.AuthSecretName = "missing-secret"
	desired := bridge(keep, bad, badNew, staticRegistry("changed", "192.0.2.30:8080"), staticRegistry("created", "192.0.2.40:8080"))
	desired.Spec.Proxies = []*apiv1.ProxyConfig{httpProxy("new-proxy", "192.0.2.11")}
	snapshotsMu.Lock()
	snapshots = nil
	snapshotsMu.Unlock()

	require.ErrorContains(t, r.Reconcile(desired), "Init Watchers failed")
	want := map[string]string{"keep.static": "192.0.2.1", "changed.static": "192.0.2.30", "created.static": "192.0.2.40"}
	require.Equal(t, want, serviceAddresses(r.Cache))
	require.Same(t, keepWatcher, r.watchers["static/keep"])
	require.Len(t, r.watchers, 3)
	require.Len(t, r.registries, 3)
	require.Len(t, r.watcherReady, 3)
	proxies := r.GetAllProxyWrapper()
	require.Len(t, proxies, 1)
	require.Equal(t, "new-proxy", proxies[0].ProxyName)
	require.NotContains(t, r.proxies, "old-proxy")
	require.Contains(t, r.proxies, "new-proxy")
	snapshotsMu.Lock()
	require.Contains(t, snapshots, want, "consumers must be notified after stale entries disappear")
	snapshotsMu.Unlock()
	require.False(t, r.PurgeStaleItems())
}

func TestReconcileTimeoutRetryWaitsForExistingWatcher(t *testing.T) {
	gate := &publicationGate{Cache: memory.NewCache(), entered: make(chan struct{}), release: make(chan struct{})}
	r := NewReconciler(func() {}, kube.NewFakeClient(), "higress-system", "test")
	r.Cache = gate
	require.NoError(t, r.Reconcile(bridge(staticRegistry("slow", "192.0.2.1:8080"), staticRegistry("bad", "192.0.2.2:8080"))))
	r.readyTimeout = 30 * time.Millisecond
	initialPurges := gate.purges.Load()
	bad := staticRegistry("bad", "192.0.2.3:8080")
	bad.AuthSecretName = "missing-secret"
	desired := bridge(staticRegistry("slow", "192.0.2.20:8080"), bad)
	defer gate.unblock()

	require.ErrorContains(t, r.Reconcile(desired), "waiting for ready timeout")
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("replacement watcher did not start")
	}
	pendingWatcher := r.watchers["static/slow"]
	for i := 0; i < 2; i++ {
		require.ErrorContains(t, r.Reconcile(desired), "waiting for ready timeout")
		require.Same(t, pendingWatcher, r.watchers["static/slow"])
		require.Equal(t, initialPurges, gate.purges.Load(), "do not finalize while replacement publication is pending")
		require.Equal(t, map[string]string{"slow.static": "192.0.2.1", "bad.static": "192.0.2.2"}, serviceAddresses(r.Cache))
	}
	gate.unblock()
	r.readyTimeout = time.Second
	require.ErrorContains(t, r.Reconcile(desired), "Init Watchers failed")
	require.Equal(t, map[string]string{"slow.static": "192.0.2.20"}, serviceAddresses(r.Cache))
	require.Equal(t, initialPurges+1, gate.purges.Load())
}

func TestReconcileWaitsForSuccessfulSiblingBeforePurging(t *testing.T) {
	gate := &publicationGate{Cache: memory.NewCache(), entered: make(chan struct{}), release: make(chan struct{})}
	r := NewReconciler(func() {}, kube.NewFakeClient(), "higress-system", "test")
	r.Cache = gate
	r.readyTimeout = time.Second
	require.NoError(t, r.Reconcile(bridge(staticRegistry("slow", "192.0.2.1:8080"))))
	initialPurges := gate.purges.Load()
	bad := staticRegistry("bad", "192.0.2.2:8080")
	bad.AuthSecretName = "missing-secret"
	done := make(chan error, 1)
	go func() { done <- r.Reconcile(bridge(staticRegistry("slow", "192.0.2.20:8080"), bad)) }()
	defer gate.unblock()
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("replacement watcher did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("returned before sibling publication: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	require.Equal(t, initialPurges, gate.purges.Load())
	require.Equal(t, map[string]string{"slow.static": "192.0.2.1"}, serviceAddresses(r.Cache))
	gate.unblock()
	require.ErrorContains(t, <-done, "Init Watchers failed")
	require.Equal(t, map[string]string{"slow.static": "192.0.2.20"}, serviceAddresses(r.Cache))
}

func TestReconcileSuccessfulReplacement(t *testing.T) {
	r := NewReconciler(func() {}, kube.NewFakeClient(), "higress-system", "test")
	require.NoError(t, r.Reconcile(bridge(staticRegistry("backend", "192.0.2.1:8080"))))
	oldWatcher := r.watchers["static/backend"]
	require.NoError(t, r.Reconcile(bridge(staticRegistry("backend", "192.0.2.2:8080"))))
	require.NotSame(t, oldWatcher, r.watchers["static/backend"])
	require.False(t, oldWatcher.IsReady())
	require.Equal(t, map[string]string{"backend.static": "192.0.2.2"}, serviceAddresses(r.Cache))
	require.False(t, r.PurgeStaleItems())
}

func serviceAddresses(cache memory.Cache) map[string]string {
	result := make(map[string]string)
	for _, service := range cache.GetAllServiceEntry() {
		result[service.Hosts[0]] = service.Endpoints[0].Address
	}
	return result
}

func httpProxy(name, address string) *apiv1.ProxyConfig {
	return &apiv1.ProxyConfig{Type: "http", Name: name, ServerAddress: address, ServerPort: 3128, ListenerPort: 10001}
}

// Gate a real direct watcher's cache publication, without replacing its factory,
// Run, Stop, readiness callback, or the underlying cache implementation.
type publicationGate struct {
	memory.Cache
	entered     chan struct{}
	release     chan struct{}
	enterOnce   sync.Once
	releaseOnce sync.Once
	purges      atomic.Int32
}

func (g *publicationGate) ForRegistry(registryType, registryName string) memory.Cache {
	return &gatedRegistryCache{Cache: g.Cache.ForRegistry(registryType, registryName), gate: g}
}

func (g *publicationGate) PurgeStaleItems() bool {
	g.purges.Add(1)
	return g.Cache.PurgeStaleItems()
}

func (g *publicationGate) unblock() { g.releaseOnce.Do(func() { close(g.release) }) }

type gatedRegistryCache struct {
	memory.Cache
	gate *publicationGate
}

func (g *gatedRegistryCache) UpdateServiceWrapper(host string, service *ingress.ServiceWrapper) {
	if host == "slow.static" && service.ServiceEntry.Endpoints[0].Address == "192.0.2.20" {
		g.gate.enterOnce.Do(func() { close(g.gate.entered) })
		<-g.gate.release
	}
	g.Cache.UpdateServiceWrapper(host, service)
}

func TestReconcileFailedReplacementPreservesOtherRegistryOwner(t *testing.T) {
	for _, owner := range []struct{ registryType, name string }{{"static", "other"}, {"dns", "backend"}} {
		t.Run(owner.registryType+"/"+owner.name, func(t *testing.T) {
			r := NewReconciler(func() {}, kube.NewFakeClient(), "higress-system", "test")
			require.NoError(t, r.Reconcile(bridge(staticRegistry("backend", "192.0.2.1:8080"))))
			// Dynamic registries can collide on the same host. Publish a different
			// registry's value before the real direct watcher's Stop deletes it.
			other := r.GetAllServiceWrapper()[0]
			other.RegistryType, other.RegistryName = owner.registryType, owner.name
			other.ServiceEntry.Endpoints[0].Address = "192.0.2.2"
			r.UpdateServiceWrapper("backend.static", other)
			bad := staticRegistry("backend", "192.0.2.3:8080")
			bad.AuthSecretName = "missing-secret"
			require.ErrorContains(t, r.Reconcile(bridge(bad)), "Init Watchers failed")
			require.Equal(t, map[string]string{"backend.static": "192.0.2.2"}, serviceAddresses(r.Cache))
			require.Empty(t, r.watchers)
			require.False(t, r.PurgeStaleItems())
		})
	}
}
