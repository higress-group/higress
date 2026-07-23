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

// Package main 实现 ai-header-session 插件。
//
// 本插件用于「AI Header 整形」：在请求头处理阶段识别不同的 AI 编程客户端
// （Claude Code、Cursor、Cline、Continue、GitHub Copilot……），从每个客户端
// 各自携带的、零散的会话标识 header 中，按「固定顺序的源 header 列表」确定性地
// 派生出一个统一的会话 header。由于派生过程是输入的纯函数，相同的输入 header
// 永远产生相同的会话 ID（可复现）。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-header-session",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
	)
}

const (
	defaultSessionHeader = "X-AI-Session-Id"
	defaultMatchHeader   = "user-agent"
	hashAlgoFNV          = "fnv"
	hashAlgoSHA256       = "sha256"
	hashHexLen           = 16

	// matchModeClients 按「客户端」识别：先用 match_header 识别客户端，再用该客户端
	// 的 session_headers 列表派生会话 ID（默认方案）。
	matchModeClients = "clients"
	// matchModeHeader 按「header」识别：按 header_rules 列表逐级提取，命中第一个后取
	// 「匹配之后的内容」作为会话来源。
	matchModeHeader = "header"
)

// ClientRule 描述如何识别一个 AI 客户端，以及用哪些 header 派生它的会话 ID。
type ClientRule struct {
	// Name 是客户端标识，同时作为会话 ID 的前缀。
	Name string `json:"name"`
	// MatchHeader 是用于识别客户端的 header 名，其值会与 MatchPattern 做匹配。
	// 为空时默认使用 "user-agent"。
	MatchHeader string `json:"match_header,omitempty"`
	// MatchPattern 是 Go RE2 正则表达式（在 parseConfig 阶段编译）。
	MatchPattern string `json:"match_pattern"`
	// SessionHeaders 是用于派生会话 ID 的 header 名「有序」列表，按此顺序取值并
	// 拼接。顺序是契约的一部分：改变顺序会改变派生出的 ID。
	SessionHeaders []string `json:"session_headers"`

	compiled *regexp.Regexp
}

// HeaderExtractRule 是「header 方案」下的一条逐级提取规则。
//
// 提取语义：取出 Header 的值后，用 Pattern 在值上做正则匹配，命中则取「匹配之后的
// 内容」（即匹配结束位置之后的子串）作为会话来源；Pattern 为空时取整个值。例如
// Header=authorization、Pattern="(?i)^Bearer\\s+"，值 "Bearer sk-abc123" 提取出
// "sk-abc123"。
type HeaderExtractRule struct {
	// Header 是要提取的 header 名。
	Header string `json:"header"`
	// Pattern 是在 header 值上匹配的 Go RE2 正则（parseConfig 阶段编译）；为空表示
	// 直接取整个值。
	Pattern string `json:"pattern,omitempty"`

	compiled *regexp.Regexp
}

// LogConfig 控制插件的诊断日志行为。
type LogConfig struct {
	// DumpUnmatched 控制匹配失败（未识别到客户端、或逐级提取均未命中、或匹配到客户端
	// 但某个源 header 缺失）时，是否打印全部请求头。默认为 true。
	DumpUnmatched bool `json:"dump_unmatched"`
}

// AIHeaderSessionConfig 是插件的顶层配置。
type AIHeaderSessionConfig struct {
	// SessionHeader 是统一输出的 header 名，默认 "X-AI-Session-Id"。
	SessionHeader string `json:"session_header,omitempty"`
	// HashAlgorithm 选择把规范串折叠成会话 ID 的确定性摘要算法：
	// "fnv"（默认）或 "sha256"。
	HashAlgorithm string `json:"hash_algorithm,omitempty"`
	// MatchMode 选择识别方案：matchModeClients（默认）或 matchModeHeader。
	MatchMode string `json:"match_mode,omitempty"`
	// Clients 是「客户端方案」的识别规则集，为空时使用内置默认规则（见 defaultClients）。
	Clients []ClientRule `json:"clients,omitempty"`
	// HeaderRules 是「header 方案」的逐级提取规则列表，仅在 MatchMode 为 header 时生效。
	HeaderRules []HeaderExtractRule `json:"header_rules,omitempty"`
	Log         LogConfig           `json:"log,omitempty"`
}

