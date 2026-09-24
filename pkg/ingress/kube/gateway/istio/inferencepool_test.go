// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package istio

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	inferencev1 "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/gateway/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/test/util/assert"
	"istio.io/istio/pkg/util/sets"
)

func TestReconcileInferencePool(t *testing.T) {
	test.SetForTest(t, &features.EnableGatewayAPIInferenceExtension, true)
	pool := &inferencev1.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: inferencev1.InferencePoolSpec{
			TargetPorts: []inferencev1.Port{
				{
					Number: inferencev1.PortNumber(8080),
				},
				{
					Number: inferencev1.PortNumber(8081),
				},
				{
					Number: inferencev1.PortNumber(8082),
				},
			},
			Selector: inferencev1.LabelSelector{
				MatchLabels: map[inferencev1.LabelKey]inferencev1.LabelValue{
					"app": "test",
				},
			},
			EndpointPickerRef: inferencev1.EndpointPickerRef{
				Name: "dummy",
				Port: &inferencev1.Port{
					Number: inferencev1.PortNumber(5421),
				},
			},
		},
	}
	controller := setupController(t,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		NewGateway("test-gw", InNamespace(DefaultTestNS), WithGatewayClass("istio")),
		NewHTTPRoute("test-route", InNamespace(DefaultTestNS),
			WithParentRefAndStatus("test-gw", DefaultTestNS, IstioController),
			WithBackendRef("test-pool", DefaultTestNS),
		),
		pool,
	)

	dumpOnFailure(t, krt.GlobalDebugHandler)

	// Verify the service was created
	var service *corev1.Service
	var err error
	assert.EventuallyEqual(t, func() bool {
		svcName := "test-pool-ip-" + generateHash("test-pool", hashSize)
		service, err = controller.client.Kube().CoreV1().Services("default").Get(t.Context(), svcName, metav1.GetOptions{})
		if err != nil {
			t.Logf("Service %s not found yet: %v", svcName, err)
			return false
		}
		return service != nil
	}, true)

	assert.Equal(t, service.ObjectMeta.Labels[constants.InternalServiceSemantics], constants.ServiceSemanticsInferencePool)
	assert.Equal(t, service.ObjectMeta.Labels[InferencePoolRefLabel], pool.Name)
	assert.Equal(t, service.OwnerReferences[0].Name, pool.Name)
	assert.Equal(t, len(service.Spec.Ports), 3)
	for i, servicePort := range service.Spec.Ports {
		assert.Equal(t, servicePort.Name, fmt.Sprintf("http-%d", i))
		assert.Equal(t, servicePort.Port, int32(54321+i))
		assert.Equal(t, servicePort.TargetPort.IntVal, int32(8080+i))
	}
}

func TestInferencePoolEndpointPickerRefModes(t *testing.T) {
	builtinGroup := inferencev1.Group(kube.BuiltinInferenceEndpointPickerGroup)
	coreGroup := inferencev1.Group("")
	tests := []struct {
		name      string
		ref       inferencev1.EndpointPickerRef
		wantMode  kube.InferencePoolEndpointPickerMode
		wantError bool
	}{
		{name: "default core Service", ref: inferencev1.EndpointPickerRef{Name: "epp"}, wantMode: kube.InferencePoolEndpointPickerModeExternal},
		{name: "explicit core Service", ref: inferencev1.EndpointPickerRef{Group: &coreGroup, Kind: "Service", Name: "epp"}, wantMode: kube.InferencePoolEndpointPickerModeExternal},
		{name: "well-known BuiltIn", ref: inferencev1.EndpointPickerRef{Group: &builtinGroup, Kind: "WasmPlugin", Name: "ai-endpoint-picker"}, wantMode: kube.InferencePoolEndpointPickerModeBuiltin},
		{name: "near-match group", ref: inferencev1.EndpointPickerRef{Group: &coreGroup, Kind: "WasmPlugin", Name: "ai-endpoint-picker"}, wantMode: kube.InferencePoolEndpointPickerModeExternal, wantError: true},
		{name: "near-match kind", ref: inferencev1.EndpointPickerRef{Group: &builtinGroup, Kind: "Service", Name: "ai-endpoint-picker"}, wantMode: kube.InferencePoolEndpointPickerModeExternal, wantError: true},
		{name: "near-match name", ref: inferencev1.EndpointPickerRef{Group: &builtinGroup, Kind: "WasmPlugin", Name: "custom"}, wantMode: kube.InferencePoolEndpointPickerModeExternal, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, message := inferencePoolEndpointPickerMode(tt.ref)
			assert.Equal(t, mode, tt.wantMode)
			assert.Equal(t, message != "", tt.wantError)
			if tt.wantError {
				pool := &inferencev1.InferencePool{Spec: inferencev1.InferencePoolSpec{EndpointPickerRef: tt.ref}}
				if got := createInferencePoolObject(pool, sets.New[types.NamespacedName]()); got != nil {
					t.Fatalf("unsupported ref created an InferencePool object: %+v", got)
				}
				resolved := calculateResolvedRefsStatus(pool, nil)
				assert.Equal(t, resolved.status, metav1.ConditionFalse)
				assert.Equal(t, resolved.reason, string(inferencev1.InferencePoolReasonInvalidExtensionRef))
			}
		})
	}
}

