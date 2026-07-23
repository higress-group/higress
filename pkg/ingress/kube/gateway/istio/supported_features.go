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
	"sigs.k8s.io/gateway-api/pkg/features"
)

// Keep this list aligned with upstream Istio. These v1.6 features are present
// in the SDK but are not implemented by the current data plane translation.
var skippedExtendedFeatures = []features.Feature{
	features.GatewayBackendClientCertificateFeature,
	features.GatewayFrontendClientCertificateValidationFeature,
	features.GatewayFrontendClientCertificateValidationInsecureFallbackFeature,
	features.GatewayHTTPSListenerDetectMisdirectedRequestsFeature,
	features.TLSRouteModeTerminateFeature,
	features.TLSRouteModeMixedFeature,
	features.UDPRouteFeature,
}

var SupportedFeatures = features.AllFeatures.Clone().Delete(skippedExtendedFeatures...)
