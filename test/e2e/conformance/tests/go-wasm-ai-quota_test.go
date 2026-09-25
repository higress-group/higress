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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAIQuotaDecode(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       int
		valid      bool
	}{
		{"positive", `{"consumer":"alice","quota":100}`, 200, 100, true},
		{"zero", `{"consumer":"alice","quota":0}`, 200, 0, true},
		{"negative", `{"consumer":"alice","quota":-1}`, 200, -1, true},
		{"upstream echo", `{"path":"/v1/chat/completions/quota"}`, 200, 0, false},
		{"wrong consumer", `{"consumer":"bob","quota":100}`, 200, 0, false},
		{"missing quota", `{"consumer":"alice"}`, 200, 0, false},
		{"null quota", `{"consumer":"alice","quota":null}`, 200, 0, false},
		{"wrong type", `{"consumer":"alice","quota":"100"}`, 200, 0, false},
		{"error", `{"consumer":"alice","quota":100}`, 403, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeAIQuota(aiQuotaReply{status: tc.status, body: []byte(tc.body)}, "alice")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t err=%v", tc.valid, err)
			}
			if tc.valid && got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestAIQuotaRequestDoesNotRetryMutation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Host != aiQuotaHost || r.Header.Get("x-api-key") != aiQuotaAdminKey {
			t.Errorf("incorrect gateway routing/authentication")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.FormValue("value") != "-10" {
			t.Errorf("form body lost")
		}
		w.WriteHeader(503)
	}))
	defer server.Close()
	c := &aiQuotaClient{strings.TrimPrefix(server.URL, "http://"), server.Client()}
	r, err := c.request(context.Background(), http.MethodPost, "/v1/chat/completions/quota/delta", aiQuotaAdminKey, "application/x-www-form-urlencoded", []byte("consumer=alice&value=-10"))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || r.status != 503 {
		t.Fatalf("calls=%d status=%d", calls, r.status)
	}
}

func TestAIQuotaEscapesConsumer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("consumer") != "a&b" {
			t.Errorf("consumer query not escaped: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"consumer":"a&b","quota":7}`))
	}))
	defer server.Close()
	c := &aiQuotaClient{strings.TrimPrefix(server.URL, "http://"), server.Client()}
	got, err := c.quota(context.Background(), "a&b")
	if err != nil || got != 7 {
		t.Fatalf("got=%d err=%v", got, err)
	}
}

func TestAIQuotaQueryDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	c := &aiQuotaClient{strings.TrimPrefix(server.URL, "http://"), server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.quota(ctx, "alice")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