// defaultClients 返回最常见 AI 编程客户端的内置识别规则。这些规则保守且可被配置
// 完全覆盖。对（敏感的）鉴权类 header 做哈希是安全的：摘要不可逆，得到的是稳定、
// 匿名的「按凭证维度」的会话 key。
func defaultClients() []ClientRule {
	return []ClientRule{
		{
			Name:           "claude-code",
			MatchHeader:    "user-agent",
			MatchPattern:   `(?i)claude`,
			SessionHeaders: []string{"authorization", "x-api-key", "user-agent"},
		},
		{
			Name:           "cursor",
			MatchHeader:    "user-agent",
			MatchPattern:   `(?i)cursor`,
			SessionHeaders: []string{"authorization", "x-cursor-checksum", "user-agent"},
		},
		{
			Name:           "cline",
			MatchHeader:    "user-agent",
			MatchPattern:   `(?i)cline`,
			SessionHeaders: []string{"authorization", "user-agent"},
		},
		{
			Name:           "continue",
			MatchHeader:    "user-agent",
			MatchPattern:   `(?i)continue`,
			SessionHeaders: []string{"authorization", "user-agent"},
		},
		{
			Name:           "github-copilot",
			MatchHeader:    "user-agent",
			MatchPattern:   `(?i)(githubcopilot|copilot|vscode)`,
			SessionHeaders: []string{"authorization", "x-request-id", "user-agent"},
		},
	}
}

func parseConfig(json gjson.Result, config *AIHeaderSessionConfig) error {
	// 统一输出 header 名，缺省回退到默认值。
	config.SessionHeader = strings.TrimSpace(json.Get("session_header").String())
	if config.SessionHeader == "" {
		config.SessionHeader = defaultSessionHeader
	}

	// 摘要算法，仅支持 fnv / sha256。
	config.HashAlgorithm = strings.ToLower(strings.TrimSpace(json.Get("hash_algorithm").String()))
	switch config.HashAlgorithm {
	case "", hashAlgoFNV:
		config.HashAlgorithm = hashAlgoFNV
	case hashAlgoSHA256:
		// 合法
	default:
		return fmt.Errorf("unsupported hash_algorithm %q, expected %q or %q",
			config.HashAlgorithm, hashAlgoFNV, hashAlgoSHA256)
	}

	// 日志配置：dump_unmatched 未显式设置 false 时默认为 true。
	logResult := json.Get("log")
	config.Log.DumpUnmatched = true
	if dump := logResult.Get("dump_unmatched"); dump.Exists() {
		config.Log.DumpUnmatched = dump.Bool()
	}

	// 识别方案开关：clients（默认）或 header。
	config.MatchMode = strings.ToLower(strings.TrimSpace(json.Get("match_mode").String()))
	switch config.MatchMode {
	case "", matchModeClients:
		config.MatchMode = matchModeClients
		return parseClients(json.Get("clients"), config)
	case matchModeHeader:
		return parseHeaderRules(json.Get("header_rules"), config)
	default:
		return fmt.Errorf("unsupported match_mode %q, expected %q or %q",
			config.MatchMode, matchModeClients, matchModeHeader)
	}
}

// parseClients 解析并校验「客户端方案」的规则集；未配置时使用内置默认规则。
func parseClients(clientsResult gjson.Result, config *AIHeaderSessionConfig) error {
	if clientsResult.IsArray() && len(clientsResult.Array()) > 0 {
		if err := json2Clients(clientsResult, config); err != nil {
			return err
		}
	} else {
		config.Clients = defaultClients()
	}
	for i := range config.Clients {
		rule := &config.Clients[i]
		if rule.Name == "" {
			return fmt.Errorf("clients[%d].name must not be empty", i)
		}
		if rule.MatchHeader == "" {
			rule.MatchHeader = defaultMatchHeader
		}
		rule.MatchHeader = strings.ToLower(rule.MatchHeader)
		if rule.MatchPattern == "" {
			return fmt.Errorf("clients[%d].match_pattern must not be empty", i)
		}
		re, err := regexp.Compile(rule.MatchPattern)
		if err != nil {
			return fmt.Errorf("clients[%d].match_pattern is not a valid regex: %w", i, err)
		}
		rule.compiled = re
		if len(rule.SessionHeaders) == 0 {
			return fmt.Errorf("clients[%d].session_headers must not be empty", i)
		}
		for j := range rule.SessionHeaders {
			rule.SessionHeaders[j] = strings.ToLower(strings.TrimSpace(rule.SessionHeaders[j]))
		}
	}
	return nil
}

