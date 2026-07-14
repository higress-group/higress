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
	"context"
	"fmt"
	serviceRegistryKube "istio.io/istio/pilot/pkg/serviceregistry/kube"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/kube"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"istio.io/api/label"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/util/sets"
)

// GatewayContext contains a minimal subset of push context functionality to be exposed to GatewayAPIControllers
type GatewayContext struct {
	ps      *model.PushContext
	cluster cluster.ID
	// Start - Updated by Higress
	client       kube.Client
	domainSuffix string
	// End - Updated by Higress
}

// Start - Updated by Higress

func NewGatewayContext(ps *model.PushContext, cluster cluster.ID, client kube.Client, domainSuffix string) GatewayContext {
	return GatewayContext{ps, cluster, client, domainSuffix}
}

// ResolveGatewayInstances attempts to resolve all instances that a gateway will be exposed on.
// Note: this function considers *all* instances of the service; its possible those instances will not actually be properly functioning
// gateways, so this is not 100% accurate, but sufficient to expose intent to users.
// The actual configuration generation is done on a per-workload basis and will get the exact set of matched instances for that workload.
// Four sets are exposed:
// * Internal addresses (eg istio-ingressgateway.istio-system.svc.cluster.local:80).
// * Internal IP addresses (eg 1.2.3.4). This comes from ClusterIP.
// * External addresses (eg 1.2.3.4), this comes from LoadBalancer services. There may be multiple in some cases (especially multi cluster).
// * Pending addresses (eg istio-ingressgateway.istio-system.svc), are LoadBalancer-type services with pending external addresses.
// * Warnings for references that could not be resolved. These are intended to be user facing.
func (gc GatewayContext) ResolveGatewayInstances(
	namespace string,
	gwsvcs []string,
	servers []*networking.Server,
) (internal, external, pending, warns []string, allUsable bool) {
	ports := map[int]struct{}{}
	for _, s := range servers {
		ports[int(s.Port.Number)] = struct{}{}
	}
	foundInternal := sets.New[string]()
	foundInternalIP := sets.New[string]()
	foundExternal := sets.New[string]()
	foundPending := sets.New[string]()
	warnings := []string{}
	foundUnusable := false

	// Cache endpoints to reduce redundant queries
	endpointsCache := make(map[string]*corev1.Endpoints)

	log.Debugf("Resolving gateway instances for %v in namespace %s", gwsvcs, namespace)
	for _, g := range gwsvcs {
		svc := gc.GetService(g, namespace, gvk.Service.Kind)
		if svc == nil {
			warnings = append(warnings, fmt.Sprintf("hostname %q not found", g))
			continue
		}

		for port := range ports {
			exists := checkServicePortExists(svc, port)
			if exists {
				foundInternal.Insert(fmt.Sprintf("%s:%d", g, port))
				dummyProxy := &model.Proxy{Metadata: &model.NodeMetadata{ClusterID: gc.cluster}}
				dummyProxy.SetIPMode(model.Dual)
				foundInternalIP.InsertAll(svc.GetAllAddressesForProxy(dummyProxy)...)
				if svc.Attributes.ClusterExternalAddresses.Len() > 0 {
					// Fetch external IPs from all clusters
					svc.Attributes.ClusterExternalAddresses.ForEach(func(c cluster.ID, externalIPs []string) {
						foundExternal.InsertAll(externalIPs...)
					})
				} else if corev1.ServiceType(svc.Attributes.Type) == corev1.ServiceTypeLoadBalancer {
					if !foundPending.Contains(g) {
						warnings = append(warnings, fmt.Sprintf("address pending for hostname %q", g))
						foundPending.Insert(g)
					}
				}
			} else {
				endpoints, ok := endpointsCache[g]
				if !ok {
					endpoints = gc.GetEndpoints(g, namespace)
					endpointsCache[g] = endpoints
				}

				if endpoints == nil {
					warnings = append(warnings, fmt.Sprintf("no instances found for hostname %q", g))
				} else {
					hintWorkloadPort := false
					for _, subset := range endpoints.Subsets {
						for _, subSetPort := range subset.Ports {
							if subSetPort.Port == int32(port) {
								hintWorkloadPort = true
								break
							}
						}
						if hintWorkloadPort {
							break
						}
					}
					if hintWorkloadPort {
						warnings = append(warnings, fmt.Sprintf(
							"port %d not found for hostname %q (hint: the service port should be specified, not the workload port", port, g))
						foundUnusable = true
					} else {
						_, isManaged := svc.Attributes.Labels[label.GatewayManaged.Name]
						var portExistsOnService bool
						for _, p := range svc.Ports {
							if p.Port == port {
								portExistsOnService = true
								break
							}
						}
						// If this is a managed gateway, the only possible explanation for no instances for the port
						// is a delay in endpoint sync. Therefore, we don't want to warn/change the Programmed condition
						// in this case as long as the port exists on the `Service` object.
						if !isManaged || !portExistsOnService {
							warnings = append(warnings, fmt.Sprintf("port %d not found for hostname %q", port, g))
							foundUnusable = true
						}
					}
				}
			}
		}
	}
	sort.Strings(warnings)
	return sets.SortedList(foundInternal), sets.SortedList(foundExternal), sets.SortedList(foundPending),
		warnings, !foundUnusable
}

func (gc GatewayContext) GetService(hostname, namespace, kind string) *model.Service {
	// Currently only supports type Kubernetes Service and InferencePool
	if kind != gvk.Service.Kind && kind != gvk.InferencePool.Kind {
		log.Warnf("Unsupported kind: expected 'Service', but got '%s'", kind)
		return nil
	}
	serviceName := extractServiceName(hostname)

	svc, err := gc.client.Kube().CoreV1().Services(namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		log.Errorf("failed to get service (serviceName: %s, namespace: %s): %v", serviceName, namespace, err)
		return nil
	}

	return serviceRegistryKube.ConvertService(*svc, gc.domainSuffix, gc.cluster, nil)
}

func (gc GatewayContext) GetEndpoints(hostname, namespace string) *corev1.Endpoints {
	serviceName := extractServiceName(hostname)

	endpoints, err := gc.client.Kube().CoreV1().Endpoints(namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})

	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		log.Errorf("failed to get endpoints (serviceName: %s, namespace: %s): %v", serviceName, namespace, err)
		return nil
	}

	return endpoints
}

func checkServicePortExists(svc *model.Service, port int) bool {
	if svc == nil {
		return false
	}
	for _, svcPort := range svc.Ports {
		if port == svcPort.Port {
			return true
		}
	}
	return false
}

func extractServiceName(hostName string) string {
	parts := strings.Split(hostName, ".")
	if len(parts) >= 4 {
		return parts[0]
	}
	return ""
}

// End - Updated by Higress
