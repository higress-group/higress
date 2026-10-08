// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

package services

import (
	"fmt"
	"net"
	"net/url"
)

func BuildAIProviderServiceBody(name, url string) map[string]interface{} {
	customBaseURL := fmt.Sprintf("%s/compatible-mode/v1", url)
	return map[string]interface{}{
		"type":     "openai",
		"name":     name,
		"tokens":   []string{},
		"version":  0,
		"protocol": "openai/v1",
		"tokenFailoverConfig": map[string]interface{}{
			"enabled": false,
		},
		"proxyName": "",
		"rawConfigs": map[string]interface{}{
			"openaiExtraCustomUrls": []string{},
			"openaiCustomUrl":       customBaseURL,
		},
	}
}

func BuildAddAIRouteBody(name, _url string) map[string]interface{} {
	return map[string]interface{}{
		"name": fmt.Sprintf("%s-route", name),
		// "version": "627198", // It's unnecessary to provide when create a new one
		"domains": []interface{}{},
		"pathPredicate": map[string]interface{}{
			"matchType": "PRE",
			// FIXME: Currently, to use model API in higress user hould follow this pattern:
			// http://<higress-gateway-ip>/<PRE_MATCH_VALUE>/v1/chat/completions or /v1/embedding
			// However in Himarket, when connecting the higress ai route as model API, himarket will directly use http://<higress-gateway-ip>/<PRE_MATCH_VALUE>
			// as the final request url, which will not get to right path. So here we make the matchValue hard-coded as `/v1/chat/completions`
			"matchValue":    "/v1/chat/completions",
			"caseSensitive": false,
			"ignoreCase":    []string{}, // "ignoreCase": ["ignore"]
		},
		"headerPredicates":   []interface{}{},
		"urlParamPredicates": []interface{}{},
		"upstreams": []interface{}{
			map[string]interface{}{
				"provider":     name,
				"weight":       100,
				"modelMapping": map[string]interface{}{},
			},
		},
		"modelPredicates": []interface{}{},
		"authConfig": map[string]interface{}{
			"enabled":                false,
			"allowedCredentialTypes": []interface{}{},
			"allowedConsumers":       []interface{}{},
		},
		"fallbackConfig": map[string]interface{}{
			"enabled":          false,
			"upstreams":        nil,
			"fallbackStrategy": nil,
			"responseCodes":    nil,
		},
	}
}

func BuildServiceBodyAndSrv(name, urlStr string) (map[string]interface{}, string, string, error) {
	res, err := url.Parse(urlStr)
	if err != nil {
		return nil, "", "", err
	}

	// add service source
	srvType := ""
	srvPort := ""

	if ip := net.ParseIP(res.Hostname()); ip == nil {
		srvType = "dns"
	} else {
		srvType = "static"
	}

	if res.Port() == "" && res.Scheme == "http" {
		srvPort = "80"
	} else if res.Port() == "" && res.Scheme == "https" {
		srvPort = "443"
	} else {
		srvPort = res.Port()
	}

	// e.g. hgctl-mcp-deepwiki.dns
	targetSrvName := fmt.Sprintf("%s.%s", name, srvType)

	return map[string]interface{}{
		"domain":        res.Host,
		"type":          srvType,
		"port":          srvPort,
		"name":          name,
		"proxyName":     "",
		"domainForEdit": res.Host,
		"protocol":      res.Scheme,
	}, targetSrvName, srvPort, nil
}

func BuildAPIRouteBody(name, srv string) map[string]interface{} {
	return map[string]interface{}{
		"name": fmt.Sprintf("%s-route", name),
		"path": map[string]interface{}{
			"matchType":     "PRE",      // default is PREFIX
			"matchValue":    "/process", // default is "/process"
			"caseSensitive": true,
		},
		"authConfig": map[string]interface{}{
			"enabled": false,
		},
		"services": []map[string]interface{}{
			{
				"name": srv,
			},
		},
	}
}

func BuildAddHigressInstanceBody(name, addr, username, password string) map[string]interface{} {
	return map[string]interface{}{
		"gatewayName": name,
		"gatewayType": "HIGRESS",
		"higressConfig": map[string]interface{}{
			"address":  addr,
			"username": username,
			"password": password,
		},
	}
}

func BuildAPIProductBody(name, desc, typ string) map[string]interface{} {
	return map[string]interface{}{
		"name": name, "description": desc, "type": typ,
	}
}

func BuildRefModelAPIProductBody(gateway_id, product_id, target_route string) map[string]interface{} {
	return map[string]interface{}{
		"gatewayId":  gateway_id,
		"sourceType": "GATEWAY",
		"productId":  product_id,
		"higressRefConfig": map[string]interface{}{
			"modelRouteName":  target_route,
			"fromGatewayType": "HIGRESS",
		},
	}
}

func BuildRefMCPAPIProductBody(gateway_id, product_id, mcp_name string) map[string]interface{} {
	return map[string]interface{}{
		"gatewayId":  gateway_id,
		"sourceType": "GATEWAY",
		"productId":  product_id,
		"higressRefConfig": map[string]interface{}{
			"mcpServerName":   mcp_name,
			"fromGatewayType": "HIGRESS",
		},
	}
}
