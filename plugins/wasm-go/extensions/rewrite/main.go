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
	"strings"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/rewrite/rewriter"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
)

func main() {
	wrapper.SetCtx(
		"rewrite",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

type RewriteConfig struct {
	rewriteRules []RewriteRule
}

type RewriteRule struct {
	matchPathType string // prefix | exact | regex
	caseSensitive bool
	matchHosts    []string
	matchPaths    []string
	rewriteHost   string
	rewritePath   string
}

var rewriters []*rewriter.Rewriter

func parseConfig(json gjson.Result, config *RewriteConfig, log wrapper.Log) error {
	log.Debug("[parseConfig]")

	var path, host string
	rules := json.Get("rewrite_rules").Array()
	config.rewriteRules = make([]RewriteRule, 0, len(rules))
	rewriters = make([]*rewriter.Rewriter, 0, len(rules))
	for _, rule := range rules {
		var rr RewriteRule
		rr.matchPathType = rule.Get("match_path_type").String()
		rr.caseSensitive = rule.Get("case_sensitive").Bool()
		rr.rewriteHost = rule.Get("rewrite_host").String()
		rr.rewritePath = rule.Get("rewrite_path").String()
		for _, item := range rule.Get("match_hosts").Array() {
			if host = item.String(); host == "" {
				continue
			}
			if rr.caseSensitive {
				rr.matchHosts = append(rr.matchHosts, host)
			} else {
				rr.matchHosts = append(rr.matchHosts, strings.ToLower(host))
			}
		}
		for _, item := range rule.Get("match_paths").Array() {
			if path = item.String(); path == "" {
				continue
			}
			if rr.caseSensitive {
				rr.matchPaths = append(rr.matchPaths, path)
			} else {
				rr.matchPaths = append(rr.matchPaths, strings.ToLower(path))
			}
		}
		config.rewriteRules = append(config.rewriteRules, rr)

		// config -> rewriters
		// rules[i] <-> rewriters[i]
		rw := rewriter.NewRewriter(len(rr.matchHosts), len(rr.matchPaths), rr.rewriteHost, rr.rewritePath)
		for _, matchHost := range rr.matchHosts {
			matchHostType := rewriter.HostUnknown
			if strings.HasSuffix(matchHost, "*") {
				matchHostType = rewriter.HostPrefix
				matchHost = matchHost[:len(matchHost)-1]
			} else if strings.HasPrefix(matchHost, "*") {
				matchHostType = rewriter.HostSuffix
				matchHost = matchHost[1:]
			} else {
				matchHostType = rewriter.HostExact
			}
			rw.AppendHostMatcher(matchHostType, matchHost)
		}

		matchPathType := rewriter.PathUnknown
		switch rr.matchPathType {
		case "prefix":
			matchPathType = rewriter.PathPrefix
		case "exact":
			matchPathType = rewriter.PathExact
		case "regex":
			matchPathType = rewriter.PathRegex
		}
		for _, matchPath := range rr.matchPaths {
			rw.AppendPathMatcher(matchPathType, matchPath, matchPath)
		}

		rewriters = append(rewriters, rw)
	}

	log.Debugf("rewrite config: %+v", *config)

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config RewriteConfig, log wrapper.Log) types.Action {
	log.Debug("[onHttpRequestHeaders]")

	var (
		host, path               = ctx.Host(), ctx.Path() // keep the original
		reqHost, reqPath         string
		matched                  bool
		rewriteHost, rewritePath string
	)
	log.Debugf("request: host=%s, path=%s", host, path)

	for i, rule := range config.rewriteRules {
		reqHost, reqPath = host, path
		if !rule.caseSensitive {
			reqHost = strings.ToLower(host)
			reqPath = strings.ToLower(path)
		}
		matched, rewriteHost, rewritePath = rewriters[i].MatchAndRewrite(reqHost, reqPath)
		if matched {
			log.Debugf("match rule#%d, type: %s, rewrite: host[%s -> %s] path[%s -> %s]", i, rule.matchPathType, reqHost, rewriteHost, reqPath, rewritePath)
			break
		}
	}
	if !matched {
		log.Debug("unmatched")
		return types.ActionContinue
	}

	hs, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Warnf("get request headers failed: %v", err)
	}
	for i, h := range hs {
		if h[0] == ":authority" {
			hs[i][1] = rewriteHost
		} else if h[0] == ":path" {
			hs[i][1] = rewritePath
		}
	}
	err = proxywasm.ReplaceHttpRequestHeaders(hs)
	if err != nil {
		log.Warnf("replace request headers failed: %v", err)
		return types.ActionContinue
	}
	log.Debug("rewrite successful")

	return types.ActionContinue
}
