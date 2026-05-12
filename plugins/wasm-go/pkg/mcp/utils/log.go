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

package utils

import (
	"fmt"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"

	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type MCPServerLog struct {
}

func setMCPInfo(msg string) string {
	requestIDRaw, _ := proxywasm.GetProperty([]string{"x_request_id"})
	requestID := string(requestIDRaw)
	if requestID == "" {
		requestID = "nil"
	}
	mcpServerNameRaw, _ := proxywasm.GetProperty([]string{"mcp_server_name"})
	mcpServerName := string(mcpServerNameRaw)
	mcpToolNameRaw, _ := proxywasm.GetProperty([]string{"mcp_tool_name"})
	mcpToolName := string(mcpToolNameRaw)
	mcpInfo := mcpServerName
	if mcpToolName != "" {
		mcpInfo = fmt.Sprintf("%s/%s", mcpServerName, mcpToolName)
	}
	return fmt.Sprintf("[mcp-server] [%s] [%s] %s", mcpInfo, requestID, msg)
}

func (l MCPServerLog) log(level wrapper.LogLevel, msg string) {
	msg = setMCPInfo(msg)
	switch level {
	case wrapper.LogLevelTrace:
		proxywasm.LogTrace(msg)
	case wrapper.LogLevelDebug:
		proxywasm.LogDebug(msg)
	case wrapper.LogLevelInfo:
		proxywasm.LogInfo(msg)
	case wrapper.LogLevelWarn:
		proxywasm.LogWarn(msg)
	case wrapper.LogLevelError:
		proxywasm.LogError(msg)
	case wrapper.LogLevelCritical:
		proxywasm.LogCritical(msg)
	}
}

func (l MCPServerLog) logFormat(level wrapper.LogLevel, format string, args ...interface{}) {
	format = setMCPInfo(format)
	switch level {
	case wrapper.LogLevelTrace:
		proxywasm.LogTracef(format, args...)
	case wrapper.LogLevelDebug:
		proxywasm.LogDebugf(format, args...)
	case wrapper.LogLevelInfo:
		proxywasm.LogInfof(format, args...)
	case wrapper.LogLevelWarn:
		proxywasm.LogWarnf(format, args...)
	case wrapper.LogLevelError:
		proxywasm.LogErrorf(format, args...)
	case wrapper.LogLevelCritical:
		proxywasm.LogCriticalf(format, args...)
	}
}

func (l MCPServerLog) Trace(msg string) {
	l.log(wrapper.LogLevelTrace, msg)
}

func (l MCPServerLog) Tracef(format string, args ...interface{}) {
	l.logFormat(wrapper.LogLevelTrace, format, args...)
}

func (l MCPServerLog) Debug(msg string) {
	l.log(wrapper.LogLevelDebug, msg)
}

func (l MCPServerLog) Debugf(format string, args ...interface{}) {
	l.logFormat(wrapper.LogLevelDebug, format, args...)
}

func (l MCPServerLog) Info(msg string) {
	l.log(wrapper.LogLevelInfo, msg)
}

func (l MCPServerLog) Infof(format string, args ...interface{}) {
	l.logFormat(wrapper.LogLevelInfo, format, args...)
}

func (l MCPServerLog) Warn(msg string) {
	l.log(wrapper.LogLevelWarn, msg)
}

func (l MCPServerLog) Warnf(format string, args ...interface{}) {
	l.logFormat(wrapper.LogLevelWarn, format, args...)
}

func (l MCPServerLog) Error(msg string) {
	l.log(wrapper.LogLevelError, msg)
}

func (l MCPServerLog) Errorf(format string, args ...interface{}) {
	l.logFormat(wrapper.LogLevelError, format, args...)
}

func (l MCPServerLog) Critical(msg string) {
	l.log(wrapper.LogLevelCritical, msg)
}

func (l MCPServerLog) Criticalf(format string, args ...interface{}) {
	l.logFormat(wrapper.LogLevelCritical, format, args...)
}

func (l MCPServerLog) ResetID(pluginID string) {
}
