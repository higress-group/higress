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
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8s "sigs.k8s.io/gateway-api/apis/v1"

	higressconstant "github.com/alibaba/higress/v2/pkg/config/constants"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/networking/core"
	"istio.io/istio/pilot/pkg/serviceregistry/kube/controller"
	"istio.io/istio/pilot/pkg/status"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/test/util/assert"
)

var (
	gatewayClassSpec = &k8s.GatewayClassSpec{
		ControllerName: higressconstant.ManagedGatewayController,
	}
	gatewaySpec = &k8s.GatewaySpec{
		GatewayClassName: "higress",
		Listeners: []k8s.Listener{
			{
				Name:          "default",
				Port:          9009,
				Protocol:      "HTTP",
				AllowedRoutes: &k8s.AllowedRoutes{Namespaces: &k8s.RouteNamespaces{From: func() *k8s.FromNamespaces { x := k8s.NamespacesFromAll; return &x }()}},
			},
		},
	}
	httpRouteSpec = &k8s.HTTPRouteSpec{
		CommonRouteSpec: k8s.CommonRouteSpec{ParentRefs: []k8s.ParentReference{{
			Name: "gwspec",
		}}},
		Hostnames: []k8s.Hostname{"test.cluster.local"},
	}

	expectedgw = &networking.Gateway{
		Servers: []*networking.Server{
			{
				Port: &networking.Port{
					Number:   9009,
					Name:     "default",
					Protocol: "HTTP",
				},
				Hosts: []string{"*/*"},
			},
		},
	}
)

var AlwaysReady = func(class schema.GroupVersionResource, stop <-chan struct{}) bool {
	return true
}

func setupController(t *testing.T, objs ...runtime.Object) *Controller {
	setGatewayClassNameForTest(t, "")
	kc := kube.NewFakeClient(objs...)
	setupClientCRDs(t, kc)
	stop := test.NewStop(t)
	controller := NewController(
		kc,
		AlwaysReady,
		controller.Options{KrtDebugger: krt.GlobalDebugHandler},
		nil)
	kc.RunAndWait(stop)
	go controller.Run(stop)
	cg := core.NewConfigGenTest(t, core.TestOptions{})
	controller.Reconcile(cg.PushContext())
	kube.WaitForCacheSync("test", stop, controller.HasSynced)

	return controller
}

func setupControllerWithGatewayClass(t *testing.T, gatewayClass string, objs ...runtime.Object) *Controller {
	setGatewayClassNameForTest(t, gatewayClass)
	kc := kube.NewFakeClient(objs...)
	setupClientCRDs(t, kc)
	stop := test.NewStop(t)
	controller := NewController(
		kc,
		AlwaysReady,
		controller.Options{KrtDebugger: krt.GlobalDebugHandler},
		nil)
	kc.RunAndWait(stop)
	go controller.Run(stop)
	cg := core.NewConfigGenTest(t, core.TestOptions{})
	controller.Reconcile(cg.PushContext())
	kube.WaitForCacheSync("test", stop, controller.HasSynced)

	return controller
}

func setGatewayClassNameForTest(t *testing.T, gatewayClass string) {
	t.Helper()
	if gatewayClass != "" {
		SetGatewayClassName(gatewayClass)
	}
}

func runInGatewayClassSubprocess(t *testing.T) bool {
	t.Helper()
	const env = "HIGRESS_TEST_GATEWAY_CLASS_SUBPROCESS"
	if os.Getenv(env) == t.Name() {
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1")
	cmd.Env = append(testEnvWithoutCoverage(), env+"="+t.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gateway class subprocess failed: %v\n%s", err, out)
	}
	return true
}

func testEnvWithoutCoverage() []string {
	var out []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GOCOVERDIR=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func TestListInvalidGroupVersionKind(t *testing.T) {
	controller := setupController(t)

	typ := config.GroupVersionKind{Kind: "wrong-kind"}
	c := controller.List(typ, "ns1")
	assert.Equal(t, len(c), 0)
}

func TestListGatewayResourceType(t *testing.T) {
	controller := setupController(t,
		&k8s.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "higress",
			},
			Spec: *gatewayClassSpec,
		},
		&k8s.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gwspec",
				Namespace: "ns1",
			},
			Spec: *gatewaySpec,
		},
		&k8s.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "http-route",
				Namespace: "ns1",
			},
			Spec: *httpRouteSpec,
		})

	dumpOnFailure(t, krt.GlobalDebugHandler)
	cfg := controller.List(gvk.Gateway, "ns1")
	assert.Equal(t, len(cfg), 1)
	for _, c := range cfg {
		assert.Equal(t, c.GroupVersionKind, gvk.Gateway)
		assert.Equal(t, c.Name, "gwspec"+"-"+constants.KubernetesGatewayName+"-default")
		assert.Equal(t, c.Namespace, "ns1")
		assert.Equal(t, c.Spec, any(expectedgw))
	}
}

