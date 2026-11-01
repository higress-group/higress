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
	"encoding/json"
	"fmt"
	"time"

	"github.com/alibaba/higress/hgctl/pkg/agent/common"
)

func HandleAddServiceSource(client *HigressClient, body interface{}) ([]byte, error) {
	data, ok := body.(map[string]interface{})
	// fmt.Printf("request body: %v\n", data)
	if !ok {
		return nil, fmt.Errorf("failed to parse request body")
	}
	// Validate
	if _, ok := data["name"]; !ok {
		return nil, fmt.Errorf("missing required field 'name' in body")
	}
	if _, ok := data["type"]; !ok {
		return nil, fmt.Errorf("missing required field 'type' in body")
	}
	if _, ok := data["domain"]; !ok {
		return nil, fmt.Errorf("missing required field 'domain' in body")
	}
	if _, ok := data["port"]; !ok {
		return nil, fmt.Errorf("missing required field 'port' in body")
	}

	resp, err := client.Post("/v1/service-sources", data)
	if err != nil {
		return nil, fmt.Errorf("failed to add service source: %w", err)
	}
	// res := make(map[string]interface{})

	return resp, nil
}

// add MCP server to higress console, example request body as followed:
//
//	{
//	  "name": "test",
//	  "description": "123",
//	  "type": "DIRECT_ROUTE",
//	  "services": [
//	    {
//	      "name": "hgctl-mcp-deepwiki.dns",
//	      "port": 443,
//	      "version": "1.0",
//	      "weight": 100
//	    }
//	  ],
//	  "consumerAuthInfo": {
//	    "type": "key-auth",
//	    "allowedConsumers": []
//	  },
//	  "domains": [],
//	  "directRouteConfig": {
//	    "path": "/mcp",
//	    "transportType": "streamable"
//	  }
//	}
func HandleAddMCPServer(client *HigressClient, body interface{}) ([]byte, error) {
	data, ok := body.(map[string]interface{})
	// fmt.Printf("mcpbody: %v\n", data)
	if !ok {
		return nil, fmt.Errorf("failed to parse request body")
	}
	// Validate
	if _, ok := data["name"]; !ok {
		return nil, fmt.Errorf("missing required field 'name' in body")
	}
	if _, ok := data["type"]; !ok {
		return nil, fmt.Errorf("missing required field 'type' in body")
	}
	// if _, ok := data["upstreamPathPrefix"]; !ok {
	// 	return nil, fmt.Errorf("missing required field 'upstreamPathPrefix' in body")
	// }

	_, ok = data["services"]
	if !ok {
		return nil, fmt.Errorf("missing required field 'port' in body")
	}

	resp, err := client.Put("/v1/mcpServer", data)
	if err != nil {
		return nil, fmt.Errorf("failed to add mcp server: %w", err)
	}

	return resp, nil
}

// return map[mcp-server-name]{}
func GetExistingMCPServers(client *HigressClient) (map[string]string, error) {
	result := make(map[string]string)
	data, err := HandleListMCPServers(client)
	if err != nil {
		return nil, err
	}
	var response map[string]interface{}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to get product id from response: %s", err)
	}

	// fmt.Println(response["data"])

	if list, ok := response["data"].([]interface{}); ok {
		for _, item := range list {
			if mcp, ok := item.(map[string]interface{}); ok {
				if name, ok := mcp["name"].(string); ok {
					result[name] = ""
				}
			}
		}

	}
	return result, nil
}

func HandleListMCPServers(client *HigressClient) ([]byte, error) {
	ts := time.Now().Unix()
	pageNum := 1
	pageSize := 100
	return client.Get(fmt.Sprintf("/v1/mcpServer?ts=%d&pageNum=%d&pageSize=%d", ts, pageNum, pageSize))
}

// add OpenAPI MCP tools to higress console, example request body:
//
//	{
//	  "id": null,
//	  "name": "openapi-name",
//	  "description": "123",
//	  "domains": [],
//	  "services": [
//	    {
//	      "name": "kubernetes.default.svc.cluster.local",
//	      "port": 443,
//	      "version": null,
//	      "weight": 100
//	    }
//	  ],
//	  "type": "OPEN_API",
//	  "consumerAuthInfo": {
//	    "type": "key-auth",
//	    "enable": false,
//	    "allowedConsumers": []
//	  },
//	  "rawConfigurations": "", // MCP configuration str
//	  "dsn": null,
//	  "dbType": null,
//	  "upstreamPathPrefix": null,
//	  "mcpServerName": "openapi-name"
//	}
func HandleAddOpenAPITool(client *HigressClient, body interface{}) ([]byte, error) {
	return client.Put("/v1/mcpServer", body)
}

