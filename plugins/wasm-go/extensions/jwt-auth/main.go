// Copyright (c) 2023 Alibaba Group Holding Ltd.
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

package main

import (
	"github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/handler"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

// @Name jwt-proxy
// @Category auth
// @Phase UNSPECIFIED_PHASE
// @Priority 0
// @Title zh-CN jwt验证
// @Description zh-CN 通过jwt进行验证
// @Version 0.1.0
//
// @Contact.name Ink33
// @Contact.url https://github.com/Ink-33
// @Contact.email ink33@smlk.org
//
// @Example
//
//	{
//	  "consumers": [
//	    {
//	      "name": "example-consumer",
//	      "issuer": "https://issuer.example.com",
//	      "remote_jwks": {
//	        "service_name": "issuer.example.com.dns",
//	        "service_host": "issuer.example.com",
//	        "service_port": 443,
//	        "path": "/.well-known/jwks.json"
//	      },
//	      "jwks_cache_duration": 600,
//	      "jwks_fetch_timeout": 1500
//	    },
//	    {
//	      "name": "inline-consumer",
//	      "issuer": "https://issuer.example.com",
//	      "jwks": "{\"keys\":[...]}"
//	    }
//	  ]
//	}
//
// @End
func main() {}

func init() {
	wrapper.SetCtx(
		// 插件名称
		"jwt-auth",
		// 为解析插件配置，设置自定义函数
		wrapper.ParseConfigBy(parseGlobalConfig),
		wrapper.ParseOverrideConfigBy(parseGlobalConfig, config.ParseRuleConfig),
		// 为处理请求头，设置自定义函数
		wrapper.ProcessRequestHeadersBy(handler.OnHTTPRequestHeaders),
	)
}

func parseGlobalConfig(json gjson.Result, cfg *config.JWTAuthConfig, logger log.Log) error {
	if err := config.ParseGlobalConfig(json, cfg, logger); err != nil {
		return err
	}
	handler.PruneRemoteJWKsCache(cfg.Consumers)
	return nil
}
