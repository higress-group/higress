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
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8s "sigs.k8s.io/gateway-api/apis/v1"
	k8sbeta "sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/gateway-api/pkg/consts"
	"sigs.k8s.io/yaml"

	istio "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/networking/core"
	"istio.io/istio/pilot/pkg/serviceregistry/kube/controller"
	"istio.io/istio/pilot/pkg/status"
	"istio.io/istio/pilot/test/util"
	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	crdvalidation "istio.io/istio/pkg/config/crd"
	"istio.io/istio/pkg/config/host"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/maps"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/util/sets"
)

var ports = []*model.Port{
	{
		Name:     "http",
		Port:     80,
		Protocol: "HTTP",
	},
	{
		Name:     "tcp",
		Port:     34000,
		Protocol: "TCP",
	},
	{
		Name:     "tcp-other",
		Port:     34001,
		Protocol: "TCP",
	},
}

var services = []*model.Service{
	{
		Attributes: model.ServiceAttributes{
			Name:      "higress-gateway",
			Namespace: "higress-system",
			ClusterExternalAddresses: &model.AddressMap{
				Addresses: map[cluster.ID][]string{
					constants.DefaultClusterName: {"1.2.3.4"},
				},
			},
		},
		Ports: []*model.Port{
			{
				Name:     "http",
				Port:     80,
				Protocol: "HTTP",
			},
			{
				Name:     "https",
				Port:     443,
				Protocol: "HTTPS",
			},
			{
				Name:     "tcp",
				Port:     34000,
				Protocol: "TCP",
			},
			{
				Name:     "tcp-other",
				Port:     34001,
				Protocol: "TCP",
			},
		},
		Hostname: "higress-gateway.higress-system.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "higress-system",
		},
		Ports:    ports,
		Hostname: "example.com",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "httpbin.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
			Labels: map[string]string{
				InferencePoolExtensionRefSvc:         "ext-proc-svc",
				InferencePoolExtensionRefPort:        "9002",
				InferencePoolExtensionRefFailureMode: "FailClose",
			},
		},
		Ports: ports,
		Hostname: host.Name(fmt.Sprintf("%s.default.svc.domain.suffix", func() string {
			name, _ := InferencePoolServiceName("infpool-gen")
			return name
		}())),
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
			Labels: map[string]string{
				InferencePoolExtensionRefSvc:         "ext-proc-svc-2",
				InferencePoolExtensionRefPort:        "9002",
				InferencePoolExtensionRefFailureMode: "FailClose",
			},
		},
		Ports: ports,
		Hostname: host.Name(fmt.Sprintf("%s.default.svc.domain.suffix", func() string {
			name, _ := InferencePoolServiceName("infpool-gen2")
			return name
		}())),
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
			Labels: map[string]string{
				InferencePoolExtensionRefSvc:         "model1-epp",
				InferencePoolExtensionRefPort:        "9002",
				InferencePoolExtensionRefFailureMode: "FailClose",
			},
		},
		Ports: ports,
		Hostname: host.Name(fmt.Sprintf("%s.default.svc.domain.suffix", func() string {
			name, _ := InferencePoolServiceName("infpool-model1")
			return name
		}())),
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
			Labels: map[string]string{
				InferencePoolExtensionRefSvc:         "model2-epp",
				InferencePoolExtensionRefPort:        "9002",
				InferencePoolExtensionRefFailureMode: "FailClose",
			},
		},
		Ports: ports,
		Hostname: host.Name(fmt.Sprintf("%s.default.svc.domain.suffix", func() string {
			name, _ := InferencePoolServiceName("infpool-model2")
			return name
		}())),
	},

	{
		Attributes: model.ServiceAttributes{
			Namespace: "apple",
		},
		Ports:    ports,
		Hostname: "httpbin-apple.apple.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "banana",
		},
		Ports:    ports,
		Hostname: "httpbin-banana.banana.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "httpbin-second.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "httpbin-wildcard.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "foo-svc.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "httpbin-other.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "example.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "echo.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "cert",
		},
		Ports:    ports,
		Hostname: "httpbin.cert.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "service",
		},
		Ports:    ports,
		Hostname: "my-svc.service.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "google.com",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "allowed-1",
		},
		Ports:    ports,
		Hostname: "a-example.allowed-1.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "allowed-2",
		},
		Ports:    ports,
		Hostname: "a-example.allowed-2.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "allowed-1",
		},
		Ports:    ports,
		Hostname: "b-example.allowed-1.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "allowed-1",
		},
		Ports:    ports,
		Hostname: "svc2.allowed-1.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "allowed-2",
		},
		Ports:    ports,
		Hostname: "svc2.allowed-2.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "allowed-1",
		},
		Ports:    ports,
		Hostname: "svc1.allowed-1.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "allowed-2",
		},
		Ports:    ports,
		Hostname: "svc3.allowed-2.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "svc4.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "group-namespace1",
		},
		Ports:    ports,
		Hostname: "httpbin.group-namespace1.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "group-namespace2",
		},
		Ports:    ports,
		Hostname: "httpbin.group-namespace2.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "httpbin-zero.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "higress-system",
		},
		Ports:    ports,
		Hostname: "httpbin.higress-system.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "httpbin-mirror.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "httpbin-foo.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "httpbin-alt.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "higress-system",
		},
		Ports:    ports,
		Hostname: "istiod.higress-system.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "higress-system",
		},
		Ports:    ports,
		Hostname: "echo.higress-system.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "httpbin-bad.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Name:      "echo-1",
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "echo-1.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Name:      "echo-2",
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "echo-2.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Name:      "echo-port",
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "echo-port.default.svc.domain.suffix",
	},
	{
		Attributes: model.ServiceAttributes{
			Name:      "not-found",
			Namespace: "default",
		},
		Ports:    ports,
		Hostname: "not-found.default.svc.domain.suffix",
	},
}

