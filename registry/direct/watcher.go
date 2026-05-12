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

package direct

import (
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/log"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	"github.com/alibaba/higress/v2/pkg/common"
	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	"github.com/alibaba/higress/v2/registry"
	provider "github.com/alibaba/higress/v2/registry"
	"github.com/alibaba/higress/v2/registry/memory"
	"github.com/go-errors/errors"
)

type watcher struct {
	provider.BaseWatcher
	apiv1.RegistryConfig
	cache memory.Cache
	mutex sync.Mutex
}

type WatcherOption func(w *watcher)

func NewWatcher(cache memory.Cache, opts ...WatcherOption) (provider.Watcher, error) {
	w := &watcher{
		cache: cache,
	}
	for _, opt := range opts {
		opt(w)
	}
	if common.ParseProtocol(w.Protocol) == common.Unsupported {
		return nil, errors.Errorf("invalid protocol:%s", w.Protocol)
	}
	return w, nil
}

func WithType(t string) WatcherOption {
	return func(w *watcher) {
		w.Type = t
	}
}

func WithName(name string) WatcherOption {
	return func(w *watcher) {
		w.Name = name
	}
}

func WithDomain(domain string) WatcherOption {
	return func(w *watcher) {
		w.Domain = domain
	}
}

func WithPort(port uint32) WatcherOption {
	return func(w *watcher) {
		w.Port = port
	}
}

func WithProtocol(protocol string) WatcherOption {
	return func(w *watcher) {
		w.Protocol = protocol
		if w.Protocol == "" {
			w.Protocol = string(common.HTTP)
		}
	}
}

func WithSNI(sni string) WatcherOption {
	return func(w *watcher) {
		w.Sni = sni
	}
}

func WithProxyName(proxyName string) WatcherOption {
	return func(w *watcher) {
		w.ProxyName = proxyName
	}
}

func (w *watcher) Run() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	host := strings.Join([]string{w.Name, w.Type}, common.DotSeparator)
	serviceEntry := w.generateServiceEntry(host)
	if serviceEntry != nil {
		var destinationRuleWrapper *ingress.WrapperDestinationRule
		destinationRule := w.generateDestinationRule(serviceEntry)
		if destinationRule != nil {
			destinationRuleWrapper = &ingress.WrapperDestinationRule{
				DestinationRule: destinationRule,
				ServiceKey:      ingress.CreateMcpServiceKey(host, int32(w.Port)),
			}
		}
		proxyConfig := w.generateProxyConfig(serviceEntry)
		w.cache.UpdateServiceWrapper(host, &ingress.ServiceWrapper{
			ServiceName:            w.Name,
			ServiceEntry:           serviceEntry,
			Suffix:                 w.Type,
			RegistryType:           w.Type,
			RegistryName:           w.Name,
			ProxyConfig:            proxyConfig,
			DestinationRuleWrapper: destinationRuleWrapper,
		})
		w.UpdateService()
	}
	w.Ready(true)
}

func (w *watcher) Stop() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	host := strings.Join([]string{w.Name, w.Type}, common.DotSeparator)
	w.cache.DeleteServiceWrapper(host)
	w.Ready(false)
}

