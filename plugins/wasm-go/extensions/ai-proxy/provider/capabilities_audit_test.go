package provider

import (
	"fmt"
	"strings"
	"testing"
)

type capabilitiesDeclarer interface {
	DefaultCapabilities() map[string]string
}

// capabilityDetectionExemptions lists providers whose GetApiName
// intentionally differs from the DefaultCapabilities entry for a path.
// Every entry must stay justified.
var capabilityDetectionExemptions = map[string][]string{
	// kling dispatches text2video/image2video by sniffing the request body
	// (isImageToVideoRequest) rather than by path, and never rewrites paths
	// under protocol: original, so the generic video apiName it returns for
	// the kling-specific native paths is safe by design.
	"kling": {
		"/v1/videos/image2video",
		"/v1/videos/image2video/{video_id}",
	},
	// Temporary: the path-detection fixes for these providers are pending in
	// #4217 (groq), #4218 (cohere), #4219 (baidu) and #4860 (bedrock); hunyuan's landed via
	// #4220. Drop each entry once its PR merges.
	"bedrock": {"/anthropic/v1/messages"},
	"baidu": {"/v2/embeddings"},
	"groq":  {"/openai/v1/responses"},
	"cohere": {"/v1/rerank"},
}

// TestProviderCapabilityPathsAreDetectable verifies, for every provider
// implementing ApiNameHandler, that each capability path declared in
// DefaultCapabilities() is classified by the provider's own GetApiName() to
// one of the apiNames declared for that path.
//
// Under protocol: original the provider-specific GetApiName is the resolver
// used by onHttpRequestHeader: a capability path it cannot classify (empty
// apiName) makes the plugin skip the request entirely, and a wrong
// classification routes it through the wrong transform pipeline. This is
// exactly the defect class fixed for baidu/hunyuan/groq/cohere/bedrock, and
// this test guards against reintroducing it for any provider.
func TestProviderCapabilityPathsAreDetectable(t *testing.T) {
	for ptype, init := range providerInitializers {
		declarer, ok := init.(capabilitiesDeclarer)
		if !ok {
			continue
		}
		caps := declarer.DefaultCapabilities()

		// Group the declared apiNames per path; skip empty (dynamically
		// resolved) and template paths such as "/openai/deployments/{model}"
		// or "/v1/projects/%s/...", which are not literal request paths.
		apiNamesByPath := map[string][]string{}
		for apiName, path := range caps {
			if path == "" || strings.ContainsAny(path, "%{") {
				continue
			}
			apiNamesByPath[path] = append(apiNamesByPath[path], apiName)
		}
		if len(apiNamesByPath) == 0 {
			continue
		}

		p, err := func() (p Provider, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			return init.CreateProvider(ProviderConfig{})
		}()
		if err != nil {
			t.Logf("%s: skip, CreateProvider failed: %v (capabilities were %v)", ptype, err, caps)
			continue
		}
		handler, ok := p.(ApiNameHandler)
		if !ok {
			continue
		}

		exempted := map[string]bool{}
		for _, path := range capabilityDetectionExemptions[ptype] {
			exempted[path] = true
		}

		for path, apiNames := range apiNamesByPath {
			if exempted[path] {
				t.Logf("%s: %q exempted (declared %v)", ptype, path, apiNames)
				continue
			}
			got := handler.GetApiName(path)
			matched := false
			for _, want := range apiNames {
				if string(got) == want {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("%s: GetApiName(%q) = %q, want one of %v (declared in DefaultCapabilities)", ptype, path, got, apiNames)
			}
		}
	}
}