// parseHeaderRules 解析并校验「header 方案」的逐级提取规则列表。
func parseHeaderRules(rulesResult gjson.Result, config *AIHeaderSessionConfig) error {
	if !rulesResult.IsArray() || len(rulesResult.Array()) == 0 {
		return fmt.Errorf("match_mode is %q but header_rules is empty", matchModeHeader)
	}
	var rules []HeaderExtractRule
	if err := json.Unmarshal([]byte(rulesResult.Raw), &rules); err != nil {
		return fmt.Errorf("failed to parse header_rules: %w", err)
	}
	config.HeaderRules = rules
	for i := range config.HeaderRules {
		rule := &config.HeaderRules[i]
		rule.Header = strings.ToLower(strings.TrimSpace(rule.Header))
		if rule.Header == "" {
			return fmt.Errorf("header_rules[%d].header must not be empty", i)
		}
		if rule.Pattern != "" {
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return fmt.Errorf("header_rules[%d].pattern is not a valid regex: %w", i, err)
			}
			rule.compiled = re
		}
	}
	return nil
}

func json2Clients(clientsResult gjson.Result, config *AIHeaderSessionConfig) error {
	var clients []ClientRule
	if err := json.Unmarshal([]byte(clientsResult.Raw), &clients); err != nil {
		return fmt.Errorf("failed to parse clients: %w", err)
	}
	config.Clients = clients
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config AIHeaderSessionConfig) types.Action {
	ctx.DisableReroute()

	// 步骤 1：幂等跳过——若统一 header 已存在，则什么都不做直接放行。
	if existing, _ := proxywasm.GetHttpRequestHeader(config.SessionHeader); existing != "" {
		log.Debugf("[ai-header-session] %s already present, skip", config.SessionHeader)
		return types.ActionContinue
	}

	headers, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Errorf("[ai-header-session] failed to get request headers: %v", err)
		return types.ActionContinue
	}

	// 步骤 2：按配置的识别方案派生会话 ID。
	if config.MatchMode == matchModeHeader {
		return handleHeaderMode(config, headers)
	}
	return handleClientsMode(config, headers)
}

// handleClientsMode 执行「客户端方案」：先识别客户端，再用其有序源 header 派生 ID。
func handleClientsMode(config AIHeaderSessionConfig, headers [][2]string) types.Action {
	rule := matchClient(config.Clients, headers)
	if rule == nil {
		// 匹配失败（未识别到客户端）：打印全部请求头，然后放行。
		dumpUnmatchedHeaders(config, "未匹配客户端", headers)
		return types.ActionContinue
	}

	// 派生确定性的会话 ID。当客户端某个源 header 缺失时 allPresent 为 false，此时仍然
	// 生成一个（用空串占位的）ID，但同时打印全部请求头以便诊断。
	sessionID, allPresent := deriveSessionID(rule, headers, config.HashAlgorithm)
	if !allPresent {
		dumpUnmatchedHeaders(config, fmt.Sprintf("client %q matched but missing source headers", rule.Name), headers)
	}
	return setSessionHeader(config.SessionHeader, sessionID, "client="+rule.Name)
}

// handleHeaderMode 执行「header 方案」：按 header_rules 逐级提取，命中第一个后取
// 「匹配之后的内容」作为会话来源，再派生确定性的会话 ID。
func handleHeaderMode(config AIHeaderSessionConfig, headers [][2]string) types.Action {
	label, value, ok := extractByHeaderRules(config.HeaderRules, headers)
	if !ok {
		// 匹配失败（逐级提取均未命中）：打印全部请求头，然后放行。
		dumpUnmatchedHeaders(config, "未匹配 header 规则", headers)
		return types.ActionContinue
	}
	sessionID := label + "-" + digest(label+"|"+value, config.HashAlgorithm)
	return setSessionHeader(config.SessionHeader, sessionID, "header="+label)
}

