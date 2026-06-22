// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

// Package main 实现 ai-context-limit Higress WASM 插件。
//
// 插件会在 OpenAI / Anthropic 等协议兼容请求到达上游模型之前估算输入 token 数，
// 并对超过配置阈值的请求提前返回错误响应。
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func main() {}

func init() {
	// 在插件加载时一次性初始化 token 编码器。
	if err := initEncoder(); err != nil {
		// 初始化失败为致命错误：缺少编码器后续无法计算 token，
		// 但 wasm 运行期不能 panic，记录后所有请求走"未启用"兜底路径
		// 实际触发概率极低（embed 词表打包失败才会出现）
		// log 包在 init() 中尚不可用，使用 fmt.Println 兜底
		fmt.Println("[ai-context-limit] init encoder failed:", err)
	}
	wrapper.SetCtx(
		"ai-context-limit",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
	)
}

// onHttpRequestHeaders 处理请求头阶段
//
// 关键约束：
//   - envoy 默认 http filter buffer 仅 14.3KB，必须在此阶段调 SetRequestBodyBufferLimit
//   - 必须返回 HeaderStopIteration，否则 envoy 不会等待 body 阶段
//   - 非 JSON 请求直接放行，不读 body
func onHttpRequestHeaders(ctx wrapper.HttpContext, cfg Config) types.Action {
	ctx.DisableReroute()

	if !cfg.IsEnabled() {
		// 配置缺失时降级为不拦截，允许用户通过配置开关此插件
		log.Warnf("max_context_tokens not configured, plugin disabled for this request")
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
	if !strings.Contains(strings.ToLower(contentType), "application/json") {
		log.Debugf("non-json content-type=%q, skip body inspection", contentType)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	if !ctx.HasRequestBody() {
		log.Debugf("no request body, skip")
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// 强制调大 envoy http downstream decoder buffer
	// 写入 envoy property: set_decoder_buffer_limit
	ctx.SetRequestBodyBufferLimit(MaxRequestBodyBytes)
	// 移除 content-length，body 处理后由 envoy 重新计算
	_ = proxywasm.RemoveHttpRequestHeader("content-length")
	// 暂停 header 流转，等待 onHttpRequestBody 处理完
	return types.HeaderStopIteration
}

// onHttpRequestBody 处理请求体阶段
//
// 流程：
//  1. 抽取请求体中所有需计 token 的文本（兼容 OpenAI / Anthropic 等协议）
//  2. 命中多模态（image_url/audio）→ 直接放行
//  3. token 计数 → ×buffer_ratio → 与阈值比较
//  4. 超阈值 → 发送 local response，OpenAI 风格错误体
//
// 各阶段统一 info 级耗时日志（[aicl]）方便 grep 与基准对照。
func onHttpRequestBody(ctx wrapper.HttpContext, cfg Config, body []byte) types.Action {
	if !cfg.IsEnabled() {
		return types.ActionContinue
	}
	bodyBytes := len(body)
	log.Infof("[aicl] body_received bytes=%d", bodyBytes)

	if encoder == nil {
		log.Errorf("[aicl] token encoder not initialized, skip token counting")
		return types.ActionContinue
	}

	t0 := time.Now()
	result := extractPromptText(body)
	extractMs := time.Since(t0).Milliseconds()
	log.Infof("[aicl] extract_done bytes=%d text_bytes=%d multimodal=%v elapsed_ms=%d",
		bodyBytes, len(result.Text), result.HasMultimodal, extractMs)

	if result.HasMultimodal {
		log.Debugf("[aicl] multimodal request detected, bypass token counting")
		return types.ActionContinue
	}

	t1 := time.Now()
	rawTokens := CountTokens(result.Text)
	encodeMs := time.Since(t1).Milliseconds()
	estimatedTokens := int(float64(rawTokens) * cfg.BufferRatio)
	log.Infof("[aicl] encode_done bytes=%d text_bytes=%d raw_tokens=%d estimated=%d "+
		"threshold=%d extract_ms=%d encode_ms=%d total_ms=%d",
		bodyBytes, len(result.Text), rawTokens, estimatedTokens,
		cfg.MaxContextTokens, extractMs, encodeMs, extractMs+encodeMs)

	if estimatedTokens > cfg.MaxContextTokens {
		return blockOverLimit(cfg, estimatedTokens)
	}
	return types.ActionContinue
}

// blockOverLimit 发送 OpenAI 风格的超限错误响应
//
// 响应体复刻 OpenAI 官方 context_length_exceeded 格式，
// 使客户端 SDK（openai-python / openai-node）可解析为 BadRequestError
func blockOverLimit(cfg Config, estimatedTokens int) types.Action {
	body := fmt.Sprintf(
		`{"error":{"message":"This model's maximum context length is %d tokens. `+
			`Your request had approximately %d tokens.",`+
			`"type":"invalid_request_error","code":"context_length_exceeded"}}`,
		cfg.MaxContextTokens, estimatedTokens,
	)
	headers := [][2]string{{"content-type", "application/json"}}
	if err := proxywasm.SendHttpResponse(uint32(cfg.ErrorStatusCode), headers, []byte(body), -1); err != nil {
		log.Errorf("send local response failed: %v", err)
		return types.ActionContinue
	}
	log.Infof("blocked: estimated %d > limit %d", estimatedTokens, cfg.MaxContextTokens)
	return types.ActionContinue
}
