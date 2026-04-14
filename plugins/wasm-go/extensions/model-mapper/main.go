package main

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	DefaultMaxBodyBytes = 100 * 1024 * 1024 // 100MB
)

func main() {}

func init() {
	wrapper.SetCtx(
		"model-mapper",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.WithRebuildAfterRequests[Config](1000),
		wrapper.WithRebuildMaxMemBytes[Config](200*1024*1024),
	)
}

type ModelMapping struct {
	Prefix string
	Target string
}

type Config struct {
	modelKey           string
	exactModelMapping  map[string]string
	prefixModelMapping []ModelMapping
	defaultModel       string
	enableOnPathSuffix []string
}

func parseConfig(json gjson.Result, config *Config) error {
	config.modelKey = json.Get("modelKey").String()
	if config.modelKey == "" {
		config.modelKey = "model"
	}

	modelMapping := json.Get("modelMapping")
	if modelMapping.Exists() && !modelMapping.IsObject() {
		return errors.New("modelMapping must be an object")
	}

	config.exactModelMapping = make(map[string]string)
	config.prefixModelMapping = make([]ModelMapping, 0)

	// To replicate C++ behavior (nlohmann::json iterates keys alphabetically),
	// we collect entries and sort them by key.
	type mappingEntry struct {
		key   string
		value string
	}
	var entries []mappingEntry
	modelMapping.ForEach(func(key, value gjson.Result) bool {
		entries = append(entries, mappingEntry{
			key:   key.String(),
			value: value.String(),
		})
		return true
	})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})

	for _, entry := range entries {
		key := entry.key
		value := entry.value
		if key == "*" {
			config.defaultModel = value
		} else if strings.HasSuffix(key, "*") {
			prefix := strings.TrimSuffix(key, "*")
			config.prefixModelMapping = append(config.prefixModelMapping, ModelMapping{
				Prefix: prefix,
				Target: value,
			})
		} else {
			config.exactModelMapping[key] = value
		}
	}

	enableOnPathSuffix := json.Get("enableOnPathSuffix")
	if enableOnPathSuffix.Exists() {
		if !enableOnPathSuffix.IsArray() {
			return errors.New("enableOnPathSuffix must be an array")
		}
		for _, item := range enableOnPathSuffix.Array() {
			config.enableOnPathSuffix = append(config.enableOnPathSuffix, item.String())
		}
	} else {
		config.enableOnPathSuffix = []string{
			"/completions",
			"/embeddings",
			"/images/generations",
			"/audio/speech",
			"/fine_tuning/jobs",
			"/moderations",
			"/image-synthesis",
			"/video-synthesis",
			"/rerank",
			"/messages",
			"/responses",
		}
	}

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	// Check path suffix
	path, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil {
		return types.ActionContinue
	}

	// Strip query parameters
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	matched := false
	for _, suffix := range config.enableOnPathSuffix {
		if strings.HasSuffix(path, suffix) {
			matched = true
			break
		}
	}

	if !matched || !ctx.HasRequestBody() {
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// Prepare for body processing
	proxywasm.RemoveHttpRequestHeader("content-length")
	// 100MB buffer limit
	ctx.SetRequestBodyBufferLimit(DefaultMaxBodyBytes)

	return types.HeaderStopIteration
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	if len(body) == 0 {
		return types.ActionContinue
	}

	if !json.Valid(body) {
		log.Error("invalid json body")
		return types.ActionContinue
	}

	oldModel := gjson.GetBytes(body, config.modelKey).String()

	newModel := config.defaultModel
	if newModel == "" {
		newModel = oldModel
	}

	// Exact match
	if target, ok := config.exactModelMapping[oldModel]; ok {
		newModel = target
	} else {
		// Prefix match
		for _, mapping := range config.prefixModelMapping {
			if strings.HasPrefix(oldModel, mapping.Prefix) {
				newModel = mapping.Target
				break
			}
		}
	}

	if newModel != "" && newModel != oldModel {
		newBody, err := sjson.SetBytes(body, config.modelKey, newModel)
		if err != nil {
			log.Errorf("failed to update model: %v", err)
			return types.ActionContinue
		}
		proxywasm.ReplaceHttpRequestBody(newBody)
		log.Debugf("model mapped, before: %s, after: %s", oldModel, newModel)
	}

	return types.ActionContinue
}