func HandleAddAIProviderService(client *HigressClient, body interface{}) ([]byte, error) {
	return client.Post("/v1/ai/providers", body)

}

func HandleAddAIRoute(client *HigressClient, body interface{}) ([]byte, error) {
	return client.Post("/v1/ai/routes", body)
}

func HandleAddRoute(client *HigressClient, body interface{}) ([]byte, error) {
	return client.Post("/v1/routes", body)
}

// Himarket-related
func HandleAddHigressInstance(client *HimarketClient, body interface{}) ([]byte, error) {
	// This api will not return the higress-gateway-id
	return client.Post("/api/v1/gateways", body)
}

func (c *HimarketClient) getProduct(typ common.ProductType) ([]byte, error) {
	return c.Get(fmt.Sprintf("/api/v1/products?type=%s&page=0&size=30", string(typ)))
}

func (c *HimarketClient) extractGetProductResponse(typ common.ProductType, response map[string]interface{}) map[string]string {
	result := make(map[string]string)

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		return result
	}

	content, ok := data["content"].([]interface{})
	if !ok {
		return result
	}

	for _, item := range content {
		product, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		productType, _ := product["type"].(string)
		if productType != string(typ) {
			continue
		}

		name, _ := product["name"].(string)
		if name == "" {
			continue
		}

		mcpConfig, ok := product["mcpConfig"].(map[string]interface{})
		if !ok {
			continue
		}

		serverConfig, ok := mcpConfig["mcpServerConfig"].(map[string]interface{})
		if !ok {
			continue
		}

		domains, ok := serverConfig["domains"].([]interface{})
		if !ok || len(domains) == 0 {
			continue
		}

		path, ok := serverConfig["path"].(string)
		if !ok {
			continue
		}

		for _, domainItem := range domains {
			domainConfig, ok := domainItem.(map[string]interface{})
			if !ok {
				continue
			}

			domain, _ := domainConfig["domain"].(string)
			protocol, _ := domainConfig["protocol"].(string)
			if domain == "" || protocol == "" {
				continue
			}

			port, _ := domainConfig["port"].(float64)
			url := ""
			if port == 0 || port == 80 {
				url = fmt.Sprintf("%s://%s%s", protocol, domain, path)
			} else {
				url = fmt.Sprintf("%s://%s:%d%s", protocol, domain, int(port), path)
			}

			result[name] = url
			break
		}
	}

	return result
}

func (c *HimarketClient) GetDevModelProduct() (map[string]string, error) {
	data, err := c.getProduct(common.MODEL_API)
	if err != nil {
		return nil, fmt.Errorf("failed request himarket: %s", err)
	}
	var response map[string]interface{}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to get model api from response %s", err)
	}

	return c.extractGetProductResponse(common.MODEL_API, response), nil
}

func (c *HimarketClient) GetDevMCPServerProduct() (map[string]string, error) {
	data, err := c.getProduct(common.MCP_SERVER)
	if err != nil {
		return nil, fmt.Errorf("failed request himarket: %s", err)
	}
	var response map[string]interface{}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to get MCP server from response %s", err)
	}

	return c.extractGetProductResponse(common.MCP_SERVER, response), nil
}

func HandleListHimarketMCPServers(client *HimarketClient) ([]byte, error) {
	return nil, nil
}

func HandleAddAPIProduct(client *HimarketClient, body interface{}) ([]byte, error) {
	data, err := client.Post("/api/v1/products", body)
	if err != nil {
		return data, err
	}
	var response map[string]interface{}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to get product id from response: %s", err)
	}

	if res, ok := response["data"].(map[string]interface{}); ok {
		if productId, ok := res["productId"].(string); ok {
			return []byte(productId), nil
		}
	}
	return data, fmt.Errorf("failed to get product id from response")
}

func HandleRefAPIProduct(client *HimarketClient, product_id string, body interface{}) ([]byte, error) {
	return client.Post(fmt.Sprintf("/api/v1/products/%s/ref", product_id), body)
}
