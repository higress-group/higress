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
	"fmt"

	"github.com/tidwall/gjson"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/traffic-editor/pkg"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	ctxKeyEditorContext = "editorContext"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"traffic-editor",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
	)
}

func parseConfig(json gjson.Result, config *PluginConfig) (err error) {
	if err := config.FromJson(json); err != nil {
		return fmt.Errorf("failed to parse plugin config: %v", err)
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config PluginConfig) types.Action {
	log.Debugf("onHttpRequestHeaders called with config")

	editorContext := pkg.NewEditorContext()
	if headers, err := proxywasm.GetHttpRequestHeaders(); err == nil {
		editorContext.SetRequestHeaders(headerSlice2Map(headers))
	} else {
		log.Errorf("failed to get request headers: %v", err)
	}
	saveEditorContext(ctx, editorContext)

	effectiveCommandSet := findEffectiveCommandSet(editorContext, &config)
	if effectiveCommandSet == nil {
		log.Debugf("no effective command set found for request %s", ctx.Path())
		return types.ActionContinue
	}
	if len(effectiveCommandSet.Commands) == 0 {
		log.Debugf("the effective command set found for request %s is empty", ctx.Path())
		return types.ActionContinue
	}

	log.Debugf("an effective command set found for request %s with %d commands", ctx.Path(), len(effectiveCommandSet.Commands))
	editorContext.SetEffectiveCommandSet(effectiveCommandSet)
	editorContext.SetCommandExecutors(effectiveCommandSet.CreatExecutors())

	// Make sure the editor context is clean before executing any command.
	editorContext.ResetDirtyFlags()

	if effectiveCommandSet.DisableReroute {
		ctx.DisableReroute()
	}

	executeCommands(editorContext, pkg.StageRequestHeaders)

	if err := saveRequestHeaderChanges(editorContext); err != nil {
		log.Errorf("failed to save request header changes: %v", err)
	}

	// Make sure the editor context is clean before continue.
	editorContext.ResetDirtyFlags()

	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config PluginConfig) types.Action {
	log.Debugf("onHttpResponseHeaders called with config")

	editorContext := loadEditorContext(ctx)
	if editorContext == nil || editorContext.GetEffectiveCommandSet() == nil {
		log.Debugf("no effective command set found for request %s", ctx.Path())
		return types.ActionContinue
	}

	if headers, err := proxywasm.GetHttpResponseHeaders(); err == nil {
		editorContext.SetResponseHeaders(headerSlice2Map(headers))
	} else {
		log.Errorf("failed to get response headers: %v", err)
	}

	// Make sure the editor context is clean before executing any command.
	editorContext.ResetDirtyFlags()

	executeCommands(editorContext, pkg.StageResponseHeaders)
	if err := saveResponseHeaderChanges(editorContext); err != nil {
		log.Errorf("failed to save response header changes: %v", err)
	}

	// Make sure the editor context is clean before continue.
	editorContext.ResetDirtyFlags()

	return types.ActionContinue
}

func findEffectiveCommandSet(editorContext pkg.EditorContext, config *PluginConfig) *pkg.CommandSet {
	if config == nil {
		return nil
	}
	if len(config.ConditionalConfigs) != 0 {
		for i, conditionalConfig := range config.ConditionalConfigs {
			log.Debugf("Evaluating conditional config %d: %+v", i, conditionalConfig)
			if conditionalConfig.Matches(editorContext) {
				log.Debugf("Use the conditional command set %d", i)
				return &conditionalConfig.CommandSet
			}
		}
	}
	log.Debugf("Use the default command set")
	return config.DefaultConfig
}

func executeCommands(editorContext pkg.EditorContext, stage pkg.Stage) {
	for _, executor := range editorContext.GetCommandExecutors() {
		if err := executor.Run(editorContext, stage); err != nil {
			log.Errorf("failed to execute a %s command in stage %s: %v", executor.GetCommand().GetType(), pkg.Stage2String[stage], err)
		}
	}
}

func saveRequestHeaderChanges(editorContext pkg.EditorContext) error {
	if !editorContext.IsRequestHeadersDirty() {
		log.Debugf("no request header change to save")
		return nil
	}

	log.Debugf("saving request header changes: %v", editorContext.GetRequestHeaders())
	headerSlice := headerMap2Slice(editorContext.GetRequestHeaders())
	return proxywasm.ReplaceHttpRequestHeaders(headerSlice)
}

func saveResponseHeaderChanges(editorContext pkg.EditorContext) error {
	if !editorContext.IsResponseHeadersDirty() {
		log.Debugf("no response header change to save")
		return nil
	}
	log.Debugf("saving response header changes: %v", editorContext.GetResponseHeaders())
	headerSlice := headerMap2Slice(editorContext.GetResponseHeaders())
	return proxywasm.ReplaceHttpResponseHeaders(headerSlice)
}

func loadEditorContext(ctx wrapper.HttpContext) pkg.EditorContext {
	editorContext, _ := ctx.GetContext(ctxKeyEditorContext).(pkg.EditorContext)
	return editorContext
}

func saveEditorContext(ctx wrapper.HttpContext, editorContext pkg.EditorContext) {
	ctx.SetContext(ctxKeyEditorContext, editorContext)
}
