// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package global_least_request

import (
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/resp"
)

type testHttpContext struct {
	wrapper.HttpContext
	values map[string]interface{}
}

func (c *testHttpContext) HasRequestBody() bool {
	return false
}

func (c *testHttpContext) SetContext(key string, value interface{}) {
	c.values[key] = value
}

func (c *testHttpContext) GetContext(key string) interface{} {
	return c.values[key]
}

type testRedisClient struct {
	wrapper.RedisClient
	evalCallback wrapper.RedisResponseCallback
}

func (c *testRedisClient) Eval(_ string, _ int, _, _ []interface{}, callback wrapper.RedisResponseCallback) error {
	c.evalCallback = callback
	return nil
}

type discardLog struct{}

func (discardLog) Trace(string)                  {}
func (discardLog) Tracef(string, ...interface{}) {}
func (discardLog) Debug(string)                  {}
func (discardLog) Debugf(string, ...interface{}) {}
func (discardLog) Info(string)                   {}
func (discardLog) Infof(string, ...interface{})  {}
func (discardLog) Warn(string)                   {}
func (discardLog) Warnf(string, ...interface{})  {}
func (discardLog) Error(string)                  {}
func (discardLog) Errorf(string, ...interface{}) {}
func (discardLog) Critical(string)               {}
func (discardLog) Criticalf(string, ...interface{}) {
}
func (discardLog) ResetID(string) {}

func TestBodylessRequestRunsAsyncSelection(t *testing.T) {
	log.SetPluginLog(discardLog{})

	ctx := &testHttpContext{values: map[string]interface{}{}}
	redisClient := &testRedisClient{}
	var overriddenHost string
	resumed := false
	lb := GlobalLeastRequestLoadBalancer{
		redisClient: redisClient,
		getRouteName: func() (string, error) {
			return "test-route", nil
		},
		getClusterName: func() (string, error) {
			return "test-cluster", nil
		},
		getUpstreamHosts: func() ([][2]string, error) {
			return [][2]string{
				{"10.0.0.1:8080", `{"health_status":"Healthy"}`},
				{"10.0.0.2:8080", `{"health_status":"Healthy"}`},
			}, nil
		},
		setUpstreamOverrideHost: func(host []byte) error {
			overriddenHost = string(host)
			return nil
		},
		resumeHttpRequest: func() error {
			resumed = true
			return nil
		},
	}

	action := lb.HandleHttpRequestHeaders(ctx)

	if action != types.ActionPause {
		t.Fatalf("header action = %v, want %v while Redis selection is pending", action, types.ActionPause)
	}
	if redisClient.evalCallback == nil {
		t.Fatal("bodyless request did not dispatch Redis selection")
	}
	if resumed {
		t.Fatal("request resumed before Redis selection completed")
	}

	redisClient.evalCallback(resp.ArrayValue([]resp.Value{
		resp.StringValue("10.0.0.2:8080"),
		resp.IntegerValue(1),
	}))

	if overriddenHost != "10.0.0.2:8080" {
		t.Fatalf("overridden host = %q, want %q", overriddenHost, "10.0.0.2:8080")
	}
	if !resumed {
		t.Fatal("request was not resumed after Redis selection completed")
	}
	if got := ctx.GetContext("host_selected"); got != "10.0.0.2:8080" {
		t.Fatalf("selected host context = %v, want %q", got, "10.0.0.2:8080")
	}
}
