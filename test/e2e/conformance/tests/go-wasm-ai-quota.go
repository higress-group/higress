// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/config"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsAIQuota)
}

var WasmPluginsAIQuota = suite.ConformanceTest{
	ShortName:   "WasmPluginsAIQuota",
	Description: "Test ai-quota admission, administration and streaming token deduction.",
	Manifests:   []string{"tests/go-wasm-ai-quota.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, s *suite.ConformanceTestSuite) {
		runAIQuota(t, s.GatewayAddress, s.TimeoutConfig)
	},
}

const aiQuotaHost = "ai-quota.test"
const aiQuotaAdminKey = "quota-admin-key"
const aiQuotaConsumer = "quota-user"
const aiQuotaUserKey = "quota-user-key"
const aiQuotaAdminPath = "/v1/chat/completions/quota"

type aiQuotaClient struct {
	address string
	client  *http.Client
}

type aiQuotaReply struct {
	status      int
	body        []byte
	contentType string
}

func (c *aiQuotaClient) request(ctx context.Context, method, path, key, contentType string, body []byte) (aiQuotaReply, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://"+c.address+path, bytes.NewReader(body))
	if err != nil {
		return aiQuotaReply{}, err
	}
	req.Host = aiQuotaHost
	if key != "" {
		req.Header.Set("x-api-key", key)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return aiQuotaReply{}, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	return aiQuotaReply{res.StatusCode, data, res.Header.Get("Content-Type")}, err
}

func decodeAIQuota(r aiQuotaReply, expectedConsumer string) (int, error) {
	if r.status != http.StatusOK {
		return 0, fmt.Errorf("quota query status=%d body=%s", r.status, r.body)
	}
	var result struct {
		Consumer string `json:"consumer"`
		Quota    *int   `json:"quota"`
	}
	if err := json.Unmarshal(r.body, &result); err != nil {
		return 0, err
	}
	if result.Consumer != expectedConsumer || result.Quota == nil {
		return 0, fmt.Errorf("not a quota response for %q: %s", expectedConsumer, r.body)
	}
	return *result.Quota, nil
}

func (c *aiQuotaClient) quota(ctx context.Context, name string) (int, error) {
	r, err := c.request(ctx, http.MethodGet, aiQuotaAdminPath+"?consumer="+url.QueryEscape(name), aiQuotaAdminKey, "", nil)
	if err != nil {
		return 0, err
	}
	return decodeAIQuota(r, name)
}

func runAIQuota(t *testing.T, address string, timeout config.TimeoutConfig) {
	t.Helper()
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	t.Cleanup(tr.CloseIdleConnections)
	c := &aiQuotaClient{
		address: address,
		client: &http.Client{
			Transport: tr,
			Timeout:   timeout.RequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	// Poll only read-only requests; replaying a completion or delta spends quota again.
	waitQuota := func(t *testing.T, name string, want int) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), timeout.MaxTimeToConsistency)
		defer cancel()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			got, err := c.quota(ctx, name)
			if err == nil && got == want {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatalf("%s quota=%d want=%d: %v (last query error: %v)", name, got, want, ctx.Err(), err)
			case <-ticker.C:
			}
		}
	}
	waitQuota(t, "quota-readiness", 0)
	send := func(t *testing.T, method, path, key, typ string, body []byte, status int) aiQuotaReply {
		t.Helper()
		r, err := c.request(context.Background(), method, path, key, typ, body)
		if err != nil {
			t.Fatal(err)
		}
		if r.status != status {
			t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, r.status, status, r.body)
		}
		return r
	}
	refresh := func(t *testing.T, name string, value int) {
		t.Helper()
		body := url.Values{"consumer": {name}, "quota": {fmt.Sprint(value)}}.Encode()
		r := send(t, http.MethodPost, aiQuotaAdminPath+"/refresh", aiQuotaAdminKey, "application/x-www-form-urlencoded", []byte(body), 200)
		if string(r.body) != "refresh quota successful" {
			t.Fatalf("unexpected refresh response: %s", r.body)
		}
	}
	expectQuota := func(t *testing.T, name string, want int) {
		t.Helper()
		got, err := c.quota(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s quota=%d want=%d", name, got, want)
		}
	}
	completion := []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	t.Run("missing credential", func(t *testing.T) {
		send(t, http.MethodPost, "/v1/chat/completions", "", "application/json", completion, 401)
	})
	t.Run("invalid credential", func(t *testing.T) {
		send(t, http.MethodPost, "/v1/chat/completions", "invalid-key", "application/json", completion, 403)
	})
	t.Run("missing quota key", func(t *testing.T) {
		r := send(t, http.MethodPost, "/v1/chat/completions", "quota-missing-key", "application/json", completion, 403)
		if !bytes.Contains(r.body, []byte("No quota left")) {
			t.Fatalf("not a quota rejection: %s", r.body)
		}
	})
	for _, value := range []int{0, -1} {
		t.Run(fmt.Sprintf("quota %d denied", value), func(t *testing.T) {
			refresh(t, aiQuotaConsumer, value)
			r := send(t, http.MethodPost, "/v1/chat/completions", aiQuotaUserKey, "application/json", completion, 403)
			if !bytes.Contains(r.body, []byte("No quota left")) {
				t.Fatalf("not a quota rejection: %s", r.body)
			}
			expectQuota(t, aiQuotaConsumer, value)
		})
	}
	t.Run("refresh query and signed delta", func(t *testing.T) {
		refresh(t, aiQuotaConsumer, 100)
		expectQuota(t, aiQuotaConsumer, 100)
		for _, tc := range []struct{ delta, want int }{{20, 120}, {-30, 90}} {
			body := url.Values{"consumer": {aiQuotaConsumer}, "value": {fmt.Sprint(tc.delta)}}.Encode()
			send(t, http.MethodPost, aiQuotaAdminPath+"/delta", aiQuotaAdminKey, "application/x-www-form-urlencoded", []byte(body), 200)
			expectQuota(t, aiQuotaConsumer, tc.want)
		}
	})
	t.Run("consumer isolation", func(t *testing.T) {
		refresh(t, aiQuotaConsumer, 100)
		refresh(t, "quota-other", 70)
		body := url.Values{"consumer": {aiQuotaConsumer}, "value": {"-25"}}.Encode()
		send(t, http.MethodPost, aiQuotaAdminPath+"/delta", aiQuotaAdminKey, "application/x-www-form-urlencoded", []byte(body), 200)
		expectQuota(t, aiQuotaConsumer, 75)
		expectQuota(t, "quota-other", 70)
	})
	t.Run("non admin cannot manage quota", func(t *testing.T) {
		refresh(t, aiQuotaConsumer, 100)
		send(t, http.MethodGet, aiQuotaAdminPath+"?consumer="+url.QueryEscape(aiQuotaConsumer), aiQuotaUserKey, "", nil, 403)
		refreshBody := url.Values{"consumer": {aiQuotaConsumer}, "quota": {"999"}}.Encode()
		send(t, http.MethodPost, aiQuotaAdminPath+"/refresh", aiQuotaUserKey, "application/x-www-form-urlencoded", []byte(refreshBody), 403)
		deltaBody := url.Values{"consumer": {aiQuotaConsumer}, "value": {"999"}}.Encode()
		send(t, http.MethodPost, aiQuotaAdminPath+"/delta", aiQuotaUserKey, "application/x-www-form-urlencoded", []byte(deltaBody), 403)
		expectQuota(t, aiQuotaConsumer, 100)
	})
	t.Run("positive quota admits an LLM stream", func(t *testing.T) {
		refresh(t, aiQuotaConsumer, 100)
		r := send(t, http.MethodPost, "/v1/chat/completions", aiQuotaUserKey, "application/json", completion, 200)
		if !strings.HasPrefix(r.contentType, "text/event-stream") || !bytes.Contains(r.body, []byte("chatcmpl-llm-mock")) || !bytes.Contains(r.body, []byte("data: [DONE]")) {
			t.Fatalf("expected completed LLM mock stream, type=%q body=%s", r.contentType, r.body)
		}
	})
	// Use Gemini because the OpenAI mock stream does not include token usage.
	t.Run("stream usage deducted once and exhaustion denied", func(t *testing.T) {
		refresh(t, aiQuotaConsumer, 10)
		refresh(t, "quota-other", 70)
		path := "/v1beta/models/gemini-2.0-flash:streamGenerateContent?key=mock-only"
		body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
		r := send(t, http.MethodPost, path, aiQuotaUserKey, "application/json", body, 200)
		if !strings.HasPrefix(r.contentType, "text/event-stream") {
			t.Fatalf("expected complete Gemini mock SSE: %s", r.body)
		}
		var total int
		var finished bool
		for _, line := range bytes.Split(r.body, []byte("\n")) {
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			var event struct {
				Candidates []struct {
					FinishReason string `json:"finishReason"`
				} `json:"candidates"`
				Usage struct {
					Input  int `json:"promptTokenCount"`
					Output int `json:"candidatesTokenCount"`
					Total  int `json:"totalTokenCount"`
				} `json:"usageMetadata"`
			}
			if err := json.Unmarshal(payload, &event); err != nil {
				t.Fatalf("invalid SSE event: %s", payload)
			}
			for _, candidate := range event.Candidates {
				if candidate.FinishReason == "STOP" {
					finished = true
				}
			}
			if event.Usage.Total > 0 {
				if event.Usage.Input+event.Usage.Output != event.Usage.Total {
					t.Fatalf("inconsistent mock usage: %s", payload)
				}
				total = event.Usage.Total
			}
		}
		if !finished {
			t.Fatal("Gemini stream did not finish with STOP")
		}
		if total != 10 {
			t.Fatalf("expected fixture usage 10, got %d", total)
		}
		waitQuota(t, aiQuotaConsumer, 0)
		expectQuota(t, "quota-other", 70)
		r = send(t, http.MethodPost, path, aiQuotaUserKey, "application/json", body, 403)
		if !bytes.Contains(r.body, []byte("No quota left")) {
			t.Fatalf("not a quota rejection: %s", r.body)
		}
		expectQuota(t, aiQuotaConsumer, 0)
	})
}
