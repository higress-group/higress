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
	corev1 "k8s.io/api/core/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1beta1"

	higressconstants "github.com/alibaba/higress/v2/pkg/config/constants"
)

// classInfo holds information about a gateway class
type classInfo struct {
	// controller name for this class
	controller string
	// controller label for this class
	controllerLabel string
	// description for this class
	description string
	// The key in the templates to use for this class
	templates string

	// defaultServiceType sets the default service type if one is not explicit set
	defaultServiceType corev1.ServiceType

	// disableRouteGeneration, if set, will make it so the controller ignores this class.
	disableRouteGeneration bool

	// supportsListenerSet declares whether a given class supports ListenerSet
	supportsListenerSet bool

	// disableNameSuffix, if set, will avoid appending -<class> to names
	disableNameSuffix bool

	// addressType is the default address type to report
	addressType gateway.AddressType
}

var (
	gatewayClassName         = gateway.ObjectName(higressconstants.DefaultGatewayClass)
	managedGatewayController = gateway.GatewayController(higressconstants.ManagedGatewayController)
	classInfos               = getClassInfos()
	builtinClasses           = getBuiltinClasses()
)

// SetGatewayClassName configures the single GatewayClassName this process owns.
func SetGatewayClassName(gatewayClass string) {
	if gatewayClass == "" {
		gatewayClass = higressconstants.DefaultGatewayClass
	}
	gatewayClassName = gateway.ObjectName(gatewayClass)
	if gatewayClass == higressconstants.DefaultGatewayClass {
		managedGatewayController = gateway.GatewayController(higressconstants.ManagedGatewayController)
	} else {
		managedGatewayController = gateway.GatewayController(higressconstants.ManagedGatewayController + "-" + gatewayClass)
	}
	classInfos = getClassInfos()
	builtinClasses = getBuiltinClasses()
}

func getBuiltinClasses() map[gateway.ObjectName]gateway.GatewayController {
	res := map[gateway.ObjectName]gateway.GatewayController{
		// Start - Updated by Higress
		//gateway.ObjectName(features.GatewayAPIDefaultGatewayClass): gateway.GatewayController(features.ManagedGatewayController),
		gatewayClassName: managedGatewayController,
		// End - Updated by Higress
	}
	// Start - Commented by Higress
	//if features.MultiNetworkGatewayAPI {
	//	res[constants.RemoteGatewayClassName] = constants.UnmanagedGatewayController
	//}
	//
	//if features.EnableAmbientWaypoints {
	//	res[constants.WaypointGatewayClassName] = constants.ManagedGatewayMeshController
	//}
	//
	//// N.B Ambient e/w gateways are just fancy waypoints, but we want a different
	//// GatewayClass for better UX
	//if features.EnableAmbientMultiNetwork {
	//	res[constants.EastWestGatewayClassName] = constants.ManagedGatewayEastWestController
	//}
	// End - Commented by Higress
	return res
}

func getClassInfos() map[gateway.GatewayController]classInfo {
	// Start - Updated by Higress
	m := map[gateway.GatewayController]classInfo{
		managedGatewayController: {
			controller:         string(managedGatewayController),
			description:        "The default Higress GatewayClass",
			templates:          "kube-gateway",
			defaultServiceType: corev1.ServiceTypeLoadBalancer,
			//addressType:         gateway.HostnameAddressType,
			//controllerLabel:     constants.ManagedGatewayControllerLabel,
			//supportsListenerSet: true,
		},
	}

	//if features.MultiNetworkGatewayAPI {
	//	m[constants.UnmanagedGatewayController] = classInfo{
	//		// This represents a gateway that our control plane cannot discover directly via the API server.
	//		// We shouldn't generate Istio resources for it. We aren't programming this gateway.
	//		controller:             constants.UnmanagedGatewayController,
	//		description:            "Remote to this cluster. Does not deploy or affect configuration.",
	//		disableRouteGeneration: true,
	//		addressType:            gateway.HostnameAddressType,
	//		supportsListenerSet:    false,
	//	}
	//}
	//if features.EnableAmbientWaypoints {
	//	m[constants.ManagedGatewayMeshController] = classInfo{
	//		controller:          constants.ManagedGatewayMeshController,
	//		description:         "The default Istio waypoint GatewayClass",
	//		templates:           "waypoint",
	//		disableNameSuffix:   true,
	//		defaultServiceType:  corev1.ServiceTypeClusterIP,
	//		supportsListenerSet: false,
	//		// Report both. Consumers of the gateways can choose which they want.
	//		// In particular, Istio across different versions consumes different address types, so this retains compat
	//		addressType:     "",
	//		controllerLabel: constants.ManagedGatewayMeshControllerLabel,
	//	}
	//}
	//
	//if features.EnableAmbientMultiNetwork {
	//	m[constants.ManagedGatewayEastWestController] = classInfo{
	//		controller:         constants.ManagedGatewayEastWestController,
	//		description:        "The default GatewayClass for Istio East West Gateways",
	//		templates:          "waypoint",
	//		disableNameSuffix:  true,
	//		defaultServiceType: corev1.ServiceTypeLoadBalancer,
	//		addressType:        "",
	//		controllerLabel:    constants.ManagedGatewayEastWestControllerLabel,
	//	}
	//}

	// End - Updated by Higress
	return m
}

// DeploymentController is removed by Higress