var svcPorts = []corev1.ServicePort{
	{
		Name:     "http",
		Port:     80,
		Protocol: "HTTP",
	},
	{
		Name:     "https",
		Port:     443,
		Protocol: "HTTPS",
	},
	{
		Name:     "tcp",
		Port:     34000,
		Protocol: "TCP",
	},
	{
		Name:     "tcp-other",
		Port:     34001,
		Protocol: "TCP",
	},
}

var (
	// https://github.com/kubernetes/kubernetes/blob/v1.25.4/staging/src/k8s.io/kubectl/pkg/cmd/create/create_secret_tls_test.go#L31
	rsaCertPEM = `-----BEGIN CERTIFICATE-----
MIIB0zCCAX2gAwIBAgIJAI/M7BYjwB+uMA0GCSqGSIb3DQEBBQUAMEUxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTIwOTEyMjE1MjAyWhcNMTUwOTEyMjE1MjAyWjBF
MQswCQYDVQQGEwJBVTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBANLJ
hPHhITqQbPklG3ibCVxwGMRfp/v4XqhfdQHdcVfHap6NQ5Wok/4xIA+ui35/MmNa
rtNuC+BdZ1tMuVCPFZcCAwEAAaNQME4wHQYDVR0OBBYEFJvKs8RfJaXTH08W+SGv
zQyKn0H8MB8GA1UdIwQYMBaAFJvKs8RfJaXTH08W+SGvzQyKn0H8MAwGA1UdEwQF
MAMBAf8wDQYJKoZIhvcNAQEFBQADQQBJlffJHybjDGxRMqaRmDhX0+6v02TUKZsW
r5QuVbpQhH6u+0UgcW0jp9QwpxoPTLTWGXEWBBBurxFwiCBhkQ+V
-----END CERTIFICATE-----
`
	rsaKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIBOwIBAAJBANLJhPHhITqQbPklG3ibCVxwGMRfp/v4XqhfdQHdcVfHap6NQ5Wo
k/4xIA+ui35/MmNartNuC+BdZ1tMuVCPFZcCAwEAAQJAEJ2N+zsR0Xn8/Q6twa4G
6OB1M1WO+k+ztnX/1SvNeWu8D6GImtupLTYgjZcHufykj09jiHmjHx8u8ZZB/o1N
MQIhAPW+eyZo7ay3lMz1V01WVjNKK9QSn1MJlb06h/LuYv9FAiEA25WPedKgVyCW
SmUwbPw8fnTcpqDWE3yTO3vKcebqMSsCIBF3UmVue8YU3jybC3NxuXq3wNm34R8T
xVLHwDXh/6NJAiEAl2oHGGLz64BuAfjKrqwz7qMYr9HCLIe/YsoWq/olzScCIQDi
D2lWusoe2/nEqfDVVWGWlyJ7yOmqaVm/iNUN9B2N2g==
-----END RSA PRIVATE KEY-----
`

	objects = []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cert-http",
				Namespace: "higress-system",
			},
			Data: map[string][]byte{
				"tls.crt": []byte(rsaCertPEM),
				"tls.key": []byte(rsaKeyPEM),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ns2-cert",
				Namespace: "ns2",
			},
			Data: map[string][]byte{
				"tls.crt": []byte(rsaCertPEM),
				"tls.key": []byte(rsaKeyPEM),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ns3-cert",
				Namespace: "ns3",
			},
			Data: map[string][]byte{
				"tls.crt": []byte(rsaCertPEM),
				"tls.key": []byte(rsaKeyPEM),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ns4-cert",
				Namespace: "ns4",
			},
			Data: map[string][]byte{
				"tls.crt": []byte(rsaCertPEM),
				"tls.key": []byte(rsaKeyPEM),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cert-http2",
				Namespace: "higress-system",
			},
			Data: map[string][]byte{
				"tls.crt": []byte(rsaCertPEM),
				"tls.key": []byte(rsaKeyPEM),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cert",
				Namespace: "cert",
			},
			Data: map[string][]byte{
				"tls.crt": []byte(rsaCertPEM),
				"tls.key": []byte(rsaKeyPEM),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "malformed",
				Namespace: "higress-system",
			},
			Data: map[string][]byte{
				// nolint: lll
				// https://github.com/kubernetes-sigs/gateway-api/blob/d7f71d6b7df7e929ae299948973a693980afc183/conformance/tests/gateway-invalid-tls-certificateref.yaml#L87-L90
				// this certificate is invalid because contains an invalid pem (base64 of "Hello world"),
				// and the certificate and the key are identical
				"tls.crt": []byte("SGVsbG8gd29ybGQK"),
				"tls.key": []byte("SGVsbG8gd29ybGQK"),
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "malformed",
				Namespace: "higress-system",
			},
			Data: map[string]string{
				"not-ca.crt": "hello",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "malformed-trustbundle",
				Namespace: "higress-system",
			},
			Data: map[string]string{
				"ca.crt": "hello",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cert-http",
				Namespace: "higress-system",
			},
			Data: map[string]string{
				"ca.crt": rsaCertPEM,
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "malformed",
				Namespace: "default",
			},
			Data: map[string]string{
				"not-ca.crt": "hello",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "auth-cert",
				Namespace: "default",
			},
			Data: map[string]string{
				"ca.crt": rsaCertPEM,
			},
		},
	}
)

func init() {
	features.EnableAlphaGatewayAPI = true
	features.EnableAmbientWaypoints = true
	features.EnableAmbientMultiNetwork = true
	// Recompute with ambient enabled
	classInfos = getClassInfos()
	builtinClasses = getBuiltinClasses()
}

type TestStatusQueue struct {
	mu    sync.Mutex
	state map[status.Resource]any
}

func (t *TestStatusQueue) EnqueueStatusUpdateResource(context any, target status.Resource) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state[target] = context
}

func (t *TestStatusQueue) Statuses() []any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return maps.Values(t.state)
}

func (t *TestStatusQueue) Dump() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	sb := strings.Builder{}
	objs := []crd.IstioKind{}
	for k, v := range t.state {
		statusj, _ := json.Marshal(v)
		gk, _ := gvk.FromGVR(k.GroupVersionResource)
		obj := crd.IstioKind{
			TypeMeta: metav1.TypeMeta{
				Kind:       gk.Kind,
				APIVersion: k.GroupVersion().String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      k.Name,
				Namespace: k.Namespace,
			},
			Spec:   nil,
			Status: ptr.Of(json.RawMessage(statusj)),
		}
		objs = append(objs, obj)
	}
	slices.SortFunc(objs, func(a, b crd.IstioKind) int {
		ord := []string{gvk.GatewayClass.Kind, gvk.Gateway.Kind, gvk.HTTPRoute.Kind, gvk.GRPCRoute.Kind, gvk.TLSRoute.Kind, gvk.TCPRoute.Kind}
		if r := cmp.Compare(slices.Index(ord, a.Kind), slices.Index(ord, b.Kind)); r != 0 {
			return r
		}
		if r := a.CreationTimestamp.Time.Compare(b.CreationTimestamp.Time); r != 0 {
			return r
		}
		if r := cmp.Compare(a.Namespace, b.Namespace); r != 0 {
			return r
		}
		return cmp.Compare(a.Name, b.Name)
	})
	for _, obj := range objs {
		b, err := yaml.Marshal(obj)
		if err != nil {
			panic(err.Error())
		}
		// Replace parts that are not stable
		b = timestampRegex.ReplaceAll(b, []byte("lastTransitionTime: fake"))
		sb.WriteString(string(b))
		sb.WriteString("---\n")
	}
	return sb.String()
}

var _ status.Queue = &TestStatusQueue{}

func TestConvertResources(t *testing.T) {
	validator := crdvalidation.NewIstioValidator(t)
	cases := []struct {
		name string
		// Some configs are intended to be generated with invalid configs, and since they will be validated
		// by the validator, we need to ignore the validation errors to prevent the test from failing.
		validationIgnorer *crdvalidation.ValidationIgnorer
	}{
		{name: "http"},
		{name: "tcp"},
		{name: "tls"},
		{name: "grpc"},
		{name: "mismatch"},
		{name: "weighted"},
		{name: "zero"},
		//{name: "mesh"},
		{
			name: "invalid",
			validationIgnorer: crdvalidation.NewValidationIgnorer(
				"default/^invalid-backendRef-kind-",
				"default/^invalid-backendRef-mixed-",
			),
		},
		//{name: "multi-gateway"},
		{name: "delegated"},
		{name: "route-binding"},
		{name: "reference-policy-tls"},
		{
			name: "reference-policy-service",
			validationIgnorer: crdvalidation.NewValidationIgnorer(
				"higress-system/^backend-not-allowed-",
			),
		},
		{
			name: "reference-policy-tcp",
			validationIgnorer: crdvalidation.NewValidationIgnorer(
				"higress-system/^not-allowed-echo-",
			),
		},
		{
			name: "reference-policy-inferencepool",
			validationIgnorer: crdvalidation.NewValidationIgnorer(
				"higress-system/^backend-not-allowed-",
			),
		},
		//{name: "serviceentry"},
		//{name: "status"},
		//{name: "eastwest"},
		//{name: "eastwest-tlsoption"},
		//{name: "eastwest-labelport"},
		//{name: "eastwest-remote"},
		//{name: "east-west-ambient"},
		//{name: "mcs"},
		//{name: "route-precedence"},
		//{name: "waypoint"},
		//{name: "isolation"},
		{name: "backend-lb-policy"},
		{
			name: "backend-tls-policy",
			validationIgnorer: crdvalidation.NewValidationIgnorer(
				"default/echo-https",
				"default/external-service",
				"default/multi-host-service",
			),
		},
		{name: "mix-backend-policy"},
		//{name: "listenerset"},
		//{name: "listenerset-cross-namespace"},
		//{name: "listenerset-invalid"},
		//{
		//	name: "listenerset-empty-listeners",
		//	validationIgnorer: crdvalidation.NewValidationIgnorer(
		//		"higress-system/parent-gateway",
		//	),
		//},
		//{
		//	name: "valid-invalid-parent-ref",
		//	validationIgnorer: crdvalidation.NewValidationIgnorer(
		//		"default/^valid-invalid-parent-ref-",
		//	),
		//},
	}
	test.SetForTest(t, &features.EnableGatewayAPIGatewayClassController, false)
	test.SetForTest(t, &features.EnableGatewayAPIInferenceExtension, true)

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			stop := test.NewStop(t)
			input := readConfig(t, fmt.Sprintf("testdata/%s.yaml", tt.name), validator, tt.validationIgnorer)
			kc := kube.NewFakeClient(input...)
			setupClientCRDs(t, kc)
			// Setup a few preconfigured services
			instances := []*model.ServiceInstance{}
			for _, svc := range services {
				for i, port := range svc.Ports {
					epPort := uint32(0)
					if i == 0 {
						epPort = 8080 // Just to make sure we test mismatch
					}
					instances = append(instances, &model.ServiceInstance{
						Service:     svc,
						ServicePort: port,
						Endpoint:    &model.IstioEndpoint{EndpointPort: epPort},
					})
				}
			}
			cg := core.NewConfigGenTest(t, core.TestOptions{
				Services:  services,
				Instances: instances,
			})

			dbg := &krt.DebugHandler{}
			dumpOnFailure(t, dbg)
			ctrl := NewController(
				kc,
				AlwaysReady,
				controller.Options{DomainSuffix: "domain.suffix", KrtDebugger: dbg},
				nil,
			)
			sq := &TestStatusQueue{
				state: map[status.Resource]any{},
			}
			go ctrl.Run(stop)
			kc.RunAndWait(stop)
			ctrl.Reconcile(cg.PushContext())
			kube.WaitForCacheSync("test", stop, ctrl.HasSynced)
			// Normally we don't care to block on status being written, but here we need to since we want to test output
			statusSynced := ctrl.status.SetQueue(sq)
			for _, st := range statusSynced {
				st.WaitUntilSynced(stop)
			}

			res := ctrl.List(gvk.Gateway, "")
			sortConfigByCreationTime(res)
			vs := ctrl.List(gvk.VirtualService, "")
			res = append(res, sortedConfigByCreationTime(vs)...)
			dr := ctrl.List(gvk.DestinationRule, "")
			res = append(res, sortedConfigByCreationTime(dr)...)

			goldenFile := fmt.Sprintf("testdata/%s.yaml.golden", tt.name)
			b := marshalYaml(t, res)
			//t.Logf("marshaled yaml result : %s", string(b))

			util.CompareContent(t, b, goldenFile)

			outputStatus := sq.Dump()
			goldenStatusFile := fmt.Sprintf("testdata/%s.status.yaml.golden", tt.name)
			util.CompareContent(t, []byte(outputStatus), goldenStatusFile)
		})
	}
}

func TestReportGatewayStatusAddressType(t *testing.T) {
	cases := []struct {
		name      string
		ingresses []corev1.LoadBalancerIngress
		want      map[string]k8s.AddressType
	}{
		{
			name: "load balancer IPv4",
			ingresses: []corev1.LoadBalancerIngress{
				{IP: "47.98.1.2"},
			},
			want: map[string]k8s.AddressType{
				"47.98.1.2": k8s.IPAddressType,
			},
		},
		{
			name: "load balancer IPv6",
			ingresses: []corev1.LoadBalancerIngress{
				{IP: "2001:db8::1"},
			},
			want: map[string]k8s.AddressType{
				"2001:db8::1": k8s.IPAddressType,
			},
		},
		{
			name: "load balancer hostname",
			ingresses: []corev1.LoadBalancerIngress{
				{Hostname: "k8s-higress-higressg-1234567890.us-west-2.elb.amazonaws.com"},
			},
			want: map[string]k8s.AddressType{
				"k8s-higress-higressg-1234567890.us-west-2.elb.amazonaws.com": k8s.HostnameAddressType,
			},
		},
		{
			name: "mixed load balancer addresses",
			ingresses: []corev1.LoadBalancerIngress{
				{IP: "47.98.1.2"},
				{Hostname: "higress.cn-hangzhou.alb.aliyuncs.com"},
			},
			want: map[string]k8s.AddressType{
				"47.98.1.2":                            k8s.IPAddressType,
				"higress.cn-hangzhou.alb.aliyuncs.com": k8s.HostnameAddressType,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "higress-gateway",
					Namespace: "higress-system",
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: tt.ingresses,
					},
				},
			}
			stop := test.NewStop(t)
			kc := kube.NewFakeClient(svc)
			kc.RunAndWait(stop)
			ctx := NewGatewayContext(nil, constants.DefaultClusterName, kc, "cluster.local")
			gw := &k8sbeta.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "higress-gateway",
					Namespace:  "higress-system",
					Generation: 1,
				},
				Spec: k8sbeta.GatewaySpec{
					GatewayClassName: "higress",
					Listeners: []k8sbeta.Listener{
						{
							Name:     "http",
							Port:     80,
							Protocol: k8s.HTTPProtocolType,
						},
					},
				},
			}
			gs := &k8sbeta.GatewayStatus{}
			servers := []*istio.Server{
				{
					Port: &istio.Port{
						Name:     "http",
						Number:   80,
						Protocol: "HTTP",
					},
				},
			}

			reportGatewayStatus(&ctx, gw, gs, []string{"higress-gateway.higress-system.svc.cluster.local"}, servers, 0, nil)

			if len(gs.Addresses) != len(tt.want) {
				t.Fatalf("expected %d addresses, got %d: %#v", len(tt.want), len(gs.Addresses), gs.Addresses)
			}
			for _, got := range gs.Addresses {
				wantType, ok := tt.want[got.Value]
				if !ok {
					t.Fatalf("unexpected address value %q in %#v", got.Value, gs.Addresses)
				}
				if got.Type == nil {
					t.Fatalf("expected address %q type %q, got nil", got.Value, wantType)
				}
				if *got.Type != wantType {
					t.Fatalf("expected address %q type %q, got %q", got.Value, wantType, *got.Type)
				}
			}
		})
	}
}

func setupClientCRDs(t *testing.T, kc kube.CLIClient) {
	for _, crd := range []schema.GroupVersionResource{
		gvr.KubernetesGateway,
		gvr.ReferenceGrant,
		gvr.XListenerSet,
		gvr.GatewayClass,
		gvr.HTTPRoute,
		gvr.GRPCRoute,
		gvr.TCPRoute,
		gvr.TLSRoute,
		gvr.ServiceEntry,
		gvr.XBackendTrafficPolicy,
		gvr.BackendTLSPolicy,
		gvr.InferencePool,
	} {
		clienttest.MakeCRDWithAnnotations(t, kc, crd, map[string]string{
			consts.BundleVersionAnnotation: "v1.1.0",
		})
	}
}

func dumpOnFailure(t *testing.T, debugger *krt.DebugHandler) {
	t.Cleanup(func() {
		if t.Failed() {
			b, _ := yaml.Marshal(debugger)
			t.Log(string(b))
		}
	})
}

func TestSortHTTPRoutes(t *testing.T) {
	cases := []struct {
		name string
		in   []*istio.HTTPRoute
		out  []*istio.HTTPRoute
	}{
		{
			"match is preferred over no match",
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Exact{
									Exact: "/foo",
								},
							},
						},
					},
				},
			},
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Exact{
									Exact: "/foo",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{},
				},
			},
		},
		{
			"path matching exact > regex > prefix",
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Regex{
									Regex: ".*foo",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Exact{
									Exact: "/foo",
								},
							},
						},
					},
				},
			},
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Exact{
									Exact: "/foo",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Regex{
									Regex: ".*foo",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/",
								},
							},
						},
					},
				},
			},
		},
		{
			"path prefix matching with largest characters",
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/foo",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/foobar",
								},
							},
						},
					},
				},
			},
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/foobar",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/foo",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/",
								},
							},
						},
					},
				},
			},
		},
		{
			"path match is preferred over method match",
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Method: &istio.StringMatch{
								MatchType: &istio.StringMatch_Exact{
									Exact: "GET",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/foobar",
								},
							},
						},
					},
				},
			},
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/foobar",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Method: &istio.StringMatch{
								MatchType: &istio.StringMatch_Exact{
									Exact: "GET",
								},
							},
						},
					},
				},
			},
		},
		{
			"largest number of header matches is preferred",
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Headers: map[string]*istio.StringMatch{
								"header1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Headers: map[string]*istio.StringMatch{
								"header1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
								"header2": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value2",
									},
								},
							},
						},
					},
				},
			},
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Headers: map[string]*istio.StringMatch{
								"header1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
								"header2": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value2",
									},
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Headers: map[string]*istio.StringMatch{
								"header1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			"largest number of query params is preferred",
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							QueryParams: map[string]*istio.StringMatch{
								"param1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							QueryParams: map[string]*istio.StringMatch{
								"param1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
								"param2": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value2",
									},
								},
							},
						},
					},
				},
			},
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							QueryParams: map[string]*istio.StringMatch{
								"param1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
								"param2": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value2",
									},
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							QueryParams: map[string]*istio.StringMatch{
								"param1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			"method > header > query params > path",
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/",
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							QueryParams: map[string]*istio.StringMatch{
								"param1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Method: &istio.StringMatch{
								MatchType: &istio.StringMatch_Exact{Exact: "GET"},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Headers: map[string]*istio.StringMatch{
								"param1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
							},
						},
					},
				},
			},
			[]*istio.HTTPRoute{
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Method: &istio.StringMatch{
								MatchType: &istio.StringMatch_Exact{Exact: "GET"},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Headers: map[string]*istio.StringMatch{
								"param1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							QueryParams: map[string]*istio.StringMatch{
								"param1": {
									MatchType: &istio.StringMatch_Exact{
										Exact: "value1",
									},
								},
							},
						},
					},
				},
				{
					Match: []*istio.HTTPMatchRequest{
						{
							Uri: &istio.StringMatch{
								MatchType: &istio.StringMatch_Prefix{
									Prefix: "/",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			sortHTTPRoutes(tt.in)
			if !reflect.DeepEqual(tt.in, tt.out) {
				t.Fatalf("expected %v, got %v", tt.out, tt.in)
			}
		})
	}
}

// Test is a little janky, but it checks if we can pass a `parent.Hostnames` in the form
// of `*.example.com` and `*/*.example.com` without a panic and successfully match.
func TestGatewayReferenceAllowedParentHostnameParsing(t *testing.T) {
	cases := []struct {
		Name            string
		ParentHostnames []string
		RouteHostnames  []k8s.Hostname
	}{
		{
			Name:            "implied wildcard",
			ParentHostnames: []string{"*.example.com"},
			RouteHostnames:  []k8s.Hostname{"bookinfo.example.com"},
		},
		{
			Name:            "explicit wildcard",
			ParentHostnames: []string{"*/*.example.com"},
			RouteHostnames:  []k8s.Hostname{"bookinfo.example.com"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			// ctx doesn't end up getting used, but we need to pass something
			ctx := RouteContext{}
			routeKind := gvk.HTTPRoute
			parent := parentInfo{
				InternalName: "default/bookinfo-gateway-istio-autogenerated-k8s-gateway-http",
				Hostnames:    []string{"*.example.com"},
				AllowedKinds: []k8s.RouteGroupKind{
					toRouteKind(gvk.HTTPRoute),
					toRouteKind(gvk.GRPCRoute),
				},
				OriginalHostname: "",
				SectionName:      "http",
				Port:             80,
				Protocol:         "HTTP",
			}
			parentRef := parentReference{
				parentKey: parentKey{
					Kind:      gvk.Gateway,
					Name:      "bookinfo-gateway",
					Namespace: "default",
				},
				SectionName: "",
				Port:        0,
			}
			hostnames := []k8s.Hostname{"bookinfo.example.com"}

			parentError, waypointError := referenceAllowed(ctx, &parent, routeKind, parentRef, hostnames, "default")
			if parentError != nil {
				t.Fatalf("expected no error, got %v", parentError)
			}
			if waypointError != nil {
				t.Fatalf("expected no error, got %v", waypointError)
			}
		})
	}
}

