// Copyright (c) 2022 Alibaba Group Holding Ltd.
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

package custom

import (
	"context"
	"sync"
	"testing"
	"time"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	"github.com/alibaba/higress/v2/pkg/common"
	"github.com/alibaba/higress/v2/registry"
	"github.com/alibaba/higress/v2/registry/memory"

	"istio.io/api/networking/v1alpha3"
)

// fakeDiscoverer is a controllable Discoverer used by the adapter tests.
type fakeDiscoverer struct {
	mu     sync.Mutex
	ch     chan []*registry.TargetGroup
	ctx    context.Context
	sent   int
	closed bool
}

func newFakeDiscoverer() *fakeDiscoverer {
	return &fakeDiscoverer{ch: make(chan []*registry.TargetGroup, 16)}
}

func (f *fakeDiscoverer) Run(ctx context.Context, up chan<- []*registry.TargetGroup) {
	f.mu.Lock()
	f.ctx = ctx
	f.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return
		case groups, ok := <-f.ch:
			if !ok {
				return
			}
			up <- groups
			f.mu.Lock()
			f.sent++
			f.mu.Unlock()
		}
	}
}

func (f *fakeDiscoverer) send(groups []*registry.TargetGroup) { f.ch <- groups }

func (f *fakeDiscoverer) sentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent
}

func TestToServiceWrapper_BasicConversion(t *testing.T) {
	group := &registry.TargetGroup{
		Targets: []registry.LabelSet{
			{
				registry.InstanceLabel: "10.11.150.1:7870",
				"hostname":             "demo-target-1",
				registry.ProtocolLabel: "grpc",
			},
			{
				registry.InstanceLabel: "10.11.150.4:7870",
				"hostname":             "demo-target-2",
			},
		},
		Labels: registry.LabelSet{
			registry.ServiceLabel: "mysql",
		},
		Source: "src-1",
	}

	sew, host := toServiceWrapper(group, "myreg", "custom")
	if sew == nil {
		t.Fatal("expected a non-nil ServiceWrapper")
	}
	if host != "mysql" {
		t.Fatalf("expected host mysql, got %s", host)
	}
	if sew.ServiceName != "mysql" {
		t.Fatalf("expected ServiceName mysql, got %s", sew.ServiceName)
	}
	if sew.RegistryName != "myreg" || sew.RegistryType != "custom" {
		t.Fatalf("unexpected registry name/type: %s/%s", sew.RegistryName, sew.RegistryType)
	}

	se := sew.ServiceEntry
	if len(se.Hosts) != 1 || se.Hosts[0] != "mysql" {
		t.Fatalf("unexpected hosts: %v", se.Hosts)
	}
	if len(se.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(se.Endpoints))
	}

	ep0 := se.Endpoints[0]
	if ep0.Address != "10.11.150.1" {
		t.Fatalf("expected endpoint address 10.11.150.1, got %s", ep0.Address)
	}
	if port, ok := ep0.Ports[common.GRPC.String()]; !ok || port != 7870 {
		t.Fatalf("expected GRPC port 7870, got %v", ep0.Ports)
	}
	if ep0.Labels["hostname"] != "demo-target-1" {
		t.Fatalf("expected hostname label carried over, got %v", ep0.Labels)
	}
	if _, present := ep0.Labels[registry.InstanceLabel]; present {
		t.Fatalf("the __instance__ label must not leak into endpoint labels")
	}

	// Second endpoint falls back to the default HTTP protocol.
	ep1 := se.Endpoints[1]
	if port, ok := ep1.Ports[common.HTTP.String()]; !ok || port != 7870 {
		t.Fatalf("expected default HTTP port 7870, got %v", ep1.Ports)
	}

	// The service port list is derived from the distinct protocols seen.
	if len(se.Ports) != 2 {
		t.Fatalf("expected 2 service ports, got %d", len(se.Ports))
	}
}

func TestToServiceWrapper_ServiceNameResolution(t *testing.T) {
	cases := []struct {
		name  string
		group *registry.TargetGroup
		host  string
		isNil bool
	}{
		{
			name:  "alias label wins",
			group: &registry.TargetGroup{Labels: registry.LabelSet{registry.ServiceLabel: "a", registry.ServiceLabelAlias: "b"}},
			host:  "b",
		},
		{
			name:  "job label used",
			group: &registry.TargetGroup{Labels: registry.LabelSet{registry.ServiceLabel: "a"}},
			host:  "a",
		},
		{
			name:  "source fallback",
			group: &registry.TargetGroup{Source: "from-source"},
			host:  "from-source",
		},
		{
			name:  "no name returns nil",
			group: &registry.TargetGroup{Targets: []registry.LabelSet{{registry.InstanceLabel: "1.2.3.4:80"}}},
			isNil: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Ensure the group always has at least one usable target so that the
			// nil result is solely due to the missing service name.
			if tc.group.Targets == nil && !tc.isNil {
				tc.group.Targets = []registry.LabelSet{{registry.InstanceLabel: "1.2.3.4:80"}}
			}
			sew, host := toServiceWrapper(tc.group, "reg", "custom")
			if tc.isNil {
				if sew != nil {
					t.Fatalf("expected nil wrapper, got host %s", host)
				}
				return
			}
			if sew == nil {
				t.Fatalf("expected non-nil wrapper")
			}
			if host != tc.host {
				t.Fatalf("expected host %s, got %s", tc.host, host)
			}
		})
	}
}

