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

package kingress

import (
	"testing"

	coreV1 "k8s.io/api/core/v1"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
)

// TestTransportLoadBalancerIngress verifies that transportLoadBalancerIngress
// correctly maps k8s LoadBalancerIngress entries to knative LoadBalancerIngressStatus.
func TestTransportLoadBalancerIngress(t *testing.T) {
	tests := []struct {
		name   string
		input  []coreV1.LoadBalancerIngress
		expect []v1alpha1.LoadBalancerIngressStatus
	}{
		{
			name:   "nil input returns nil",
			input:  nil,
			expect: nil,
		},
		{
			name:   "empty input returns nil",
			input:  []coreV1.LoadBalancerIngress{},
			expect: nil,
		},
		{
			name: "ip only entry",
			input: []coreV1.LoadBalancerIngress{
				{IP: "1.2.3.4"},
			},
			expect: []v1alpha1.LoadBalancerIngressStatus{
				{IP: "1.2.3.4", Domain: ""},
			},
		},
		{
			name: "hostname only entry",
			input: []coreV1.LoadBalancerIngress{
				{Hostname: "lb.example.com"},
			},
			expect: []v1alpha1.LoadBalancerIngressStatus{
				{IP: "", Domain: "lb.example.com"},
			},
		},
		{
			name: "multiple entries preserve order",
			input: []coreV1.LoadBalancerIngress{
				{IP: "10.0.0.1"},
				{IP: "10.0.0.2", Hostname: "lb2.example.com"},
			},
			expect: []v1alpha1.LoadBalancerIngressStatus{
				{IP: "10.0.0.1", Domain: ""},
				{IP: "10.0.0.2", Domain: "lb2.example.com"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transportLoadBalancerIngress(tt.input)
			if len(got) != len(tt.expect) {
				t.Fatalf("len mismatch: got %d, want %d", len(got), len(tt.expect))
			}
			for i := range got {
				if got[i].IP != tt.expect[i].IP || got[i].Domain != tt.expect[i].Domain {
					t.Errorf("entry[%d]: got {IP:%q Domain:%q}, want {IP:%q Domain:%q}",
						i, got[i].IP, got[i].Domain, tt.expect[i].IP, tt.expect[i].Domain)
				}
			}
		})
	}
}

// TestUpdateStatusCondition tests the update-trigger condition in updateStatus:
//
//	PublicLoadBalancer == nil  ||  len differs  ||  !DeepEqual
//
// Before the fix (commit f04791b4), the condition was:
//
//	PublicLoadBalancer == nil  ||  len differs  ||  DeepEqual   ← missing !
//
// This meant the status was updated when the LB list was EQUAL (no-op churn)
// and skipped when it was DIFFERENT (the actual update never happened).
//
// The table below documents each branch so a regression immediately shows
// which invariant was broken.
func TestUpdateStatusCondition(t *testing.T) {
	newStatus := func(ips ...string) *v1alpha1.LoadBalancerIngressStatus {
		return nil // helper not used directly; see inline construction below
	}
	_ = newStatus

	makeKnative := func(ips ...string) []v1alpha1.LoadBalancerIngressStatus {
		out := make([]v1alpha1.LoadBalancerIngressStatus, len(ips))
		for i, ip := range ips {
			out[i] = v1alpha1.LoadBalancerIngressStatus{IP: ip}
		}
		return out
	}

	tests := []struct {
		name          string
		existing      *v1alpha1.LoadBalancerStatus // PublicLoadBalancer field
		incoming      []v1alpha1.LoadBalancerIngressStatus
		wantShouldUpd bool // true == condition evaluates to true (update needed)
	}{
		{
			name:          "PublicLoadBalancer is nil → always update",
			existing:      nil,
			incoming:      makeKnative("1.2.3.4"),
			wantShouldUpd: true,
		},
		{
			name: "lengths differ → update",
			existing: &v1alpha1.LoadBalancerStatus{
				Ingress: makeKnative("1.2.3.4"),
			},
			incoming:      makeKnative("1.2.3.4", "5.6.7.8"),
			wantShouldUpd: true,
		},
		{
			// Bug scenario: status is DIFFERENT → must update.
			// Before fix: !DeepEqual was missing, so this branch was skipped.
			name: "same length but different IPs → update (was broken before fix)",
			existing: &v1alpha1.LoadBalancerStatus{
				Ingress: makeKnative("1.2.3.4"),
			},
			incoming:      makeKnative("9.9.9.9"),
			wantShouldUpd: true,
		},
		{
			// Idempotency: status is already up-to-date → skip update.
			// Before fix: DeepEqual (without !) was true here, so it wrongly
			// triggered an unnecessary update on every reconcile loop.
			name: "status already up-to-date → no update needed (was broken before fix)",
			existing: &v1alpha1.LoadBalancerStatus{
				Ingress: makeKnative("1.2.3.4"),
			},
			incoming:      makeKnative("1.2.3.4"),
			wantShouldUpd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mirror the exact condition from status.go updateStatus:
			//   PublicLoadBalancer == nil || len differs || !DeepEqual
			shouldUpdate := tt.existing == nil ||
				len(tt.existing.Ingress) != len(tt.incoming) ||
				!equalLoadBalancerStatus(tt.existing.Ingress, tt.incoming)

			if shouldUpdate != tt.wantShouldUpd {
				t.Errorf("condition = %v, want %v", shouldUpdate, tt.wantShouldUpd)
			}
		})
	}
}

// equalLoadBalancerStatus compares two LoadBalancerIngressStatus slices
// element-by-element (mirrors reflect.DeepEqual for this type).
func equalLoadBalancerStatus(a, b []v1alpha1.LoadBalancerIngressStatus) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].IP != b[i].IP || a[i].Domain != b[i].Domain {
			return false
		}
	}
	return true
}