func TestReferencePolicy(t *testing.T) {
	validator := crdvalidation.NewIstioValidator(t)
	type res struct {
		name, namespace string
		allowed         bool
	}
	cases := []struct {
		name         string
		config       string
		expectations []res
	}{
		{
			name: "simple",
			config: `apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
 name: allow-gateways-to-ref-secrets
 namespace: default
spec:
 from:
 - group: gateway.networking.k8s.io
   kind: Gateway
   namespace: higress-system
 to:
 - group: ""
   kind: Secret
`,
			expectations: []res{
				// allow cross namespace
				{"kubernetes-gateway://default/wildcard-example-com-cert", "higress-system", true},
				// denied same namespace. We do not implicitly allow (in this code - higher level code does)
				{"kubernetes-gateway://default/wildcard-example-com-cert", "default", false},
				// denied namespace
				{"kubernetes-gateway://default/wildcard-example-com-cert", "bad", false},
			},
		},
		{
			name: "multiple in one",
			config: `apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
 name: allow-gateways-to-ref-secrets
 namespace: default
spec:
 from:
 - group: gateway.networking.k8s.io
   kind: Gateway
   namespace: ns-1
 - group: gateway.networking.k8s.io
   kind: Gateway
   namespace: ns-2
 to:
 - group: ""
   kind: Secret
`,
			expectations: []res{
				{"kubernetes-gateway://default/wildcard-example-com-cert", "ns-1", true},
				{"kubernetes-gateway://default/wildcard-example-com-cert", "ns-2", true},
				{"kubernetes-gateway://default/wildcard-example-com-cert", "bad", false},
			},
		},
		{
			name: "multiple",
			config: `apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
 name: ns1
 namespace: default
spec:
 from:
 - group: gateway.networking.k8s.io
   kind: Gateway
   namespace: ns-1
 to:
 - group: ""
   kind: Secret
---
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
 name: ns2
 namespace: default
spec:
 from:
 - group: gateway.networking.k8s.io
   kind: Gateway
   namespace: ns-2
 to:
 - group: ""
   kind: Secret
`,
			expectations: []res{
				{"kubernetes-gateway://default/wildcard-example-com-cert", "ns-1", true},
				{"kubernetes-gateway://default/wildcard-example-com-cert", "ns-2", true},
				{"kubernetes-gateway://default/wildcard-example-com-cert", "bad", false},
			},
		},
		{
			name: "same namespace",
			config: `apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
 name: allow-gateways-to-ref-secrets
 namespace: default
spec:
 from:
 - group: gateway.networking.k8s.io
   kind: Gateway
   namespace: default
 to:
 - group: ""
   kind: Secret
`,
			expectations: []res{
				{"kubernetes-gateway://default/wildcard-example-com-cert", "higress-system", false},
				{"kubernetes-gateway://default/wildcard-example-com-cert", "default", true},
				{"kubernetes-gateway://default/wildcard-example-com-cert", "bad", false},
			},
		},
		{
			name: "same name",
			config: `apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
 name: allow-gateways-to-ref-secrets
 namespace: default
spec:
 from:
 - group: gateway.networking.k8s.io
   kind: Gateway
   namespace: default
 to:
 - group: ""
   kind: Secret
   name: public
`,
			expectations: []res{
				{"kubernetes-gateway://default/public", "higress-system", false},
				{"kubernetes-gateway://default/public", "default", true},
				{"kubernetes-gateway://default/private", "default", false},
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			input := readConfigString(t, tt.config, validator, nil)
			kr := setupController(t, input...)
			for _, sc := range tt.expectations {
				t.Run(fmt.Sprintf("%v/%v", sc.name, sc.namespace), func(t *testing.T) {
					got := kr.SecretAllowed(gvk.KubernetesGateway, sc.name, sc.namespace)
					if got != sc.allowed {
						t.Fatalf("expected allowed=%v, got allowed=%v", sc.allowed, got)
					}
				})
			}
		})
	}
}