func TestToServiceWrapper_InvalidInstances(t *testing.T) {
	group := &registry.TargetGroup{
		Labels: registry.LabelSet{registry.ServiceLabel: "svc"},
		Targets: []registry.LabelSet{
			{registry.InstanceLabel: ""},                 // missing
			{registry.InstanceLabel: "no-port"},          // no port
			{registry.InstanceLabel: "1.2.3.4:0"},        // zero port
			{registry.InstanceLabel: "1.2.3.4:70000"},    // out of range
			{registry.InstanceLabel: "fe80::1"},          // bare ipv6
			{registry.InstanceLabel: "1.2.3.4:80"},       // valid
			{registry.InstanceLabel: "[2001:db8::1]:80"}, // valid ipv6
			{registry.InstanceLabel: "[2001:db8::1]"},    // ipv6 missing port
		},
	}

	sew, _ := toServiceWrapper(group, "reg", "custom")
	if sew == nil {
		t.Fatal("expected a wrapper with only the valid targets")
	}
	if len(sew.ServiceEntry.Endpoints) != 2 {
		t.Fatalf("expected 2 valid endpoints, got %d", len(sew.ServiceEntry.Endpoints))
	}
}

func TestWatcher_LifecycleAppliesUpdates(t *testing.T) {
	cache := memory.NewCache()
	fd := newFakeDiscoverer()
	w := &watcher{
		cache:      cache,
		discoverer: fd,
		hosts:      make(map[string]struct{}),
	}

	updates := make(chan struct{}, 16)
	ready := make(chan bool, 16)
	w.AppendServiceUpdateHandler(func() {
		select {
		case updates <- struct{}{}:
		default:
		}
	})
	w.ReadyHandler(func(isReady bool) {
		select {
		case ready <- isReady:
		default:
		}
	})

	done := make(chan struct{})
	go func() {
		w.Run()
		close(done)
	}()

	// Wait until the watcher reports readiness.
	select {
	case r := <-ready:
		if !r {
			t.Fatal("expected the watcher to become ready")
		}
	case <-time.After(time.Second):
		t.Fatal("watcher did not become ready in time")
	}

	// First snapshot: add two services.
	fd.send([]*registry.TargetGroup{
		{Labels: registry.LabelSet{registry.ServiceLabel: "a"}, Targets: []registry.LabelSet{{registry.InstanceLabel: "10.0.0.1:80"}}},
		{Labels: registry.LabelSet{registry.ServiceLabel: "b"}, Targets: []registry.LabelSet{{registry.InstanceLabel: "10.0.0.2:80"}}},
	})
	waitForUpdate(t, updates)
	cache.PurgeStaleItems()

	if got := countServiceWrappers(cache); got != 2 {
		t.Fatalf("expected 2 cached services, got %d", got)
	}

	// Second snapshot: remove "a", keep "b", add "c".
	fd.send([]*registry.TargetGroup{
		{Labels: registry.LabelSet{registry.ServiceLabel: "b"}, Targets: []registry.LabelSet{{registry.InstanceLabel: "10.0.0.2:80"}}},
		{Labels: registry.LabelSet{registry.ServiceLabel: "c"}, Targets: []registry.LabelSet{{registry.InstanceLabel: "10.0.0.3:90"}}},
	})
	waitForUpdate(t, updates)
	cache.PurgeStaleItems()

	hosts := serviceHosts(cache.GetAllServiceEntry())
	for _, want := range []string{"b", "c"} {
		if _, ok := hosts[want]; !ok {
			t.Fatalf("expected service %s to remain, got hosts %v", want, hosts)
		}
	}
	if _, ok := hosts["a"]; ok {
		t.Fatalf("stale service a should have been removed")
	}

	// Stop should clean the cache and exit Run.
	w.Stop()
	cache.PurgeStaleItems()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit after Stop")
	}

	if got := countServiceWrappers(cache); got != 0 {
		t.Fatalf("expected cache to be empty after Stop, got %d", got)
	}
}

func TestNewWatcher_NoFactory(t *testing.T) {
	cache := memory.NewCache()
	_, err := NewWatcher(cache, &apiv1.RegistryConfig{Type: "definitely-not-registered-xyz"}, registry.AuthOption{})
	if err == nil {
		t.Fatal("expected an error when no discoverer factory is registered")
	}
}

func waitForUpdate(t *testing.T, updates <-chan struct{}) {
	t.Helper()
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("did not receive a service update notification in time")
	}
}

func countServiceWrappers(cache memory.Cache) int {
	return len(cache.GetAllServiceWrapper())
}

func serviceHosts(entries []*v1alpha3.ServiceEntry) map[string]bool {
	out := make(map[string]bool)
	for _, se := range entries {
		for _, h := range se.Hosts {
			out[h] = true
		}
	}
	return out
}