var domainRegex = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$`)

func (w *watcher) generateServiceEntry(host string) *v1alpha3.ServiceEntry {
	endpoints := make([]*v1alpha3.WorkloadEntry, 0)
	protocol := string(common.ParseProtocol(w.Protocol))
	for _, ep := range strings.Split(w.Domain, common.CommaSeparator) {
		var endpoint *v1alpha3.WorkloadEntry
		if w.Type == string(registry.Static) {
			var ip string
			var portStr string
			// Support IPv6 format: [2001:db8::1]:8080
			if strings.HasPrefix(ep, "[") {
				// IPv6 format: [IPv6]:port
				lastBracket := strings.LastIndex(ep, "]")
				if lastBracket == -1 {
					log.Errorf("invalid IPv6 endpoint format:%s with static type", ep)
					return nil
				}
				ip = ep[1:lastBracket] // Extract IPv6 address without brackets
				if lastBracket+1 >= len(ep) || ep[lastBracket+1] != ':' {
					log.Errorf("invalid IPv6 endpoint format:%s with static type, missing colon after bracket", ep)
					return nil
				}
				portStr = ep[lastBracket+2:] // Extract port after "]:"
			} else {
				// IPv4 format: 192.168.1.1:8080
				pair := strings.Split(ep, common.ColonSeparator)
				if len(pair) != 2 {
					log.Errorf("invalid endpoint:%s with static type", ep)
					return nil
				}
				ip = pair[0]
				portStr = pair[1]
			}
			port, err := strconv.ParseUint(portStr, 10, 32)
			if err != nil {
				log.Errorf("invalid port:%s of endpoint:%s", portStr, ep)
				return nil
			}
			if net.ParseIP(ip) == nil {
				log.Errorf("invalid ip:%s of endpoint:%s", ip, ep)
				return nil
			}
			endpoint = &v1alpha3.WorkloadEntry{
				Address: ip,
				Ports:   map[string]uint32{protocol: uint32(port)},
			}
		} else if w.Type == string(registry.DNS) {
			if !domainRegex.MatchString(ep) {
				log.Errorf("invalid domain format:%s", ep)
				return nil
			}
			endpoint = &v1alpha3.WorkloadEntry{
				Address: ep,
			}
		} else {
			log.Errorf("unknown direct service type:%s", w.Type)
			return nil
		}
		endpoints = append(endpoints, endpoint)
	}
	if len(endpoints) == 0 {
		log.Errorf("empty endpoints will not be pushed, host:%s", host)
		return nil
	}
	var ports []*v1alpha3.ServicePort
	ports = append(ports, &v1alpha3.ServicePort{
		Number:   w.Port,
		Name:     protocol,
		Protocol: protocol,
	})
	se := &v1alpha3.ServiceEntry{
		Hosts:     []string{host},
		Ports:     ports,
		Location:  v1alpha3.ServiceEntry_MESH_INTERNAL,
		Endpoints: endpoints,
	}
	if w.Type == string(registry.Static) {
		se.Resolution = v1alpha3.ServiceEntry_STATIC
	} else if w.Type == string(registry.DNS) {
		se.Resolution = v1alpha3.ServiceEntry_DNS
	}
	return se
}

func (w *watcher) generateDestinationRule(se *v1alpha3.ServiceEntry) *v1alpha3.DestinationRule {
	if !common.Protocol(se.Ports[0].Protocol).IsHTTPS() {
		return nil
	}
	sni := w.getSni(se)
	return &v1alpha3.DestinationRule{
		Host: se.Hosts[0],
		TrafficPolicy: &v1alpha3.TrafficPolicy{
			PortLevelSettings: []*v1alpha3.TrafficPolicy_PortTrafficPolicy{
				{
					Port: &v1alpha3.PortSelector{
						Number: se.Ports[0].Number,
					},
					Tls: &v1alpha3.ClientTLSSettings{
						Mode: v1alpha3.ClientTLSSettings_SIMPLE,
						Sni:  sni,
					},
				},
			},
		},
	}
}

func (w *watcher) generateProxyConfig(entry *v1alpha3.ServiceEntry) *ingress.ServiceProxyConfig {
	if w.ProxyName == "" {
		return nil
	}
	return &ingress.ServiceProxyConfig{
		ProxyName:        w.ProxyName,
		UpstreamProtocol: common.ParseProtocol(entry.Ports[0].Protocol),
		UpstreamSni:      w.getSni(entry),
	}
}

func (w *watcher) getSni(se *v1alpha3.ServiceEntry) string {
	sni := w.Sni
	// DNS type, automatically sets SNI based on domain name.
	if sni == "" && w.Type == string(registry.DNS) && len(se.Endpoints) == 1 {
		sni = w.Domain
	}
	return sni
}

func (w *watcher) GetRegistryType() string {
	return w.RegistryConfig.Type
}
