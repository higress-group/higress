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
		return nil, err
	}
	if resourceType == AllEnvoyConfigType {
		return cd, nil
	}
	globalConfigs, ok := cd["configs"].([]any)
	if !ok {
		return nil, fmt.Errorf("invalid config dump: configs field is missing or not an array")
	}

	switch resourceType {
	case BootstrapEnvoyConfigType:
		for _, config := range globalConfigs {
			m, ok := config.(map[string]interface{})
			if !ok {
				continue
			}
			if m["@type"] == "type.googleapis.com/envoy.admin.v3.BootstrapConfigDump" {
				return config, nil
			}
		}
	case EndpointEnvoyConfigType:
		for _, config := range globalConfigs {
			m, ok := config.(map[string]interface{})
			if !ok {
				continue
			}
			if m["@type"] == "type.googleapis.com/envoy.admin.v3.EndpointsConfigDump" {
				return config, nil
			}
		}

	case ClusterEnvoyConfigType:
		for _, config := range globalConfigs {
			m, ok := config.(map[string]interface{})
			if !ok {
				continue
			}
			if m["@type"] == "type.googleapis.com/envoy.admin.v3.ClustersConfigDump" {
				return config, nil
			}
		}
	case ListenerEnvoyConfigType:
		for _, config := range globalConfigs {
			m, ok := config.(map[string]interface{})
			if !ok {
				continue
			}
			if m["@type"] == "type.googleapis.com/envoy.admin.v3.ListenersConfigDump" {
				return config, nil
			}
		}
	case RouteEnvoyConfigType:
		for _, config := range globalConfigs {
			m, ok := config.(map[string]interface{})
			if !ok {
				continue
			}
			if m["@type"] == "type.googleapis.com/envoy.admin.v3.RoutesConfigDump" {
				return config, nil
			}
		}
	default:
		return nil, fmt.Errorf("unknown resourceType %s", resourceType)
	}

	return nil, fmt.Errorf("unknown resourceType %s", resourceType)
}
