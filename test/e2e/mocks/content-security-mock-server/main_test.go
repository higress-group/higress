// Copyright (c) 2025 Alibaba Group Holding Ltd.
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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func postModeration(t *testing.T, content, service string, withAuth bool) *httptest.ResponseRecorder {
	t.Helper()
	params, err := json.Marshal(map[string]string{
		"content":     content,
		"sessionId":   "sess-1",
		"requestFrom": "CIPFrom/AIGateway",
	})
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{}
	form.Set("ServiceParameters", string(params))

	req := httptest.NewRequest(http.MethodPost, "/?Service="+url.QueryEscape(service), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-acs-action", "TextModerationPlus")
	req.Header.Set("x-acs-version", "2022-03-02")
	req.Header.Set("User-Agent", "CIPFrom/AIGateway")
	if withAuth {
		req.Header.Set("Authorization", "ACS3-HMAC-SHA256 Credential=mock,SignedHeaders=host,Signature=mock")
	}

	rr := httptest.NewRecorder()
	moderationHandler(rr, req)
	return rr
}

func decodeResponse(t *testing.T, rr *httptest.ResponseRecorder) Response {
	t.Helper()
	var resp Response
	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	return resp
}

func TestPass(t *testing.T) {
	rr := postModeration(t, "hello world, this is fine", "llm_query_moderation", true)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	resp := decodeResponse(t, rr)
	if resp.Code != 200 || resp.Message != "Success" {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	if resp.RequestId == "" {
		t.Fatal("missing RequestId")
	}
	if resp.Data.RiskLevel != "none" {
		t.Fatalf("RiskLevel=%q want none", resp.Data.RiskLevel)
	}
	if resp.Data.Suggestion != "pass" {
		t.Fatalf("Suggestion=%q want pass", resp.Data.Suggestion)
	}
}

func TestBlockKeywords(t *testing.T) {
	cases := []string{"please BLOCK this", "这是违规内容", "this is illegal content"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			rr := postModeration(t, c, "llm_query_moderation", true)
			if rr.Code != http.StatusOK {
				t.Fatalf("HTTP status=%d (Aliyun returns 200 even when risky)", rr.Code)
			}
			resp := decodeResponse(t, rr)
			if resp.Code != 200 {
				t.Fatalf("Code=%d", resp.Code)
			}
			if resp.Data.RiskLevel != "high" {
				t.Fatalf("RiskLevel=%q want high", resp.Data.RiskLevel)
			}
			if resp.Data.Suggestion != "block" {
				t.Fatalf("Suggestion=%q want block", resp.Data.Suggestion)
			}
			if len(resp.Data.Advice) == 0 || resp.Data.Advice[0].Answer == "" {
				t.Fatal("expected Advice Answer for block")
			}
			if len(resp.Data.Detail) == 0 || resp.Data.Detail[0].Type != "contentModeration" {
				t.Fatalf("expected contentModeration detail, got %+v", resp.Data.Detail)
			}
		})
	}
}

func TestMaskKeywords(t *testing.T) {
	cases := []string{"MASK my phone number", "包含敏感信息"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			rr := postModeration(t, c, "llm_response_moderation", true)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d", rr.Code)
			}
			resp := decodeResponse(t, rr)
			if resp.Data.RiskLevel != "none" {
				t.Fatalf("RiskLevel=%q want none for mask path", resp.Data.RiskLevel)
			}
			if resp.Data.Suggestion != "mask" {
				t.Fatalf("Suggestion=%q want mask", resp.Data.Suggestion)
			}
			if len(resp.Data.Detail) != 1 {
				t.Fatalf("Detail len=%d", len(resp.Data.Detail))
			}
			d := resp.Data.Detail[0]
			if d.Suggestion != "mask" || d.Type != "sensitiveData" || d.Level != "S4" {
				t.Fatalf("detail=%+v", d)
			}
			if len(d.Result) == 0 || d.Result[0].Ext == nil || d.Result[0].Ext.Desensitization == "" || len(d.Result[0].Ext.SensitiveData) == 0 {
				t.Fatalf("expected Ext.Desensitization and Ext.SensitiveData, got %+v", d.Result)
			}
		})
	}
}

