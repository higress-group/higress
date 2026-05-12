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

package tools

import (
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/server"
)

func LoadTools(server *mcp.MCPServer) server.Server {
	return server.AddMCPTool("maps_geo", &GeoRequest{}).
		AddMCPTool("maps_bicycling", &BicyclingRequest{}).
		AddMCPTool("maps_direction_transit_integrated", &TransitIntegratedRequest{}).
		AddMCPTool("maps_ip_location", &IPLocationRequest{}).
		AddMCPTool("maps_weather", &WeatherRequest{}).
		AddMCPTool("maps_direction_driving", &DrivingRequest{}).
		AddMCPTool("maps_around_search", &AroundSearchRequest{}).
		AddMCPTool("maps_search_detail", &SearchDetailRequest{}).
		AddMCPTool("maps_regeocode", &ReGeocodeRequest{}).
		AddMCPTool("maps_text_search", &TextSearchRequest{}).
		AddMCPTool("maps_distance", &DistanceRequest{}).
		AddMCPTool("maps_direction_walking", &WalkingRequest{})
}
