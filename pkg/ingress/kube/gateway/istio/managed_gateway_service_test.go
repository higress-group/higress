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

package istio

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	networking "istio.io/api/networking/v1alpha3"
)

func TestManagedGatewayServiceName(t *testing.T) {
	if got, want := managedGatewayServiceName("default", "example"), "default-example-higress"; got != want {
		t.Fatalf("managedGatewayServiceName() = %q, want %q", got, want)
	}

	got := managedGatewayServiceName(strings.Repeat("n", 63), strings.Repeat("g", 253))
	if len(got) != 63 {
		t.Fatalf("long managed Gateway Service name length = %d, want 63", len(got))
	}
	if got != managedGatewayServiceName(strings.Repeat("n", 63), strings.Repeat("g", 253)) {
		t.Fatal("managed Gateway Service name is not deterministic")
	}
}

func TestSetGatewayServiceTargetPorts(t *testing.T) {
	service := &corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
		{Port: 80, NodePort: 31080, TargetPort: intstr.FromInt(80)},
		{Port: 443, TargetPort: intstr.FromInt(443)},
	}}}

	if !setGatewayServiceTargetPorts(service) {
		t.Fatal("setGatewayServiceTargetPorts() did not report the target port update")
	}
	if got, want := service.Spec.Ports[0].TargetPort.IntVal, int32(31080); got != want {
		t.Fatalf("target port = %d, want %d", got, want)
	}
	if got, want := service.Spec.Ports[1].TargetPort.IntVal, int32(443); got != want {
		t.Fatalf("unallocated target port = %d, want %d", got, want)
	}
	if setGatewayServiceTargetPorts(service) {
		t.Fatal("setGatewayServiceTargetPorts() reported an update for an unchanged Service")
	}
}

func TestSetRequestRedirectPort(t *testing.T) {
	routes := []*networking.HTTPRoute{
		{Redirect: &networking.HTTPRedirect{RedirectPort: &networking.HTTPRedirect_DerivePort{
			DerivePort: networking.HTTPRedirect_FROM_REQUEST_PORT,
		}}},
		{Redirect: &networking.HTTPRedirect{RedirectPort: &networking.HTTPRedirect_Port{Port: 8443}}},
		{Redirect: &networking.HTTPRedirect{RedirectPort: &networking.HTTPRedirect_DerivePort{
			DerivePort: networking.HTTPRedirect_FROM_PROTOCOL_DEFAULT,
		}}},
	}

	got := setRequestRedirectPort(routes, 80)
	if port, ok := got[0].Redirect.RedirectPort.(*networking.HTTPRedirect_Port); !ok || port.Port != 80 {
		t.Fatalf("request-derived redirect port = %v, want explicit port 80", got[0].Redirect.RedirectPort)
	}
	if port := got[1].Redirect.GetPort(); port != 8443 {
		t.Fatalf("explicit redirect port = %d, want 8443", port)
	}
	if derive := got[2].Redirect.GetDerivePort(); derive != networking.HTTPRedirect_FROM_PROTOCOL_DEFAULT {
		t.Fatalf("protocol-derived redirect port = %v, want FROM_PROTOCOL_DEFAULT", derive)
	}
	if _, ok := routes[0].Redirect.RedirectPort.(*networking.HTTPRedirect_DerivePort); !ok {
		t.Fatal("setRequestRedirectPort() mutated its input route")
	}
}