// dumpUnmatchedHeaders 在匹配失败时打印全部请求头（受 dump_unmatched 开关控制，默认开）。
//
// 用 Warn 级输出：wasm-go 的日志会按网关当前日志级别过滤（见 log_wrapper.go，
// level < envoyLogLevel 时直接丢弃），生产网关默认级别通常高于 Info，用 Warn 可保证
// 「匹配失败」这类诊断信息默认可见，无需调低整个网关的日志级别。
func dumpUnmatchedHeaders(config AIHeaderSessionConfig, reason string, headers [][2]string) {
	if !config.Log.DumpUnmatched {
		return
	}
	log.Warnf("[ai-header-session] %s, headers: %s", reason, dumpHeaders(headers))
}

// setSessionHeader 把派生出的会话 ID 写入统一 header，并记录一条调试日志。
func setSessionHeader(name, sessionID, source string) types.Action {
	if err := proxywasm.ReplaceHttpRequestHeader(name, sessionID); err != nil {
		log.Errorf("[ai-header-session] failed to set %s: %v", name, err)
		return types.ActionContinue
	}
	log.Debugf("[ai-header-session] %s %s=%s", source, name, sessionID)
	return types.ActionContinue
}

// extractByHeaderRules 按规则列表「逐级」提取：返回第一个命中规则的标签（header 名）与
// 提取出的会话来源值。命中判定：header 存在且（无 pattern → 取整个值；有 pattern → 在
// 值上正则命中，取「匹配结束位置之后」的子串）。提取值经 trim 后为空则视为未命中，继续
// 向后逐级尝试。
func extractByHeaderRules(rules []HeaderExtractRule, headers [][2]string) (label, value string, ok bool) {
	for i := range rules {
		rule := &rules[i]
		raw := getHeader(headers, rule.Header)
		if raw == "" {
			continue
		}
		if rule.compiled == nil {
			if v := strings.TrimSpace(raw); v != "" {
				return rule.Header, v, true
			}
			continue
		}
		loc := rule.compiled.FindStringIndex(raw)
		if loc == nil {
			continue
		}
		if v := strings.TrimSpace(raw[loc[1]:]); v != "" {
			return rule.Header, v, true
		}
	}
	return "", "", false
}

// matchClient 返回第一个匹配命中的客户端规则。
func matchClient(clients []ClientRule, headers [][2]string) *ClientRule {
	for i := range clients {
		rule := &clients[i]
		if rule.compiled == nil {
			continue
		}
		value := getHeader(headers, rule.MatchHeader)
		if value != "" && rule.compiled.MatchString(value) {
			return rule
		}
	}
	return nil
}

// deriveSessionID 用客户端的有序源 header 构建规范串，再折叠成一个十六进制摘要，
// 输出形如 "<name>-<hash16>"。
//
// 规范串形式：name|h1=v1|h2=v2|...（header 名小写、值 trim、缺失值以空串表示）。
// 这是输入的纯函数，因此相同的 header 永远得到相同的 ID——可复现。
func deriveSessionID(rule *ClientRule, headers [][2]string, algo string) (string, bool) {
	var sb strings.Builder
	sb.WriteString(rule.Name)
	allPresent := true
	for _, h := range rule.SessionHeaders {
		value := strings.TrimSpace(getHeader(headers, h))
		if value == "" {
			allPresent = false
		}
		sb.WriteString("|")
		sb.WriteString(h)
		sb.WriteString("=")
		sb.WriteString(value)
	}
	return rule.Name + "-" + digest(sb.String(), algo), allPresent
}

func digest(canonical, algo string) string {
	switch algo {
	case hashAlgoSHA256:
		sum := sha256.Sum256([]byte(canonical))
		return hex.EncodeToString(sum[:])[:hashHexLen]
	default: // fnv
		h := fnv.New64a()
		_, _ = h.Write([]byte(canonical))
		return fmt.Sprintf("%016x", h.Sum64())
	}
}

// getHeader 返回指定 header 的值（按小写化的键做大小写不敏感匹配）。Envoy 已经会把
// header 名小写化，这里再做一次防御性归一。
func getHeader(headers [][2]string, name string) string {
	name = strings.ToLower(name)
	for _, h := range headers {
		if strings.ToLower(h[0]) == name {
			return h[1]
		}
	}
	return ""
}

// dumpHeaders 把全量 header 渲染成紧凑的 JSON 对象，用于日志输出。
func dumpHeaders(headers [][2]string) string {
	m := make(map[string]string, len(headers))
	for _, h := range headers {
		m[strings.ToLower(h[0])] = h[1]
	}
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Sprintf("%v", m)
	}
	return string(b)
}