var timestampRegex = regexp.MustCompile(`lastTransitionTime:.*`)

func readConfig(t testing.TB, filename string, validator *crdvalidation.Validator, ignorer *crdvalidation.ValidationIgnorer) []runtime.Object {
	t.Helper()

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read input yaml file: %v", err)
	}
	objs := readConfigString(t, string(data), validator, ignorer)

	namespaces := sets.New[string](slices.Map(objs, func(e runtime.Object) string {
		return e.(controllers.Object).GetNamespace()
	})...)
	for _, svc := range services {
		if !strings.HasSuffix(svc.Hostname.String(), "domain.suffix") {
			continue
		}
		name := svc.Attributes.Name
		if name == "" {
			name, _, _ = strings.Cut(svc.Hostname.String(), ".")
		}
		svcObj := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: svc.Attributes.Namespace,
				Name:      name,
				Labels:    svc.Attributes.Labels,
			},
			Spec: corev1.ServiceSpec{
				Ports: svcPorts,
			},
		}
		objs = append(objs, svcObj)
	}
	objs = append(objs, objects...)

	for ns := range namespaces {
		objs = append(objs, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
				Labels: map[string]string{
					"higress.io/test-name-part": strings.Split(ns, "-")[0],
				},
			},
		})
	}
	return objs
}