func TestBuiltinInferencePoolDoesNotResolveWasmOrUseServiceOptions(t *testing.T) {
	builtinGroup := inferencev1.Group(kube.BuiltinInferenceEndpointPickerGroup)
	pool := &inferencev1.InferencePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "default"},
		Spec: inferencev1.InferencePoolSpec{EndpointPickerRef: inferencev1.EndpointPickerRef{
			Group: &builtinGroup, Kind: "WasmPlugin", Name: "ai-endpoint-picker",
			Port: &inferencev1.Port{Number: 9002}, FailureMode: inferencev1.EndpointPickerFailOpen,
		}},
	}
	object := createInferencePoolObject(pool, sets.New(types.NamespacedName{Name: "gateway", Namespace: "default"}))
	if object == nil || object.extRef.mode != kube.InferencePoolEndpointPickerModeBuiltin ||
		object.extRef.name != "" || object.extRef.port != 0 || object.extRef.failureMode != "" {
		t.Fatalf("unexpected BuiltIn collection result: %+v", object)
	}
	resolved := calculateResolvedRefsStatus(pool, nil)
	assert.Equal(t, resolved.status, metav1.ConditionTrue)
	assert.Equal(t, resolved.reason, string(inferencev1.InferencePoolReasonResolvedRefs))

	shadow := shadowServiceInfo{key: types.NamespacedName{Name: "pool-ip", Namespace: "default"}, poolName: "pool"}
	service := translateShadowServiceToService(map[string]string{
		InferencePoolExtensionRefSvc:         "stale-epp",
		InferencePoolExtensionRefPort:        "9002",
		InferencePoolExtensionRefFailureMode: string(inferencev1.EndpointPickerFailOpen),
	}, shadow, object.extRef)
	assert.Equal(t, service.Labels[constants.InferencePoolEndpointPickerModeLabel], string(kube.InferencePoolEndpointPickerModeBuiltin))
	for _, label := range []string{InferencePoolExtensionRefSvc, InferencePoolExtensionRefPort, InferencePoolExtensionRefFailureMode} {
		if _, found := service.Labels[label]; found {
			t.Fatalf("BuiltIn shadow Service retained External-only label %q: %v", label, service.Labels)
		}
	}

	externalRef := extRefInfo{
		mode:        kube.InferencePoolEndpointPickerModeExternal,
		name:        "epp",
		port:        9002,
		failureMode: string(inferencev1.EndpointPickerFailOpen),
	}
	externalLabels := translateShadowServiceToService(nil, shadow, externalRef).Labels
	if _, found := externalLabels[constants.InferencePoolEndpointPickerModeLabel]; found {
		t.Fatalf("External shadow Service gained a mode label: %v", externalLabels)
	}
	assert.Equal(t, externalLabels[InferencePoolExtensionRefSvc], "epp")
	transitionLabels := translateShadowServiceToService(service.Labels, shadow, externalRef).Labels
	if _, found := transitionLabels[constants.InferencePoolEndpointPickerModeLabel]; found {
		t.Fatalf("External shadow Service retained the BuiltIn mode label: %v", transitionLabels)
	}
	assert.Equal(t, transitionLabels[InferencePoolExtensionRefSvc], "epp")
	assert.Equal(t, transitionLabels[InferencePoolExtensionRefPort], "9002")
	assert.Equal(t, transitionLabels[InferencePoolExtensionRefFailureMode], string(inferencev1.EndpointPickerFailOpen))
}
