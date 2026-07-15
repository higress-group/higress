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
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	gateway "sigs.k8s.io/gateway-api/apis/v1beta1"

	"istio.io/api/label"
	"istio.io/istio/pkg/env"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"

	higressconfig "github.com/alibaba/higress/v2/pkg/config"
)

const (
	managedGatewayServiceLabel   = "gateway.higress.io/managed-service"
	managedGatewayNamespaceLabel = "gateway.higress.io/gateway-namespace"
	managedGatewayNameAnnotation = "gateway.higress.io/gateway-name"
)

var enableManagedGatewayService = env.Register(
	"HIGRESS_ENABLE_GATEWAY_API_MANAGED_SERVICE",
	false,
	"Create an isolated Service and listener port for each managed Gateway",
).Get()

func managedGatewayServiceName(namespace, name string) string {
	fullName := fmt.Sprintf("%s-%s-%s", namespace, name, gatewayClassName)
	if len(fullName) <= 63 {
		return fullName
	}
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(fullName)))[:8]
	return fullName[:54] + "-" + sum
}

func managedGatewayServiceHostname(domainSuffix, namespace, name string) string {
	return fmt.Sprintf("%s.%s.svc.%s", managedGatewayServiceName(namespace, name), higressconfig.PodNamespace, domainSuffix)
}

func (c *Controller) reconcileManagedGatewayService(
	gateways krt.Collection[*gateway.Gateway],
	services krt.Collection[*corev1.Service],
) func(types.NamespacedName) error {
	return func(key types.NamespacedName) error {
		serviceKey := types.NamespacedName{
			Namespace: higressconfig.PodNamespace,
			Name:      managedGatewayServiceName(key.Namespace, key.Name),
		}
		existing := ptr.Flatten(services.GetKey(serviceKey.String()))
		gw := ptr.Flatten(gateways.GetKey(key.String()))
		if gw == nil || gw.Spec.GatewayClassName != gatewayClassName || !UseDefaultService(&gw.Spec) {
			if existing != nil &&
				existing.Labels[managedGatewayServiceLabel] == "true" &&
				existing.Labels[managedGatewayNamespaceLabel] == key.Namespace &&
				existing.Annotations[managedGatewayNameAnnotation] == key.Name {
				return c.client.Kube().CoreV1().Services(serviceKey.Namespace).Delete(
					context.Background(), serviceKey.Name, metav1.DeleteOptions{})
			}
			return nil
		}

		desired, err := c.buildManagedGatewayService(gw, existing)
		if err != nil {
			return err
		}
		client := c.client.Kube().CoreV1().Services(desired.Namespace)
		if existing == nil {
			created, err := client.Create(context.Background(), desired, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			updated := setGatewayServiceTargetPorts(created)
			if updated {
				_, err = client.Update(context.Background(), created, metav1.UpdateOptions{})
			}
			return err
		}

		desired.ResourceVersion = existing.ResourceVersion
		preserveServiceAllocatedFields(desired, existing)
		if managedGatewayServiceEqual(desired, existing) {
			return nil
		}
		_, err = client.Update(context.Background(), desired, metav1.UpdateOptions{})
		return err
	}
}

func (c *Controller) buildManagedGatewayService(gw *gateway.Gateway, existing *corev1.Service) (*corev1.Service, error) {
	base, err := c.client.Kube().CoreV1().Services(higressconfig.PodNamespace).Get(
		context.Background(), higressconfig.GatewayName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("default gateway Service %s/%s was not found", higressconfig.PodNamespace, higressconfig.GatewayName)
		}
		return nil, err
	}

	serviceType := base.Spec.Type
	if serviceType == corev1.ServiceTypeClusterIP {
		serviceType = corev1.ServiceTypeNodePort
	}
	ports := make([]corev1.ServicePort, 0, len(gw.Spec.Listeners))
	seen := map[gateway.PortNumber]struct{}{}
	for _, listener := range gw.Spec.Listeners {
		if _, found := seen[listener.Port]; found {
			continue
		}
		seen[listener.Port] = struct{}{}
		ports = append(ports, corev1.ServicePort{
			Name:       "gateway-" + strconv.Itoa(int(listener.Port)),
			Protocol:   corev1.ProtocolTCP,
			Port:       int32(listener.Port),
			TargetPort: intstr.FromInt(int(listener.Port)),
		})
	}

	labels := map[string]string{}
	annotations := map[string]string{}
	if existing != nil {
		for key, value := range existing.Labels {
			labels[key] = value
		}
		for key, value := range existing.Annotations {
			annotations[key] = value
		}
	}
	labels[managedGatewayServiceLabel] = "true"
	labels[managedGatewayNamespaceLabel] = gw.Namespace
	annotations[managedGatewayNameAnnotation] = gw.Name
	if len(gw.Name) <= 63 {
		labels[label.IoK8sNetworkingGatewayGatewayName.Name] = gw.Name
	} else {
		delete(labels, label.IoK8sNetworkingGatewayGatewayName.Name)
	}
	selector := make(map[string]string, len(c.DefaultGatewaySelector))
	for key, value := range c.DefaultGatewaySelector {
		selector[key] = value
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        managedGatewayServiceName(gw.Namespace, gw.Name),
			Namespace:   higressconfig.PodNamespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:                  serviceType,
			Selector:              selector,
			Ports:                 ports,
			ExternalTrafficPolicy: base.Spec.ExternalTrafficPolicy,
		},
	}, nil
}

