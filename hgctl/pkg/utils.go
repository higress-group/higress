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

package hgctl

import (
	"encoding/json"
	"errors"
	"fmt"
)

type envoyConfigType string

var (
	BootstrapEnvoyConfigType envoyConfigType = "bootstrap"
	ClusterEnvoyConfigType   envoyConfigType = "cluster"
	EndpointEnvoyConfigType  envoyConfigType = "endpoint"
	ListenerEnvoyConfigType  envoyConfigType = "listener"
	RouteEnvoyConfigType     envoyConfigType = "route"
	AllEnvoyConfigType       envoyConfigType = "all"
)

func GetXDSResource(resourceType envoyConfigType, configDump []byte) (any, error) {
	cd := map[string]any{}
	if err := json.Unmarshal(configDump, &cd); err != nil {
		return nil, fmt.Errorf("decode config dump: %w", err)
	}
	if resourceType == AllEnvoyConfigType {
		return cd, nil
	}
	configs, ok := cd["configs"]
	if !ok {
		return nil, errors.New("config dump is missing configs")
	}
	globalConfigs, ok := configs.([]any)
	if !ok {
		return nil, errors.New("config dump configs must be an array")
	}

	var typeURL string
	switch resourceType {
	case BootstrapEnvoyConfigType:
		typeURL = "type.googleapis.com/envoy.admin.v3.BootstrapConfigDump"
	case EndpointEnvoyConfigType:
		typeURL = "type.googleapis.com/envoy.admin.v3.EndpointsConfigDump"
	case ClusterEnvoyConfigType:
		typeURL = "type.googleapis.com/envoy.admin.v3.ClustersConfigDump"
	case ListenerEnvoyConfigType:
		typeURL = "type.googleapis.com/envoy.admin.v3.ListenersConfigDump"
	case RouteEnvoyConfigType:
		typeURL = "type.googleapis.com/envoy.admin.v3.RoutesConfigDump"
	default:
		return nil, fmt.Errorf("unknown resource type %q", resourceType)
	}

	for i, config := range globalConfigs {
		configObject, ok := config.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("config dump configs[%d] must be an object", i)
		}
		if configObject["@type"] == typeURL {
			return config, nil
		}
	}

	return nil, fmt.Errorf("config dump is missing %s resource", resourceType)
}
