// Copyright (c) 2023 Alibaba Group Holding Ltd.
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

package annotations

import (
	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
	networking "istio.io/api/networking/v1alpha3"
)

const (
	mirrorTargetService  = "mirror-target-service"
	mirrorPercentage     = "mirror-percentage"
	mirrorTargetFQDN     = "mirror-target-fqdn"
	mirrorTargetFQDNPort = "mirror-target-fqdn-port"
)

var (
	_ Parser       = &mirror{}
	_ RouteHandler = &mirror{}
)

type MirrorConfig struct {
	util.ServiceInfo
	Percentage *wrappers.DoubleValue
	FQDN       string
	FPort      uint32 // Port for FQDN
}

type mirror struct{}

func (m mirror) Parse(annotations Annotations, config *Ingress, globalContext *GlobalContext) error {
	if !needMirror(annotations) {
		return nil
	}

	// if FQDN is set, then parse FQDN
	if fqdn, err := annotations.ParseStringASAP(mirrorTargetFQDN); err == nil {
		// default is 80
		var port uint32
		port = 80

		if p, err := annotations.ParseInt32ASAP(mirrorTargetFQDNPort); err == nil {
			port = uint32(p)
		}

		config.Mirror = &MirrorConfig{
			Percentage: parsePercentage(annotations),
			FQDN:       fqdn,
			FPort:      port,
		}
		return nil
	}

	target, err := annotations.ParseStringASAP(mirrorTargetService)
	if err != nil {
		IngressLog.Errorf("Get mirror target service fail, err: %v", err)
		return nil
	}

	serviceInfo, err := util.ParseServiceInfo(target, config.Namespace)
	if err != nil {
		IngressLog.Errorf("Get mirror target service fail, err: %v", err)
		return nil
	}

	serviceLister, exist := globalContext.ClusterServiceList[config.ClusterId]
	if !exist {
		IngressLog.Errorf("service lister of cluster %s doesn't exist", config.ClusterId)
		return nil
	}

	service, err := serviceLister.Services(serviceInfo.Namespace).Get(serviceInfo.Name)
	if err != nil {
		IngressLog.Errorf("Mirror service %s/%s within ingress %s/%s is not found, with err: %v",
			serviceInfo.Namespace, serviceInfo.Name, config.Namespace, config.Name, err)
		return nil
	}
	if service == nil {
		IngressLog.Errorf("service %s/%s within ingress %s/%s is empty value",
			serviceInfo.Namespace, serviceInfo.Name, config.Namespace, config.Name)
		return nil
	}

	if serviceInfo.Port == 0 {
		// Use the first port
		serviceInfo.Port = uint32(service.Spec.Ports[0].Port)
	}

	config.Mirror = &MirrorConfig{
		ServiceInfo: serviceInfo,
		Percentage:  parsePercentage(annotations),
	}
	return nil
}

func parsePercentage(annotations Annotations) *wrappers.DoubleValue {
	var percentage *wrappers.DoubleValue

	if value, err := annotations.ParseIntASAP(mirrorPercentage); err == nil {
		// Envoy's mirrorPercentage is a Percent: a negative value is not a valid
		// fraction and used to be forwarded as-is.
		if value >= 0 && value < 100 {
			percentage = &wrappers.DoubleValue{
				Value: float64(value),
			}
		}
	}
	return percentage
}

func (m mirror) ApplyRoute(route *networking.HTTPRoute, config *Ingress) {
	if config.Mirror == nil {
		return
	}

	var mirrorHost string
	var mirrorPort uint32

	if config.Mirror.FQDN != "" {
		mirrorHost = config.Mirror.FQDN
		mirrorPort = config.Mirror.FPort
	} else {
		mirrorHost = util.CreateServiceFQDN(config.Mirror.Namespace, config.Mirror.Name)
		mirrorPort = config.Mirror.Port
	}

	route.Mirror = &networking.Destination{
		Host: mirrorHost,
		Port: &networking.PortSelector{
			Number: mirrorPort,
		},
	}

	if config.Mirror.Percentage != nil {
		route.MirrorPercentage = &networking.Percent{
			Value: config.Mirror.Percentage.GetValue(),
		}
	}
}

func needMirror(annotations Annotations) bool {
	return annotations.HasASAP(mirrorTargetService) || annotations.HasASAP(mirrorTargetFQDN)
}
