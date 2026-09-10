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

// Package custom adapts a registry.Discoverer to the registry.Watcher
// interface so that custom service-discovery implementations (e.g. xDS or HTTP
// watch based) can feed services into Higress through the standard Reconciler
// pipeline, exactly like the built-in nacos/consul/eureka/zookeeper registries.
package custom

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/log"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	"github.com/alibaba/higress/v2/pkg/common"
	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	"github.com/alibaba/higress/v2/registry"
	"github.com/alibaba/higress/v2/registry/memory"
)

// watcher adapts a registry.Discoverer to the registry.Watcher interface.
type watcher struct {
	registry.BaseWatcher
	apiv1.RegistryConfig

	cache      memory.Cache
	discoverer registry.Discoverer

	mutex  sync.Mutex
	cancel context.CancelFunc
	hosts  map[string]struct{}
}

// NewWatcher builds a registry.Watcher backed by the Discoverer registered for
// cfg.Type via registry.RegisterDiscoverer. The returned watcher converts every
// discovery update into ingress.ServiceWrapper entries stored in cache.
func NewWatcher(cache memory.Cache, cfg *apiv1.RegistryConfig, auth registry.AuthOption) (registry.Watcher, error) {
	if cfg == nil {
		return nil, fmt.Errorf("custom watcher: nil registry config")
	}
	factory := registry.LookupDiscovererFactory(cfg.Type)
	if factory == nil {
		return nil, fmt.Errorf("custom watcher: no discoverer registered for type %q", cfg.Type)
	}
	discoverer, err := factory(cfg, auth)
	if err != nil {
		return nil, fmt.Errorf("custom watcher: build discoverer for type %q: %w", cfg.Type, err)
	}
	return &watcher{
		cache:      cache,
		discoverer: discoverer,
		hosts:      make(map[string]struct{}),
	}, nil
}

// Run starts the underlying discoverer and keeps the cache in sync with the
// reported target groups until Stop is called.
func (w *watcher) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	w.mutex.Lock()
	w.cancel = cancel
	w.mutex.Unlock()

	up := make(chan []*registry.TargetGroup, 1)
	go w.discoverer.Run(ctx, up)

	w.Ready(true)
	for {
		select {
		case groups, ok := <-up:
			if !ok {
				return
			}
			if w.apply(groups) {
				w.UpdateService()
			}
		case <-ctx.Done():
			return
		}
	}
}

// Stop cancels the discoverer and removes every service owned by this watcher
// from the cache.
func (w *watcher) Stop() {
	w.mutex.Lock()
	cancel := w.cancel
	w.cancel = nil
	hosts := w.hosts
	w.hosts = make(map[string]struct{})
	w.mutex.Unlock()

	if cancel != nil {
		cancel()
	}

	for host := range hosts {
		w.cache.DeleteServiceWrapper(host)
	}
	w.UpdateService()
	w.Ready(false)
}

func (w *watcher) GetRegistryType() string {
	return w.Type
}

// apply reconciles the full snapshot of target groups with the cache. It returns
// true when any service was added, updated or removed.
func (w *watcher) apply(groups []*registry.TargetGroup) bool {
	type update struct {
		host string
		sew  *ingress.ServiceWrapper
	}

	updates := make([]update, 0, len(groups))
	newHosts := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		sew, host := toServiceWrapper(group, w.Name, w.Type)
		if sew == nil {
			continue
		}
		updates = append(updates, update{host: host, sew: sew})
		newHosts[host] = struct{}{}
	}

	w.mutex.Lock()
	var deleted []string
	for host := range w.hosts {
		if _, ok := newHosts[host]; !ok {
			deleted = append(deleted, host)
		}
	}
	w.hosts = newHosts
	w.mutex.Unlock()

	for _, u := range updates {
		w.cache.UpdateServiceWrapper(u.host, u.sew)
	}
	for _, host := range deleted {
		w.cache.DeleteServiceWrapper(host)
	}

	return len(updates) > 0 || len(deleted) > 0
}

