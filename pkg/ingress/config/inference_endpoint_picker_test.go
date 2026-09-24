// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package config

import (
	"testing"

	extensions "istio.io/api/extensions/v1alpha1"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/gateway/kube"
	"istio.io/istio/pkg/config/schema/gvk"

	higressconfig "github.com/alibaba/higress/v2/pkg/config"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"
)

func TestConvertBuiltinInferenceEndpointPicker(t *testing.T) {
	ingressConfig := &IngressConfig{
		namespace: "higress-system",
		commonOptions: common.Options{
			GatewaySelectorKey:   "app",
			GatewaySelectorValue: "higress-gateway",
		},
	}
	virtualService := config.Config{
		Meta: config.Meta{GroupVersionKind: gvk.VirtualService, Name: "mixed"},
		Extra: map[string]any{constants.ConfigExtraPerRouteRuleInferencePoolConfigs: map[string]kube.InferencePoolRouteRuleConfig{
			"builtin-route":   {Mode: kube.InferencePoolEndpointPickerModeBuiltin},
			"default/foo/0/0": {Mode: kube.InferencePoolEndpointPickerModeBuiltin},
			"default/foo.0.0": {Mode: kube.InferencePoolEndpointPickerModeExternal, FQDN: "epp.default.svc.cluster.local", Port: "9002"},
			"external-route":  {Mode: kube.InferencePoolEndpointPickerModeExternal, FQDN: "epp.default.svc.cluster.local", Port: "9002"},
		}},
	}

	got := ingressConfig.convertBuiltinInferenceEndpointPicker([]config.Config{virtualService})
	if len(got) != 1 {
		t.Fatalf("expected one internal WasmPlugin, got %d", len(got))
	}
	if got[0].Name != "higress-internal-ai-endpoint-picker" || got[0].Namespace != ingressConfig.namespace {
		t.Fatalf("unexpected internal plugin identity: %s/%s", got[0].Namespace, got[0].Name)
	}
	plugin := got[0].Spec.(*extensions.WasmPlugin)
	if plugin.Selector.MatchLabels["app"] != "higress-gateway" {
		t.Fatalf("expected plugin to select the configured gateway, got %v", plugin.Selector.MatchLabels)
	}
	if plugin.PluginName != "ai-endpoint-picker" || plugin.FailStrategy != extensions.FailStrategy_FAIL_OPEN {
		t.Fatalf("unexpected internal plugin contract: %+v", plugin)
	}
	if plugin.Url != higressconfig.AIEndpointPickerPluginURL {
		t.Fatalf("unexpected plugin URL %q", plugin.Url)
	}
	pluginFields := plugin.PluginConfig.Fields
	if len(pluginFields) != 1 {
		t.Fatalf("expected PluginConfig to contain only _rules_, got %v", pluginFields)
	}
	rulesValue, found := pluginFields["_rules_"]
	if !found {
		t.Fatalf("expected PluginConfig _rules_, got %v", pluginFields)
	}
	rules := rulesValue.GetListValue().Values
	if len(rules) != 2 {
		t.Fatalf("expected one isolated rule per BuiltIn route, got %d", len(rules))
	}
	for i, routeName := range []string{"builtin-route", "default/foo/0/0"} {
		ruleFields := rules[i].GetStructValue().Fields
		if len(ruleFields) != 1 {
			t.Fatalf("expected an empty BuiltIn rule config apart from route matching, got %v", ruleFields)
		}
		matches := ruleFields["_match_route_"].GetListValue().Values
		if len(matches) != 1 || matches[0].GetStringValue() != routeName {
			t.Fatalf("expected rule %d to bind only route %q, got %v", i, routeName, matches)
		}
	}

	virtualService.Extra = map[string]any{constants.ConfigExtraPerRouteRuleInferencePoolConfigs: map[string]kube.InferencePoolRouteRuleConfig{
		"external-route": {Mode: kube.InferencePoolEndpointPickerModeExternal, FQDN: "epp.default.svc.cluster.local", Port: "9002"},
	}}
	if got := ingressConfig.convertBuiltinInferenceEndpointPicker([]config.Config{virtualService}); len(got) != 0 {
		t.Fatalf("expected ExternalEPP routes not to bind the internal plugin, got %d configs", len(got))
	}
}