func TestListGatewayResourceTypeWithAlternateGatewayClassName(t *testing.T) {
	alternateGateway := gatewaySpec.DeepCopy()
	alternateGateway.GatewayClassName = "alternate"
	controller := setupController(t,
		&k8s.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "alternate"},
			Spec:       *gatewayClassSpec,
		},
		&k8s.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "alternate-gw", Namespace: "ns1"},
			Spec:       *alternateGateway,
		})

	cfg := controller.List(gvk.Gateway, "ns1")
	assert.Equal(t, len(cfg), 1)
	assert.Equal(t, cfg[0].Name, "alternate-gw-"+constants.KubernetesGatewayName+"-default")
}

func TestHTTPRouteBackendServiceLifecycle(t *testing.T) {
	backendPort := k8s.PortNumber(80)
	route := httpRouteSpec.DeepCopy()
	route.Rules = []k8s.HTTPRouteRule{{
		BackendRefs: []k8s.HTTPBackendRef{{
			BackendRef: k8s.BackendRef{
				BackendObjectReference: k8s.BackendObjectReference{
					Name: "backend",
					Port: &backendPort,
				},
			},
		}},
	}}
	controller := setupController(t,
		&k8s.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "higress"},
			Spec:       *gatewayClassSpec,
		},
		&k8s.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gwspec", Namespace: "ns1"},
			Spec:       *gatewaySpec,
		},
		&k8s.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "http-route", Namespace: "ns1"},
			Spec:       *route,
		},
	)

	statusQueue := &TestStatusQueue{state: map[status.Resource]any{}}
	setAndWait(t, controller, statusQueue)
	resolvedRefs := func() string {
		for _, rawStatus := range statusQueue.Statuses() {
			routeStatus, ok := rawStatus.(*k8s.HTTPRouteStatus)
			if !ok {
				continue
			}
			for _, parent := range routeStatus.Parents {
				for _, condition := range parent.Conditions {
					if condition.Type == string(k8s.RouteConditionResolvedRefs) {
						return string(condition.Status) + "/" + condition.Reason
					}
				}
			}
		}
		return ""
	}
	assert.EventuallyEqual(t, resolvedRefs, "False/BackendNotFound")

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "ns1"},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{
			Name: "http",
			Port: int32(backendPort),
		}}},
	}
	if _, err := controller.client.Kube().CoreV1().Services("ns1").Create(t.Context(), service, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	assert.EventuallyEqual(t, resolvedRefs, "True/ResolvedRefs")

	if err := controller.client.Kube().CoreV1().Services("ns1").Delete(t.Context(), service.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	assert.EventuallyEqual(t, resolvedRefs, "False/BackendNotFound")
}

func TestListGatewayResourceTypeWithCustomGatewayClass(t *testing.T) {
	if runInGatewayClassSubprocess(t) {
		return
	}
	customGatewayClass := "higress-internal"
	customControllerName := higressconstant.ManagedGatewayController + "-" + customGatewayClass
	defaultGateway := gatewaySpec.DeepCopy()
	defaultGateway.GatewayClassName = k8s.ObjectName(higressconstant.DefaultGatewayClass)
	customGateway := gatewaySpec.DeepCopy()
	customGateway.GatewayClassName = k8s.ObjectName(customGatewayClass)

	controller := setupControllerWithGatewayClass(t, customGatewayClass,
		&k8s.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: higressconstant.DefaultGatewayClass,
			},
			Spec: *gatewayClassSpec,
		},
		&k8s.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: customGatewayClass,
			},
			Spec: k8s.GatewayClassSpec{
				ControllerName: k8s.GatewayController(customControllerName),
			},
		},
		&k8s.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default-gw",
				Namespace: "ns1",
			},
			Spec: *defaultGateway,
		},
		&k8s.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "custom-gw",
				Namespace: "ns1",
			},
			Spec: *customGateway,
		})

	dumpOnFailure(t, krt.GlobalDebugHandler)
	cfg := controller.List(gvk.Gateway, "ns1")
	assert.Equal(t, len(cfg), 1)
	assert.Equal(t, cfg[0].Name, "custom-gw"+"-"+constants.KubernetesGatewayName+"-default")
	assert.Equal(t, cfg[0].Namespace, "ns1")
	assert.Equal(t, cfg[0].Spec, any(expectedgw))
}
