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

package main

import (
	ejson "encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"api-workflow/utils"
	. "api-workflow/workflow"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	DefaultMaxDepth    uint32 = 100
	WorkflowExecStatus string = "workflowExecStatus"
	DefaultTimeout     uint32 = 5000
)

func main() {}

func init() {
	wrapper.SetCtx(
		"api-workflow",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
	)
}

func parseConfig(json gjson.Result, c *PluginConfig, log log.Log) error {

	edges := make([]Edge, 0)
	nodes := make(map[string]Node)
	var err error
	// env
	env := json.Get("env")
	// timeout
	c.Env.Timeout = uint32(env.Get("timeout").Int())
	if c.Env.Timeout == 0 {
		c.Env.Timeout = DefaultTimeout
	}
	// max_depth
	c.Env.MaxDepth = uint32(env.Get("max_depth").Int())
	if c.Env.MaxDepth == 0 {
		c.Env.MaxDepth = DefaultMaxDepth
	}
	// workflow
	workflow := json.Get("workflow")
	if !workflow.Exists() {
		return errors.New("workflow is empty")
	}
	// workflow.edges
	edges_ := workflow.Get("edges")
	if edges_.Exists() && edges_.IsArray() {
		for _, w := range edges_.Array() {
			task := Task{}
			edge := Edge{}
			edge.Source = w.Get("source").String()
			if edge.Source == "" {
				return errors.New("source is empty")
			}
			edge.Target = w.Get("target").String()
			if edge.Target == "" {
				return errors.New("target is empty")
			}
			edge.Task = &task

			edge.Conditional = w.Get("conditional").String()
			edges = append(edges, edge)
		}
	}
	c.Workflow.Edges = edges
	// workflow.nodes
	nodes_ := workflow.Get("nodes")
	if nodes_.Exists() && nodes_.IsArray() {
		for _, value := range nodes_.Array() {
			node := Node{}
			node.Name = value.Get("name").String()
			if node.Name == "" {
				return errors.New("tool name is empty")
			}
			node.ServiceName = value.Get("service_name").String()
			if node.ServiceName == "" {
				return errors.New("tool service name is empty")
			}
			node.ServicePort = value.Get("service_port").Int()
			if node.ServicePort == 0 {
				if strings.HasSuffix(node.ServiceName, ".static") {
					// use default logic port which is 80 for static service
					node.ServicePort = 80
				} else {
					return errors.New("tool service port is empty")
				}

			}
			node.ServiceDomain = value.Get("service_domain").String()
			node.ServicePath = value.Get("service_path").String()
			if node.ServicePath == "" {
				node.ServicePath = "/"
			}
			node.ServiceMethod = value.Get("service_method").String()
			if node.ServiceMethod == "" {
				return errors.New("service_method is empty")
			}
			serviceHeaders := value.Get("service_headers")
			if serviceHeaders.Exists() && serviceHeaders.IsArray() {
				serviceHeaders_ := []ServiceHeader{}
				err = ejson.Unmarshal([]byte(serviceHeaders.Raw), &serviceHeaders_)
				node.ServiceHeaders = serviceHeaders_
			}

			node.ServiceBodyTmpl = value.Get("service_body_tmpl").String()
			serviceBodyReplaceKeys := value.Get("service_body_replace_keys")
			if serviceBodyReplaceKeys.Exists() && serviceBodyReplaceKeys.IsArray() {
				serviceBodyReplaceKeys_ := []BodyReplaceKeyPair{}
				err = ejson.Unmarshal([]byte(serviceBodyReplaceKeys.Raw), &serviceBodyReplaceKeys_)
				node.ServiceBodyReplaceKeys = serviceBodyReplaceKeys_
				if err != nil {
					return fmt.Errorf("unmarshal service body replace keys failed, err:%v", err)
				}
			}

			nodes[node.Name] = node
		}
		c.Workflow.Nodes = nodes
		// workflow.WorkflowExecStatus
		c.Workflow.WorkflowExecStatus, err = initWorkflowExecStatus(c)
		log.Debugf("init status : %v", c.Workflow.WorkflowExecStatus)
		if err != nil {
			log.Errorf("init workflow exec status failed, err:%v", err)
			return fmt.Errorf("init workflow exec status failed, err:%v", err)
		}
	}
	log.Debugf("config : %v", c)
	return nil
}

func initWorkflowExecStatus(config *PluginConfig) (map[string]int, error) {
	result := make(map[string]int)

	for name, _ := range config.Workflow.Nodes {
		result[name] = 0
	}
	for _, edge := range config.Workflow.Edges {

		if edge.Source == TaskStart || edge.Target == TaskContinue || edge.Target == TaskEnd {
			continue
		}

		count, ok := result[edge.Target]
		if !ok {
			return nil, fmt.Errorf("Target %s is not exist in nodes", edge.Target)
		}
		result[edge.Target] = count + 1

	}
	return result, nil
}