func readConfigString(t testing.TB, data string, validator *crdvalidation.Validator, ignorer *crdvalidation.ValidationIgnorer,
) []runtime.Object {
	if err := validator.ValidateCustomResourceYAML(data, ignorer); err != nil {
		t.Error(err)
	}

	c, err := kubernetesObjectsFromString(data)
	if err != nil {
		t.Fatalf("failed to parse CRD: %v", err)
	}
	return c
}

// Print as YAML
func marshalYaml(t test.Failer, cl []config.Config) []byte {
	t.Helper()
	result := []byte{}
	separator := []byte("---\n")
	for _, config := range cl {
		obj, err := crd.ConvertConfig(config)
		if err != nil {
			t.Fatalf("Could not decode %v: %v", config.Name, err)
		}
		bytes, err := yaml.Marshal(obj)
		if err != nil {
			t.Fatalf("Could not convert %v to YAML: %v", config, err)
		}
		result = append(result, bytes...)
		result = append(result, separator...)
	}
	return result
}

func TestHumanReadableJoin(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b, and c"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.input, "_"), func(t *testing.T) {
			if got := humanReadableJoin(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

//
//func BenchmarkBuildHTTPVirtualServices(b *testing.B) {
//	ports := []*model.Port{
//		{
//			Name:     "http",
//			Port:     80,
//			Protocol: "HTTP",
//		},
//		{
//			Name:     "tcp",
//			Port:     34000,
//			Protocol: "TCP",
//		},
//	}
//	ingressSvc := &model.Service{
//		Attributes: model.ServiceAttributes{
//			Name:      "istio-ingressgateway",
//			Namespace: "higress-system",
//			ClusterExternalAddresses: &model.AddressMap{
//				Addresses: map[cluster.ID][]string{
//					constants.DefaultClusterName: {"1.2.3.4"},
//				},
//			},
//		},
//		Ports:    ports,
//		Hostname: "istio-ingressgateway.higress-system.svc.domain.suffix",
//	}
//	altIngressSvc := &model.Service{
//		Attributes: model.ServiceAttributes{
//			Namespace: "higress-system",
//		},
//		Ports:    ports,
//		Hostname: "example.com",
//	}
//	cg := core.NewConfigGenTest(b, core.TestOptions{
//		Services: []*model.Service{ingressSvc, altIngressSvc},
//		Instances: []*model.ServiceInstance{
//			{Service: ingressSvc, ServicePort: ingressSvc.Ports[0], Endpoint: &model.IstioEndpoint{EndpointPort: 8080}},
//			{Service: ingressSvc, ServicePort: ingressSvc.Ports[1], Endpoint: &model.IstioEndpoint{}},
//			{Service: altIngressSvc, ServicePort: altIngressSvc.Ports[0], Endpoint: &model.IstioEndpoint{}},
//			{Service: altIngressSvc, ServicePort: altIngressSvc.Ports[1], Endpoint: &model.IstioEndpoint{}},
//		},
//	})
//
//	validator := crdvalidation.NewIstioValidator(b)
//	input := readConfig(b, "testdata/benchmark-httproute.yaml", validator, nil)
//	kr := splitInput(b, input)
//	kr.Context = NewGatewayContext(cg.PushContext(), "Kubernetes")
//	ctx := configContext{
//		GatewayResources:  kr,
//		AllowedReferences: convertReferencePolicies(kr),
//	}
//	_, gwMap, _ := convertGateways(ctx)
//	ctx.GatewayReferences = gwMap
//
//	b.ResetTimer()
//	for n := 0; n < b.N; n++ {
//		// for gateway routes, build one VS per gateway+host
//		gatewayRoutes := make(map[string]map[string]*config.Config)
//		// for mesh routes, build one VS per namespace+host
//		meshRoutes := make(map[string]map[string]*config.Config)
//		for _, obj := range kr.HTTPRoute {
//			buildHTTPVirtualServices(ctx, obj, gatewayRoutes, meshRoutes)
//		}
//	}
//}

//func TestExtractGatewayServices(t *testing.T) {
//	tests := []struct {
//		name            string
//		r               GatewayResources
//		kgw             *k8s.Gateway
//		obj             config.Config
//		gatewayServices []string
//		err             *ConfigError
//	}{
//		{
//			name: "managed gateway",
//			r:    GatewayResources{Domain: "cluster.local"},
//			kgw: &k8s.GatewaySpec{
//				GatewayClassName: "istio",
//			},
//			obj: config.Config{
//				Meta: config.Meta{
//					Name:      "foo",
//					Namespace: "default",
//				},
//			},
//			gatewayServices: []string{"foo-istio.default.svc.cluster.local"},
//		},
//		{
//			name: "managed gateway with name overridden",
//			r:    GatewayResources{Domain: "cluster.local"},
//			kgw: &k8s.GatewaySpec{
//				GatewayClassName: "istio",
//			},
//			obj: config.Config{
//				Meta: config.Meta{
//					Name:      "foo",
//					Namespace: "default",
//					Annotations: map[string]string{
//						annotation.GatewayNameOverride.Name: "bar",
//					},
//				},
//			},
//			gatewayServices: []string{"bar.default.svc.cluster.local"},
//		},
//		{
//			name: "unmanaged gateway",
//			r:    GatewayResources{Domain: "domain"},
//			kgw: &k8s.GatewaySpec{
//				GatewayClassName: "istio",
//				Addresses: []k8s.GatewayAddress{
//					{
//						Value: "abc",
//					},
//					{
//						Type: func() *k8s.AddressType {
//							t := k8s.HostnameAddressType
//							return &t
//						}(),
//						Value: "example.com",
//					},
//					{
//						Type: func() *k8s.AddressType {
//							t := k8s.IPAddressType
//							return &t
//						}(),
//						Value: "1.2.3.4",
//					},
//				},
//			},
//			obj: config.Config{
//				Meta: config.Meta{
//					Name:      "foo",
//					Namespace: "default",
//				},
//			},
//			gatewayServices: []string{"abc.default.svc.domain", "example.com"},
//			err: &ConfigError{
//				Reason:  InvalidAddress,
//				Message: "only Hostname is supported, ignoring [1.2.3.4]",
//			},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			gatewayServices, err := extractGatewayServices(tt.r, tt.kgw, tt.obj, classInfo{})
//			assert.Equal(t, gatewayServices, tt.gatewayServices)
//			assert.Equal(t, err, tt.err)
//		})
//	}
//}

func kubernetesObjectsFromString(s string) ([]runtime.Object, error) {
	var objects []runtime.Object
	decode := kube.IstioCodec.UniversalDeserializer().Decode
	objectStrs := strings.Split(s, "---")
	for _, s := range objectStrs {
		if len(strings.TrimSpace(s)) == 0 {
			continue
		}
		o, _, err := decode([]byte(s), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed deserializing kubernetes object: %v (%v)", err, s)
		}
		objects = append(objects, o)
	}
	return objects, nil
}

func firstValue[T, U any](val T, _ U) T {
	return val
}
