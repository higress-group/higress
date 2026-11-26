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

package memory

import (
	"testing"

	"istio.io/api/networking/v1alpha3"

	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
)

// Scenario 1: a proxy revived by UpdateProxyWrapper must survive PurgeStaleItems.
// The buggy code cancels the deferred-delete registration in deferredDeleteServices
// instead of deferredDeleteProxies, so the revived proxy gets purged.
func TestUpdateProxyWrapperCancelsDeferredDelete(t *testing.T) {
	c := NewCache()
	pw := &ingress.ProxyWrapper{ProxyName: "p1"}
	c.UpdateProxyWrapper("p1", pw)
	c.DeleteProxyWrapper("p1")     // registers deferredDeleteProxies["p1"]
	c.UpdateProxyWrapper("p1", pw) // revival, should cancel the registration
	if c.PurgeStaleItems() {
		t.Fatalf("revived proxy should not be purged")
	}
	if n := len(c.GetAllProxyWrapper()); n != 1 {
		t.Fatalf("revived proxy purged from cache, got %d, want 1", n)
	}
}

// Scenario 2: updating a proxy must not cancel the deferred-delete registration
// of a same-named service. The buggy code checks deferredDeleteServices with the
// proxy name, so a deleted service survives PurgeStaleItems and its ServiceEntry
// keeps being pushed.
func TestUpdateProxyWrapperMustNotTouchServiceDeferredDelete(t *testing.T) {
	c := NewCache()
	sw := &ingress.ServiceWrapper{
		ServiceName:  "s1",
		ServiceEntry: &v1alpha3.ServiceEntry{Hosts: []string{"s1.com"}},
	}
	c.UpdateServiceWrapper("s1", sw)
	c.DeleteServiceWrapper("s1") // registers deferredDeleteServices["s1"]
	c.UpdateProxyWrapper("s1", &ingress.ProxyWrapper{ProxyName: "s1"})
	// The service must be purged; whether PurgeStaleItems returns true only
	// tells that something was purged, so only assert the observable cache state.
	c.PurgeStaleItems()
	if n := len(c.GetAllServiceEntry()); n != 0 {
		t.Fatalf("deleted service survived purge, got %d entries, want 0", n)
	}
}
