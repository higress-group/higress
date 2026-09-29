package main

import (
	"testing"

	"ext-auth/config"

	"github.com/tidwall/gjson"
)

// OPE-8474 方式二：路径前缀门的语义单测（替代官方 match_list——线上版本语义与源码相反）
func TestShouldAuthByPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/v1-ent/chat/completions", true},  // 企业路由：认证
		{"/v1-ent", true},                   // 前缀本身
		{"/v1/chat/completions", false},     // 现网共享路由：放行（零影响的生命线）
		{"/v1-entries/other", true},         // 前缀语义：/v1-ent* 都算（与 ingress prefix match 一致）
		{"/models", false},
		{"/v1ent", false},                   // 无斜杠边界不算
		{"/", false},
	}
	orig := entPathPrefixes
	defer func() { entPathPrefixes = orig }()
	entPathPrefixes = []string{"/v1-ent"}
	for _, c := range cases {
		if got := shouldAuthByPath(c.path); got != c.want {
			t.Errorf("shouldAuthByPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}

	// 配置化多前缀
	entPathPrefixes = []string{"/v1-ent", "/v2-ent"}
	if !shouldAuthByPath("/v2-ent/x") || shouldAuthByPath("/v1/x") {
		t.Error("multi-prefix config failed")
	}
}

// parseEntConfig：官方 http_service 校验保留（缺省必报错），ent_path_prefixes 可覆盖默认
func TestParseEntConfig(t *testing.T) {
	err := parseEntConfig(gjson.Parse(`{}`), &config.ExtAuthConfig{})
	if err == nil || err.Error() != "missing http_service in config" {
		t.Fatalf("expected missing http_service error, got: %v", err)
	}

	orig := entPathPrefixes
	defer func() { entPathPrefixes = orig }()
	cfg := config.ExtAuthConfig{}
	// 最小合法 http_service（forward_auth 指向 verify 端点）
	// 注意 allowed_headers/allowed_upstream_headers 是 StringMatcher 数组（{exact: ...}），裸字符串会解析失败——
	// 这也是 ops 线上配置需要核对的一个坑：格式错会导致整条规则作废。
	min := `{"http_service":{"endpoint_mode":"forward_auth","endpoint":{"service_name":"wujieai-pref.dns","service_port":443,"path":"/account-private/aipc/enterprise/token/verify"},"authorization_request":{"allowed_headers":[{"exact":"authorization"}]},"authorization_response":{"allowed_upstream_headers":[{"exact":"x-original-api-key"}]}},"failure_mode_allow":false,"status_on_error":401,"ent_path_prefixes":"/v1-ent,/v9-ent"}`
	if err := parseEntConfig(gjson.Parse(min), &cfg); err != nil {
		t.Fatalf("valid config should parse: %v", err)
	}
	if len(entPathPrefixes) != 2 || entPathPrefixes[1] != "/v9-ent" {
		t.Errorf("ent_path_prefixes override failed: %v", entPathPrefixes)
	}
}
