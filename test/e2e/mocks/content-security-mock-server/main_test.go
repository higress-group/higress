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
			if d.Suggestion != "mask" || d.Type != "sensitiveData" || d.Level != "S3" {
				t.Fatalf("detail=%+v", d)
			}
			if len(d.Result) == 0 || d.Result[0].Ext == nil || d.Result[0].Ext.Desensitization == "" {
				t.Fatalf("expected Ext.Desensitization, got %+v", d.Result)
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
