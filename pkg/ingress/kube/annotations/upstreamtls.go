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

package annotations

import (
	"regexp"
	"strings"

	"github.com/golang/protobuf/ptypes/wrappers"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/model/credentials"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
)

const (
	backendProtocol    = "backend-protocol"
	proxySSLSecret     = "proxy-ssl-secret"
	proxySSLVerify     = "proxy-ssl-verify"
	proxySSLName       = "proxy-ssl-name"
	proxySSLServerName = "proxy-ssl-server-name"

	defaultBackendProtocol = "HTTP"
)

var (
	_ Parser               = &upstreamTLS{}
	_ TrafficPolicyHandler = &upstreamTLS{}

	validProtocols = regexp.MustCompile(`^(HTTP|HTTP2|HTTPS|GRPC|GRPCS)$`)

	OnOffRegex = regexp.MustCompile(`^(on|off)$`)
)

type UpstreamTLSConfig struct {
	BackendProtocol string

	SecretName string
	SSLVerify  bool
	SNI        string
	EnableSNI  bool
}

type upstreamTLS struct{}

func (u upstreamTLS) Parse(annotations Annotations, config *Ingress, _ *GlobalContext) error {
	if !needUpstreamTLSConfig(annotations) {
		return nil
	}

	upstreamTLSConfig := &UpstreamTLSConfig{
		BackendProtocol: defaultBackendProtocol,
	}

	defer func() {
		if upstreamTLSConfig.BackendProtocol == defaultBackendProtocol {
			// no need destination rule when use HTTP protocol
			config.UpstreamTLS = nil
		} else {
			config.UpstreamTLS = upstreamTLSConfig
		}
	}()

	if proto, err := annotations.ParseStringASAP(backendProtocol); err == nil {
		proto = strings.TrimSpace(strings.ToUpper(proto))
		if validProtocols.MatchString(proto) {
			upstreamTLSConfig.BackendProtocol = proto
		}
	}

	if sslVerify, err := annotations.ParseStringASAP(proxySSLVerify); err == nil {
		if OnOffRegex.MatchString(sslVerify) {
			upstreamTLSConfig.SSLVerify = onOffToBool(sslVerify)
		}
	}

	upstreamTLSConfig.SNI, _ = annotations.ParseStringASAP(proxySSLName)

	if enableSNI, err := annotations.ParseStringASAP(proxySSLServerName); err == nil {
		if OnOffRegex.MatchString(enableSNI) {
			upstreamTLSConfig.EnableSNI = onOffToBool(enableSNI)
		}
	}

	secretName, _ := annotations.ParseStringASAP(proxySSLSecret)
	namespacedName := util.SplitNamespacedName(secretName)
	if namespacedName.Name == "" {
		return nil
	}

	if namespacedName.Namespace == "" {
		namespacedName.Namespace = config.Namespace
	}
	upstreamTLSConfig.SecretName = namespacedName.String()

	return nil
}

func (u upstreamTLS) ApplyTrafficPolicy(trafficPolicy *networking.TrafficPolicy, portTrafficPolicy *networking.TrafficPolicy_PortTrafficPolicy, config *Ingress) {
	if config.UpstreamTLS == nil {
		return
	}

	upstreamTLSConfig := config.UpstreamTLS

	var connectionPool *networking.ConnectionPoolSettings
	if isH2(upstreamTLSConfig.BackendProtocol) {
		connectionPool = &networking.ConnectionPoolSettings{
			Http: &networking.ConnectionPoolSettings_HTTPSettings{
				H2UpgradePolicy: networking.ConnectionPoolSettings_HTTPSettings_UPGRADE,
			},
		}
	}

	var tls *networking.ClientTLSSettings
	if upstreamTLSConfig.SecretName != "" {
		// MTLS
		tls = processMTLS(config)
	} else if isHTTPS(upstreamTLSConfig.BackendProtocol) {
		tls = processSimple(config)
	}
	if trafficPolicy != nil {
		trafficPolicy.ConnectionPool = connectionPool
		trafficPolicy.Tls = tls
	}
	if portTrafficPolicy != nil {
		portTrafficPolicy.ConnectionPool = connectionPool
		portTrafficPolicy.Tls = tls
	}
}

func processMTLS(config *Ingress) *networking.ClientTLSSettings {
	namespacedName := util.SplitNamespacedName(config.UpstreamTLS.SecretName)
	if namespacedName.Name == "" {
		return nil
	}

	tls := &networking.ClientTLSSettings{
		Mode:           networking.ClientTLSSettings_MUTUAL,
		CredentialName: credentials.ToKubernetesIngressResource(config.RawClusterId, namespacedName.Namespace, namespacedName.Name),
	}

	if !config.UpstreamTLS.SSLVerify {
		// This api InsecureSkipVerify hasn't been support yet.
		// Until this pr https://github.com/istio/istio/pull/35357.
		tls.InsecureSkipVerify = &wrappers.BoolValue{
			Value: false,
		}
	}

	if config.UpstreamTLS.EnableSNI && config.UpstreamTLS.SNI != "" {
		tls.Sni = config.UpstreamTLS.SNI
	}

	return tls
}

func processSimple(config *Ingress) *networking.ClientTLSSettings {
	tls := &networking.ClientTLSSettings{
		Mode: networking.ClientTLSSettings_SIMPLE,
	}

	if config.UpstreamTLS.EnableSNI && config.UpstreamTLS.SNI != "" {
		tls.Sni = config.UpstreamTLS.SNI
	}

	return tls
}

func needUpstreamTLSConfig(annotations Annotations) bool {
	return annotations.HasASAP(backendProtocol) ||
		annotations.HasASAP(proxySSLSecret)
}

func onOffToBool(onOff string) bool {
	return onOff == "on"
}

func isH2(protocol string) bool {
	return protocol == "HTTP2" ||
		protocol == "GRPC" ||
		protocol == "GRPCS"
}

func isHTTPS(protocol string) bool {
	return protocol == "HTTPS" ||
		protocol == "GRPCS"
}
