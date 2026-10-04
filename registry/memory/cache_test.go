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

package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/api/networking/v1alpha3"

	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
)

func TestRegistryCacheDeletionChecksCurrentOwner(t *testing.T) {
	for _, owner := range []struct{ registryType, registryName string }{
		{"nacos2", "other"},
		{"nacos3", "original"},
	} {
		t.Run(owner.registryType+"/"+owner.registryName, func(t *testing.T) {
			cache := NewCache()
			original := cache.ForRegistry("nacos2", "original")
			current := cache.ForRegistry(owner.registryType, owner.registryName)
			current.UpdateServiceWrapper("shared.nacos", serviceWrapper("shared.nacos", owner.registryType, owner.registryName, "192.0.2.2"))
			original.DeleteServiceWrapper("shared.nacos")
			require.False(t, cache.PurgeStaleItems(), "another registry's Stop must not delete the current owner")
			_, deleted := cache.GetIncrementalServiceWrapper()
			require.Empty(t, deleted, "do not emit a deletion for another registry's current value")
			require.Len(t, cache.GetAllServiceEntry(), 1)
			current.DeleteServiceWrapper("shared.nacos")
			require.True(t, cache.PurgeStaleItems())
			require.Empty(t, cache.GetAllServiceEntry())
		})
	}
}

func TestRegistryCacheSharedHostHandoverOrders(t *testing.T) {
	// All interleavings that retain Stop-before-publication for each registry.
	for _, order := range []string{"aAbB", "abAB", "abBA", "bBaA", "baAB", "baBA"} {
		t.Run(order, func(t *testing.T) {
			cache := NewCache()
			a := cache.ForRegistry("consul", "a")
			b := cache.ForRegistry("consul", "b")
			a.UpdateServiceWrapper("shared.dc.consul", serviceWrapper("shared.dc.consul", "consul", "a", "192.0.2.1"))
			lastPublisher := ""
			for _, event := range order {
				switch event {
				case 'a':
					a.DeleteServiceWrapper("shared.dc.consul")
				case 'b':
					b.DeleteServiceWrapper("shared.dc.consul")
				case 'A':
					a.UpdateServiceWrapper("shared.dc.consul", serviceWrapper("shared.dc.consul", "consul", "a", "192.0.2.10"))
					lastPublisher = "a"
				case 'B':
					b.UpdateServiceWrapper("shared.dc.consul", serviceWrapper("shared.dc.consul", "consul", "b", "192.0.2.20"))
					lastPublisher = "b"
				}
			}
			require.False(t, cache.PurgeStaleItems(), "a fresh publication must cancel its host's pending deletion")
			services := cache.GetAllServiceWrapper()
			require.Len(t, services, 1)
			require.Equal(t, lastPublisher, services[0].RegistryName)
		})
	}
}

func TestRegistryCacheRetainsUnscopedDeletion(t *testing.T) {
	cache := NewCache()
	cache.ForRegistry("static", "a").UpdateServiceWrapper("a.static", serviceWrapper("a.static", "static", "a", "192.0.2.1"))
	cache.DeleteServiceWrapper("a.static")
	require.True(t, cache.PurgeStaleItems())
	require.Empty(t, cache.GetAllServiceEntry())
}

func serviceWrapper(host, registryType, registryName, address string) *ingress.ServiceWrapper {
	return &ingress.ServiceWrapper{
		ServiceName: "shared", RegistryType: registryType, RegistryName: registryName,
		ServiceEntry: &v1alpha3.ServiceEntry{Hosts: []string{host}, Endpoints: []*v1alpha3.WorkloadEntry{{Address: address}}},
	}
}
