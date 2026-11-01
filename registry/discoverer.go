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

package registry

import (
	"context"
	"sync"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
)

const (
	// Custom is a generic ServiceRegistryType backed by a Discoverer registered
	// via RegisterDiscoverer. Custom discoverers may also register under their
	// own arbitrary type strings.
	Custom ServiceRegistryType = "custom"

	// InstanceLabel is the target label that carries the target address in the
	// form "host:port". It mirrors Prometheus' "__address__" convention.
	InstanceLabel = "__instance__"
	// ProtocolLabel is the target/group label that carries the upstream protocol
	// (e.g. http, grpc, dubbo). It defaults to HTTP when absent.
	ProtocolLabel = "protocol"
	// ServiceLabel is the group label that names the discovered service. It is
	// the primary way a discoverer identifies the service host.
	ServiceLabel = "job"
	// ServiceLabelAlias is an alternative group label for naming the service. It
	// takes precedence over ServiceLabel when both are present.
	ServiceLabelAlias = "__service__"
)

// LabelSet is a set of key/value labels describing a discovered target or a
// group of targets. It is the native equivalent of Prometheus'
// model.LabelSet and is used so that the registry package does not need to
// depend on the Prometheus modules directly.
type LabelSet map[string]string

// TargetGroup describes a set of discovered service targets that share common
// labels. It mirrors the structure of Prometheus' discovery targetgroup.Group
// so that custom discovery mechanisms (for example xDS or HTTP watch based
// ones) can feed service information into Higress through a standard interface.
type TargetGroup struct {
	// Targets is the list of discovered targets. Every target LabelSet should
	// carry the "__instance__" key with the target address in "host:port" form.
	// Any other labels (e.g. "hostname", "protocol") are attached to the
	// corresponding workload entry.
	Targets []LabelSet
	// Labels are labels shared by every target in this group (e.g. "job"). The
	// "job" (or "__service__") label names the resulting service.
	Labels LabelSet
	// Source uniquely identifies the group within a single discoverer. When no
	// "job"/"__service__" label is present it is used as the service name.
	Source string
}

// Discoverer discovers groups of service targets and reports the full current
// set of groups to the provided channel whenever it changes. A Discoverer must
// block until ctx is cancelled, sending the complete target set on every
// update (a snapshot, not a delta). This is the same contract as the
// Prometheus discovery.Discoverer interface.
//
// Users can add custom registration centers (e.g. xDS or HTTP watch based) by
// implementing this interface and registering a factory with
// RegisterDiscoverer.
type Discoverer interface {
	Run(ctx context.Context, up chan<- []*TargetGroup)
}

// DiscovererFactory builds a Discoverer from a registry configuration. It
// receives the full RegistryConfig (so custom discoverers can reuse fields such
// as Domain, Port, Protocol and Metadata) together with the resolved
// AuthOption.
type DiscovererFactory func(registry *apiv1.RegistryConfig, auth AuthOption) (Discoverer, error)

var (
	discovererFactoriesMu sync.RWMutex
	discovererFactories   = make(map[string]DiscovererFactory)
)

// RegisterDiscoverer registers a DiscovererFactory under the given registry
// type. It is intended to be called from package init() of packages providing
// custom discovery implementations (for example xDS or HTTP watch based).
// Registering the same type twice or registering with an empty type or nil
// factory panics, mirroring the standard library registration conventions
// (e.g. database/sql.Register, image.RegisterFormat).
func RegisterDiscoverer(registryType string, factory DiscovererFactory) {
	if registryType == "" {
		panic("registry: RegisterDiscoverer called with empty type")
	}
	if factory == nil {
		panic("registry: RegisterDiscoverer called with nil factory for type " + registryType)
	}
	discovererFactoriesMu.Lock()
	defer discovererFactoriesMu.Unlock()
	if _, exists := discovererFactories[registryType]; exists {
		panic("registry: RegisterDiscoverer called twice for type " + registryType)
	}
	discovererFactories[registryType] = factory
}

// LookupDiscovererFactory returns the DiscovererFactory registered for the
// given registry type, or nil if none has been registered.
func LookupDiscovererFactory(registryType string) DiscovererFactory {
	discovererFactoriesMu.RLock()
	defer discovererFactoriesMu.RUnlock()
	return discovererFactories[registryType]
}
