// Copyright Istio Authors
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

package istio

import (
	"strings"
	"testing"
	"time"

	istio "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/gateway/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
)

func TestMergeHTTPRoutesMergesInferencePoolExtra(t *testing.T) {
	stop := test.NewStop(t)
	routeKey := "default/gateway/example.com"
	baseRouteName := "default/local-ai-chat"
	otherRouteName := "default/local-ai-chat-360m"
	baseInferenceConfigs := map[string]kube.InferencePoolRouteRuleConfig{
		baseRouteName: {
			FQDN:             "local-ai-chat-pool-epp.default.svc.cluster.local",
			Port:             "9002",
			FailureModeAllow: true,
		},
	}
	otherInferenceConfigs := map[string]kube.InferencePoolRouteRuleConfig{
		otherRouteName: {
			FQDN: "local-ai-chat-360m-pool-epp.default.svc.cluster.local",
			Port: "9002",
		},
	}
	baseCfg := &config.Config{
		Meta: config.Meta{
			Name:              "local-ai-chat",
			Namespace:         "default",
			CreationTimestamp: time.Unix(1, 0),
			Annotations: map[string]string{
				constants.InternalParentNames: "parent-a",
			},
		},
		Spec: &istio.VirtualService{
			Hosts:    []string{"example.com"},
			Gateways: []string{"default/gateway"},
			Http: []*istio.HTTPRoute{{
				Name: baseRouteName,
			}},
		},
		Extra: map[string]any{
			constants.ConfigExtraPerRouteRuleInferencePoolConfigs: baseInferenceConfigs,
			"non-inference-extra": "kept-from-base",
		},
	}
	otherCfg := &config.Config{
		Meta: config.Meta{
			Name:              "local-ai-chat-360m",
			Namespace:         "default",
			CreationTimestamp: time.Unix(2, 0),
			Annotations: map[string]string{
				constants.InternalParentNames: "parent-b",
			},
		},
		Spec: &istio.VirtualService{
			Hosts:    []string{"example.com"},
			Gateways: []string{"default/gateway"},
			Http: []*istio.HTTPRoute{{
				Name: otherRouteName,
			}},
		},
		Extra: map[string]any{
			constants.ConfigExtraPerRouteRuleInferencePoolConfigs: otherInferenceConfigs,
			"non-inference-extra": "ignored-from-later-route",
			"other-extra":         "added-from-later-route",
		},
	}
	baseVirtualServices := krt.NewStaticCollection[RouteWithKey](nil, []RouteWithKey{
		{
			Config: baseCfg,
			Key:    routeKey,
		},
		{
			Config: otherCfg,
			Key:    routeKey,
		},
	}, krt.WithStop(stop), krt.WithName("base"))

	merged := mergeHTTPRoutes(baseVirtualServices, krt.WithStop(stop), krt.WithName("merged"))
	merged.WaitUntilSynced(stop)
	gotList := merged.List()
	if len(gotList) != 1 {
		t.Fatalf("expected one merged VirtualService, got %d", len(gotList))
	}

	got := gotList[0]
	if got.Name != strings.ReplaceAll(routeKey, "/", "~") {
		t.Fatalf("expected merged VirtualService name %q, got %q", strings.ReplaceAll(routeKey, "/", "~"), got.Name)
	}
	gotVS := got.Spec.(*istio.VirtualService)
	if len(gotVS.Http) != 2 {
		t.Fatalf("expected merged VirtualService to contain 2 HTTP routes, got %d", len(gotVS.Http))
	}

	gotInferenceConfigs, ok := got.Extra[constants.ConfigExtraPerRouteRuleInferencePoolConfigs].(map[string]kube.InferencePoolRouteRuleConfig)
	if !ok {
		t.Fatalf("expected merged InferencePool configs, got %T", got.Extra[constants.ConfigExtraPerRouteRuleInferencePoolConfigs])
	}
	if len(gotInferenceConfigs) != 2 {
		t.Fatalf("expected 2 merged InferencePool configs, got %d: %v", len(gotInferenceConfigs), gotInferenceConfigs)
	}
	if gotInferenceConfigs[baseRouteName].FQDN != baseInferenceConfigs[baseRouteName].FQDN {
		t.Fatalf("expected base route InferencePool config to be preserved, got %v", gotInferenceConfigs[baseRouteName])
	}
	if gotInferenceConfigs[otherRouteName].FQDN != otherInferenceConfigs[otherRouteName].FQDN {
		t.Fatalf("expected later route InferencePool config to be merged, got %v", gotInferenceConfigs[otherRouteName])
	}
	if got.Extra["non-inference-extra"] != "kept-from-base" {
		t.Fatalf("expected non-InferencePool Extra to keep base value, got %v", got.Extra["non-inference-extra"])
	}
	if got.Extra["other-extra"] != "added-from-later-route" {
		t.Fatalf("expected missing non-InferencePool Extra to be added, got %v", got.Extra["other-extra"])
	}
	if _, found := baseInferenceConfigs[otherRouteName]; found {
		t.Fatalf("expected base InferencePool config map not to be mutated by merge")
	}
}