// toServiceWrapper converts a single registry.TargetGroup into an
// ingress.ServiceWrapper together with the cache host key. It returns a nil
// wrapper when the group cannot be converted (no service name or no valid
// target).
func toServiceWrapper(group *registry.TargetGroup, registryName, registryType string) (*ingress.ServiceWrapper, string) {
	host := groupHost(group)
	if host == "" {
		log.Warnf("custom discoverer: target group has no service name (job/__service__ label or source), skipping")
		return nil, ""
	}

	endpoints := make([]*v1alpha3.WorkloadEntry, 0, len(group.Targets))
	servicePorts := make(map[string]uint32)
	portOrder := make([]string, 0)

	for _, target := range group.Targets {
		instance := target[registry.InstanceLabel]
		address, portStr, ok := splitHostPort(instance)
		if !ok {
			log.Warnf("custom discoverer: invalid __instance__ %q, skipping target", instance)
			continue
		}
		port, err := strconv.ParseUint(portStr, 10, 32)
		if err != nil || port == 0 || port > 65535 {
			log.Warnf("custom discoverer: invalid port %q in __instance__ %q, skipping target", portStr, instance)
			continue
		}

		protocol := common.ParseProtocol(firstNonEmpty(target[registry.ProtocolLabel], group.Labels[registry.ProtocolLabel]))
		if protocol.IsUnsupported() {
			protocol = common.HTTP
		}

		labels := make(map[string]string, len(group.Labels)+len(target))
		mergeLabels(labels, group.Labels)
		mergeLabels(labels, target)
		delete(labels, registry.InstanceLabel)

		endpoints = append(endpoints, &v1alpha3.WorkloadEntry{
			Address: address,
			Ports:   map[string]uint32{protocol.String(): uint32(port)},
			Labels:  labels,
		})

		if _, exists := servicePorts[protocol.String()]; !exists {
			servicePorts[protocol.String()] = uint32(port)
			portOrder = append(portOrder, protocol.String())
		}
	}

	if len(endpoints) == 0 {
		return nil, ""
	}

	ports := make([]*v1alpha3.ServicePort, 0, len(portOrder))
	for _, proto := range portOrder {
		ports = append(ports, &v1alpha3.ServicePort{
			Number:   servicePorts[proto],
			Name:     proto,
			Protocol: proto,
		})
	}

	serviceEntry := &v1alpha3.ServiceEntry{
		Hosts:      []string{host},
		Ports:      ports,
		Location:   v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: v1alpha3.ServiceEntry_STATIC,
		Endpoints:  endpoints,
	}

	return &ingress.ServiceWrapper{
		ServiceName:  host,
		ServiceEntry: serviceEntry,
		Suffix:       registryType,
		RegistryType: registryType,
		RegistryName: registryName,
	}, host
}

// groupHost resolves the service name (and cache key) for a target group.
func groupHost(group *registry.TargetGroup) string {
	if group == nil {
		return ""
	}
	if name := group.Labels[registry.ServiceLabelAlias]; name != "" {
		return name
	}
	if name := group.Labels[registry.ServiceLabel]; name != "" {
		return name
	}
	return group.Source
}

// splitHostPort splits an "__instance__" value into host and port. It supports
// both IPv4 ("1.2.3.4:80") and IPv6 ("[2001:db8::1]:80") forms.
func splitHostPort(instance string) (host, port string, ok bool) {
	if instance == "" {
		return "", "", false
	}
	if strings.HasPrefix(instance, "[") {
		last := strings.LastIndex(instance, "]")
		if last < 0 {
			return "", "", false
		}
		host = instance[1:last]
		rest := instance[last+1:]
		if !strings.HasPrefix(rest, ":") || len(rest) <= 1 {
			return "", "", false
		}
		return host, rest[1:], true
	}
	// IPv4 host:port.
	idx := strings.LastIndex(instance, ":")
	if idx <= 0 || idx == len(instance)-1 {
		return "", "", false
	}
	if strings.Count(instance, ":") != 1 {
		// Bare IPv6 address without brackets/port is not a usable target.
		return "", "", false
	}
	return instance[:idx], instance[idx+1:], true
}

func mergeLabels(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
