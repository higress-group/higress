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

package util

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	networking "istio.io/api/networking/v1alpha3"
)

func mustJSON(t *testing.T, spec any) string {
	t.Helper()
	jsonBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}
	return string(jsonBytes)
}

// TestDefuseSpecTemplatesFailsClosed pins the requirement that a spec which cannot be
// round-tripped is not allowed through. Failing open here would leave the references live and
// ProcessConfig would resolve them against whatever namespace the generated config carries,
// which is the cross-namespace secret read the whole check exists to prevent.
func TestDefuseSpecTemplatesFailsClosed(t *testing.T) {
	t.Run("a spec that cannot be serialized is rejected", func(t *testing.T) {
		// NaN is rejected by the protobuf JSON serializer. This is a real shape rather than a
		// synthetic one: structpb backs WasmPlugin PluginConfig, whose numbers come from
		// plugin configuration.
		spec := &structpb.Struct{Fields: map[string]*structpb.Value{
			"threshold": {Kind: &structpb.Value_NumberValue{NumberValue: math.NaN()}},
		}}
		refused, err := DefuseSpecTemplates(spec, "tenant-a", "test config")
		if err == nil {
			t.Fatal("DefuseSpecTemplates() on an unserializable spec returned no error")
		}
		if refused != nil {
			t.Errorf("DefuseSpecTemplates() refused = %v, want none: nothing was inspected", refused)
		}
		if !strings.Contains(err.Error(), "test config") {
			t.Errorf("error %q does not name the config that was dropped", err)
		}
		if got := spec.Fields["threshold"].GetNumberValue(); !math.IsNaN(got) {
			t.Errorf("spec was modified to %v, want it left alone for the caller to discard", got)
		}
	})

	t.Run("a spec that is not a protobuf message is rejected", func(t *testing.T) {
		// Nothing can be inspected in a value that is not a proto, so it must not pass through
		// on the strength of the check having been called.
		for name, spec := range map[string]any{
			"nil":          nil,
			"plain struct": &struct{ Name string }{Name: "${secret.higress-system/creds.token}"},
			"string":       "${secret.higress-system/creds.token}",
			"map":          map[string]string{"name": "${secret.higress-system/creds.token}"},
		} {
			if _, err := DefuseSpecTemplates(spec, "tenant-a", "test config"); err == nil {
				t.Errorf("DefuseSpecTemplates(%s) returned no error, want a rejected spec", name)
			}
		}
	})
}

// TestDefuseSpecTemplatesLeavesCachedValuesAlone pins the property that lets a caller defuse a
// spec assembled from values that are already in a cache. The Gateway API backend policy merge
// builds one DestinationRule out of pointers taken from cached BackendPolicy values, so an
// in-place rewrite that reached through those pointers would mutate the cache.
func TestDefuseSpecTemplatesLeavesCachedValuesAlone(t *testing.T) {
	cachedTLS := &networking.ClientTLSSettings{Sni: "${secret.higress-system/creds.token}"}
	cachedCookie := &networking.LoadBalancerSettings_ConsistentHashLB_HTTPCookie{
		Name: "${secret.higress-system/creds.token}",
	}
	spec := &networking.DestinationRule{
		Host: "reviews.tenant-a.svc.cluster.local",
		TrafficPolicy: &networking.TrafficPolicy{
			Tls: cachedTLS,
			LoadBalancer: &networking.LoadBalancerSettings{
				LbPolicy: &networking.LoadBalancerSettings_ConsistentHash{
					ConsistentHash: &networking.LoadBalancerSettings_ConsistentHashLB{
						HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_HttpCookie{HttpCookie: cachedCookie},
					},
				},
			},
		},
	}

	if _, err := DefuseSpecTemplates(spec, "tenant-a", "DestinationRule for test"); err != nil {
		t.Fatalf("DefuseSpecTemplates() error = %v", err)
	}

	if got := cachedTLS.GetSni(); got != "${secret.higress-system/creds.token}" {
		t.Errorf("cached ClientTLSSettings.Sni = %q, was mutated through the generated spec", got)
	}
	if got := cachedCookie.GetName(); got != "${secret.higress-system/creds.token}" {
		t.Errorf("cached HttpCookie.Name = %q, was mutated through the generated spec", got)
	}
	// The generated spec itself carries the restricted values, and no longer shares them.
	if got := spec.GetTrafficPolicy().GetTls().GetSni(); got != RefusedReferencePlaceholder {
		t.Errorf("spec Tls.Sni = %q, want %q", got, RefusedReferencePlaceholder)
	}
	if spec.GetTrafficPolicy().GetTls() == cachedTLS {
		t.Error("spec still points at the cached ClientTLSSettings after the rewrite")
	}
}

// TestDefuseSpecTemplatesPreservesUnrelatedFields guards the round trip itself. The rewrite
// serializes the whole spec and deserializes it again, so a field that the protobuf JSON
// mapping represents differently from its Go type would be silently corrupted every time a
// tenant writes a reference anywhere in it.
func TestDefuseSpecTemplatesPreservesUnrelatedFields(t *testing.T) {
	spec := &networking.DestinationRule{
		Host:     "reviews.tenant-a.svc.cluster.local",
		ExportTo: []string{".", "tenant-b"},
		TrafficPolicy: &networking.TrafficPolicy{
			ConnectionPool: &networking.ConnectionPoolSettings{
				Tcp: &networking.ConnectionPoolSettings_TCPSettings{
					MaxConnections: 1024,
					ConnectTimeout: durationpb.New(2500 * time.Millisecond),
				},
				Http: &networking.ConnectionPoolSettings_HTTPSettings{
					Http2MaxRequests:         77,
					MaxRequestsPerConnection: 3,
					H2UpgradePolicy:          networking.ConnectionPoolSettings_HTTPSettings_UPGRADE,
				},
			},
			LoadBalancer: &networking.LoadBalancerSettings{
				LbPolicy: &networking.LoadBalancerSettings_ConsistentHash{
					ConsistentHash: &networking.LoadBalancerSettings_ConsistentHashLB{
						HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_HttpCookie{
							HttpCookie: &networking.LoadBalancerSettings_ConsistentHashLB_HTTPCookie{
								Name: "${secret.higress-system/creds.token}",
								Path: "/session",
								Ttl:  durationpb.New(90 * time.Second),
							},
						},
						MinimumRingSize: 1024,
					},
				},
			},
			PortLevelSettings: []*networking.TrafficPolicy_PortTrafficPolicy{{
				Port: &networking.PortSelector{Number: 8080},
			}},
		},
		Subsets: []*networking.Subset{{
			Name:   "canary",
			Labels: map[string]string{"version": "v2"},
		}},
	}
	before := mustJSON(t, spec)

	refused, err := DefuseSpecTemplates(spec, "tenant-a", "DestinationRule for test")
	if err != nil {
		t.Fatalf("DefuseSpecTemplates() error = %v", err)
	}
	if len(refused) != 1 {
		t.Errorf("DefuseSpecTemplates() refused %v, want exactly the foreign cookie name", refused)
	}

	cookie := spec.GetTrafficPolicy().GetLoadBalancer().GetConsistentHash().GetHttpCookie()
	if cookie.GetName() != RefusedReferencePlaceholder {
		t.Fatalf("HttpCookie.Name = %q, want %q", cookie.GetName(), RefusedReferencePlaceholder)
	}
	// Everything else has to survive the round trip field for field.
	cookie.Name = "${secret.higress-system/creds.token}"
	if after := mustJSON(t, spec); after != before {
		t.Errorf("round trip changed unrelated fields:\nbefore: %s\nafter:  %s", before, after)
	}
}