func TestBlockTakesPrecedenceOverMask(t *testing.T) {
	rr := postModeration(t, "BLOCK and MASK together", "query_security_check", true)
	resp := decodeResponse(t, rr)
	if resp.Data.RiskLevel != "high" || resp.Data.Suggestion != "block" {
		t.Fatalf("want block precedence, got RiskLevel=%q Suggestion=%q", resp.Data.RiskLevel, resp.Data.Suggestion)
	}
}

func TestMaskExtJSONShape(t *testing.T) {
	// Data.Detail[].Result[].Ext.SensitiveData must be a string array, as parsed by
	// the plugin's config.Ext.
	rr := postModeration(t, "MASK phone", "query_security_check", true)
	var m struct {
		Data struct {
			Detail []struct {
				Result []struct {
					Ext map[string]interface{} `json:"Ext"`
				} `json:"Result"`
			} `json:"Detail"`
		} `json:"Data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	ext := m.Data.Detail[0].Result[0].Ext
	if _, ok := ext["Desensitization"].(string); !ok {
		t.Fatalf("Ext.Desensitization not a string: %v", ext)
	}
	data, ok := ext["SensitiveData"].([]interface{})
	if !ok || len(data) == 0 {
		t.Fatalf("Ext.SensitiveData not a non-empty array: %v", ext)
	}
	if _, ok := data[0].(string); !ok {
		t.Fatalf("Ext.SensitiveData element not a string: %v", data)
	}
}

func TestThrottle(t *testing.T) {
	rr := postModeration(t, "please THROTTLE", "llm_query_moderation", true)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want 429", rr.Code)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["Code"] != "Throttling.User" || m["RequestId"] == "" {
		t.Fatalf("unexpected body: %v", m)
	}
}

func TestBusinessError(t *testing.T) {
	rr := postModeration(t, "trigger ERROR", "llm_query_moderation", true)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rr.Code)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["Code"] != float64(500) {
		t.Fatalf("Code=%v want 500", m["Code"])
	}
	if _, ok := m["Data"]; ok {
		t.Fatalf("business error should not carry Data: %v", m)
	}
}

func TestTimeoutDelaysResponse(t *testing.T) {
	old := timeoutDelay
	timeoutDelay = 50 * time.Millisecond
	t.Cleanup(func() { timeoutDelay = old })

	start := time.Now()
	rr := postModeration(t, "TIMEOUT please", "llm_query_moderation", true)
	if elapsed := time.Since(start); elapsed < timeoutDelay {
		t.Fatalf("responded after %v, want >= %v", elapsed, timeoutDelay)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	resp := decodeResponse(t, rr)
	if resp.Code != 200 || resp.Data.Suggestion != "pass" {
		t.Fatalf("unexpected response after delay: %+v", resp)
	}
}

func TestMissingAuthorization(t *testing.T) {
	rr := postModeration(t, "hello", "llm_query_moderation", false)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rr.Code)
	}
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	healthHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestExtractContent(t *testing.T) {
	params, _ := json.Marshal(map[string]string{"content": "abc", "sessionId": "s"})
	form := url.Values{}
	form.Set("ServiceParameters", string(params))
	got := extractContent([]byte(form.Encode()))
	if got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildResponseJSONShape(t *testing.T) {
	// Align with plugin unit-test examples:
	// pass: {"Code":200,"Message":"Success","RequestId":"...","Data":{"RiskLevel":"none"|"low"}}
	// block: RiskLevel "high"
	pass := buildResponse("ok")
	raw, _ := json.Marshal(pass)
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"Code", "Message", "RequestId", "Data"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing key %s in %s", key, raw)
		}
	}
	block := buildResponse("BLOCK")
	if block.Data.RiskLevel != "high" {
		t.Fatalf("block RiskLevel=%q", block.Data.RiskLevel)
	}
}