func onHttpRequestBody(ctx wrapper.HttpContext, config PluginConfig, body []byte, log log.Log) types.Action {

	initHeader := make([][2]string, 0)
	// 初始化运行状态
	ctx.SetContext(WorkflowExecStatus, config.Workflow.WorkflowExecStatus)

	// 执行工作流
	for _, edge := range config.Workflow.Edges {

		if edge.Source == TaskStart {
			ctx.SetContext(fmt.Sprintf("%s", TaskStart), body)
			err := recursive(edge, initHeader, body, 1, config, log, ctx)
			if err != nil {
				// 工作流处理错误，返回500给用户
				log.Errorf("recursive failed: %v", err)
				_ = utils.SendResponse(500, "api-workflow.recursive_failed", utils.MimeTypeTextPlain, fmt.Sprintf("workflow plugin recursive failed: %v", err))

			}
		}
	}

	return types.ActionPause
}

// 放入符合条件的edge
func recursive(edge Edge, headers [][2]string, body []byte, depth uint32, config PluginConfig, log log.Log, ctx wrapper.HttpContext) error {

	var err error
	// 防止递归次数太多
	if depth > config.Env.MaxDepth {
		return fmt.Errorf("maximum recursion depth reached")
	}

	// 判断是不是end
	if edge.IsEnd() {
		log.Debugf("source is %s,target is %s,workflow is end", edge.Source, edge.Target)
		log.Debugf("body is %s", string(body))
		_ = proxywasm.SendHttpResponse(200, headers, body, -1)
		return nil
	}
	// 判断是不是continue
	if edge.IsContinue() {
		log.Debugf("source is %s,target is %s,workflow is continue", edge.Source, edge.Target)
		_ = proxywasm.ResumeHttpRequest()
		return nil
	}

	// 封装task
	err = edge.WrapperTask(config, ctx)
	if err != nil {
		log.Errorf("workflow exec wrapperTask find error,source is %s,target is %s,error is %v ", edge.Source, edge.Target, err)
		return fmt.Errorf("workflow exec wrapperTask find error,source is %s,target is %s,error is %v ", edge.Source, edge.Target, err)
	}

	// 执行task
	log.Debugf("workflow exec task,source is %s,target is %s, body is %s,header is %v", edge.Source, edge.Target, string(edge.Task.Body), edge.Task.Headers)
	err = wrapper.HttpCall(edge.Task.Cluster, edge.Task.Method, edge.Task.ServicePath, edge.Task.Headers, edge.Task.Body, func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("code:%d", statusCode)
		// 判断response code
		if statusCode < 400 {

			// 存入这轮返回的body
			ctx.SetContext(fmt.Sprintf("%s", edge.Target), responseBody)

			headers_ := make([][2]string, 0, len(responseHeaders))
			for key, value := range responseHeaders {
				headers_ = append(headers_, [2]string{key, value[0]})
			}
			// 判断是否进入下一步
			nextStatus := ctx.GetContext(WorkflowExecStatus).(map[string]int)

			// 进入下一步
			for _, next := range config.Workflow.Edges {
				if next.Source == edge.Target {
					// 更新workflow status
					if next.Target != TaskContinue && next.Target != TaskEnd {

						nextStatus[next.Target] = nextStatus[next.Target] - 1
						log.Debugf("source is %s,target is %s,status is %v", next.Source, next.Target, nextStatus)
						// 还有没执行完的边
						if nextStatus[next.Target] > 0 {
							ctx.SetContext(WorkflowExecStatus, nextStatus)
							return
						}
						// 执行出了问题
						if nextStatus[next.Target] < 0 {
							log.Errorf("workflow exec status find  error  %v", nextStatus)
							_ = utils.SendResponse(500, "api-workflow.exec_task_failed", utils.MimeTypeTextPlain, fmt.Sprintf("workflow exec status find  error  %v", nextStatus))
							return
						}
					}
					// 判断是否执行
					isPass, err2 := next.IsPass(ctx)
					if err2 != nil {
						log.Errorf("check pass find error:%v", err2)
						_ = utils.SendResponse(500, "api-workflow.task_check_paas_failed", utils.MimeTypeTextPlain, fmt.Sprintf("check pass find error:%v", err2))
						return
					}
					if isPass {
						log.Debugf("source is %s,target is %s,workflow is pass ", next.Source, next.Target)
						nextStatus = ctx.GetContext(WorkflowExecStatus).(map[string]int)
						nextStatus[next.Target] = nextStatus[next.Target] - 1
						ctx.SetContext(WorkflowExecStatus, nextStatus)
						continue

					}

					// 执行下一步
					err = recursive(next, headers_, responseBody, depth+1, config, log, ctx)
					if err != nil {
						log.Errorf("recursive error:%v", err)
						_ = utils.SendResponse(500, "api-workflow.recursive_failed", utils.MimeTypeTextPlain, fmt.Sprintf("recursive error:%v", err))
						return
					}
				}
			}

		} else {
			// statusCode >= 400 ,task httpCall执行失败，放行请求，打印错误，结束workflow
			log.Errorf("workflow exec task find error,code is %d,body is %s", statusCode, string(responseBody))
			_ = utils.SendResponse(500, "api-workflow.httpCall_failed", utils.MimeTypeTextPlain, fmt.Sprintf("workflow exec task find error,code is %d,body is %s", statusCode, string(responseBody)))
		}
		return

	}, config.Env.MaxDepth*config.Env.Timeout)
	if err != nil {
		log.Errorf("httpcall error:%v", err)
	}

	return err
}