func managedGatewayServiceEqual(desired, existing *corev1.Service) bool {
	return reflect.DeepEqual(desired.Labels, existing.Labels) &&
		reflect.DeepEqual(desired.Annotations, existing.Annotations) &&
		desired.Spec.Type == existing.Spec.Type &&
		desired.Spec.ExternalTrafficPolicy == existing.Spec.ExternalTrafficPolicy &&
		reflect.DeepEqual(desired.Spec.Selector, existing.Spec.Selector) &&
		reflect.DeepEqual(desired.Spec.Ports, existing.Spec.Ports)
}

func preserveServiceAllocatedFields(desired, existing *corev1.Service) {
	desired.Spec.ClusterIP = existing.Spec.ClusterIP
	desired.Spec.ClusterIPs = existing.Spec.ClusterIPs
	desired.Spec.IPFamilies = existing.Spec.IPFamilies
	desired.Spec.IPFamilyPolicy = existing.Spec.IPFamilyPolicy
	desired.Spec.HealthCheckNodePort = existing.Spec.HealthCheckNodePort
	for i := range desired.Spec.Ports {
		for _, oldPort := range existing.Spec.Ports {
			if oldPort.Port == desired.Spec.Ports[i].Port && oldPort.Protocol == desired.Spec.Ports[i].Protocol {
				desired.Spec.Ports[i].NodePort = oldPort.NodePort
				if oldPort.NodePort != 0 {
					desired.Spec.Ports[i].TargetPort = intstr.FromInt(int(oldPort.NodePort))
				}
				break
			}
		}
	}
}

func setGatewayServiceTargetPorts(service *corev1.Service) bool {
	updated := false
	for i := range service.Spec.Ports {
		if service.Spec.Ports[i].NodePort == 0 {
			continue
		}
		targetPort := intstr.FromInt(int(service.Spec.Ports[i].NodePort))
		if service.Spec.Ports[i].TargetPort != targetPort {
			service.Spec.Ports[i].TargetPort = targetPort
			updated = true
		}
	}
	return updated
}

func managedGatewayTargetPort(service *corev1.Service, listenerPort gateway.PortNumber) (uint32, bool) {
	if service == nil {
		return 0, false
	}
	for _, port := range service.Spec.Ports {
		if port.Port != int32(listenerPort) || port.TargetPort.Type != intstr.Int || port.TargetPort.IntVal == 0 {
			continue
		}
		return uint32(port.TargetPort.IntVal), true
	}
	return 0, false
}
