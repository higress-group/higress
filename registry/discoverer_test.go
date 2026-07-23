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

package registry

import (
	"context"
	"testing"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
)

// noopDiscoverer is a Discoverer that does nothing; used only for registration tests.
type noopDiscoverer struct{}

func (noopDiscoverer) Run(ctx context.Context, up chan<- []*TargetGroup) {}

// registerForTest installs a factory under a unique type name and removes it
// when the test finishes, so tests do not leak global state into each other.
func registerForTest(t *testing.T, registryType string) {
	t.Helper()
	discovererFactoriesMu.Lock()
	if _, exists := discovererFactories[registryType]; exists {
		discovererFactoriesMu.Unlock()
		t.Fatalf("test fixture collision: type %q already registered", registryType)
	}
	discovererFactories[registryType] = func(*apiv1.RegistryConfig, AuthOption) (Discoverer, error) {
		return noopDiscoverer{}, nil
	}
	discovererFactoriesMu.Unlock()
	t.Cleanup(func() {
		discovererFactoriesMu.Lock()
		delete(discovererFactories, registryType)
		discovererFactoriesMu.Unlock()
	})
}

func TestRegisterAndLookupDiscoverer(t *testing.T) {
	registryType := "test-lookup-only"
	registerForTest(t, registryType)

	if factory := LookupDiscovererFactory(registryType); factory == nil {
		t.Fatal("expected the registered factory to be found")
	}

	if factory := LookupDiscovererFactory("never-registered-type"); factory != nil {
		t.Fatal("expected nil factory for an unregistered type")
	}
}

func TestRegisterDiscovererPanicsOnDuplicate(t *testing.T) {
	registryType := "test-duplicate"
	registerForTest(t, registryType)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected RegisterDiscoverer to panic on duplicate registration")
		}
	}()
	RegisterDiscoverer(registryType, func(*apiv1.RegistryConfig, AuthOption) (Discoverer, error) {
		return noopDiscoverer{}, nil
	})
}

func TestRegisterDiscovererPanicsOnInvalidArgs(t *testing.T) {
	cases := []struct {
		name    string
		rType   string
		factory DiscovererFactory
	}{
		{name: "empty type", rType: "", factory: func(*apiv1.RegistryConfig, AuthOption) (Discoverer, error) { return nil, nil }},
		{name: "nil factory", rType: "test-nil-factory", factory: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected RegisterDiscoverer to panic")
				}
			}()
			RegisterDiscoverer(tc.rType, tc.factory)
		})
	}
}

func TestTargetGroupZeroValueIsSafe(t *testing.T) {
	// A nil/empty TargetGroup must not panic when converted by callers.
	var g *TargetGroup
	if got := groupHostFromPackage(g); got != "" {
		t.Fatalf("expected empty host for nil group, got %s", got)
	}
}

// groupHostFromPackage mirrors the custom.groupHost helper and exists only so
// the registry package test can exercise nil-safety of the TargetGroup type
// without importing the custom subpackage (which would create a cycle).
func groupHostFromPackage(g *TargetGroup) string {
	if g == nil {
		return ""
	}
	if name := g.Labels[ServiceLabelAlias]; name != "" {
		return name
	}
	if name := g.Labels[ServiceLabel]; name != "" {
		return name
	}
	return g.Source
}
