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

// content-security-mock-server simulates Alibaba Cloud Content Security (绿网)
// TextModerationPlus / MultiModalGuard APIs for Higress ai-security-guard e2e tests.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	listenAddr    = ":8090"
	serverVersion = "content-security-mock-server/1.0"
	maxBodySize   = 10 << 20 // 10MB
)

// Response mirrors plugins/wasm-go/extensions/ai-security-guard/config.Response.
type Response struct {
	Code      int    `json:"Code"`
	Message   string `json:"Message"`
	RequestId string `json:"RequestId"`
	Data      Data   `json:"Data"`
}

type Data struct {
	RiskLevel   string   `json:"RiskLevel,omitempty"`
	AttackLevel string   `json:"AttackLevel,omitempty"`
	Suggestion  string   `json:"Suggestion,omitempty"`
	Advice      []Advice `json:"Advice,omitempty"`
	Detail      []Detail `json:"Detail,omitempty"`
}

type Advice struct {
	Answer string `json:"Answer,omitempty"`
}

type Detail struct {
	Suggestion string   `json:"Suggestion,omitempty"`
	Type       string   `json:"Type,omitempty"`
	Level      string   `json:"Level,omitempty"`
	Result     []Result `json:"Result,omitempty"`
}

type Result struct {
	Label       string  `json:"Label,omitempty"`
	Description string  `json:"Description,omitempty"`
	Confidence  float64 `json:"Confidence,omitempty"`
	Ext         *Ext    `json:"Ext,omitempty"`
}

type Ext struct {
	Desensitization string `json:"Desensitization,omitempty"`
}

type serviceParameters struct {
	Content     string `json:"content"`
	SessionId   string `json:"sessionId"`
	RequestFrom string `json:"requestFrom"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/", moderationHandler)

	log.Printf("Starting %s on %s", serverVersion, listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, withHeaders(mux)))
}

func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Mock-Server-Timestamp", time.Now().UTC().Format(time.RFC3339))
		w.Header().Set("X-Server-Name", serverVersion)
		next.ServeHTTP(w, r)
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// moderationHandler accepts POST / as Aliyun Content Security does.
// Query: Service=<checkService>
// Body (form): ServiceParameters=<JSON with content, sessionId, requestFrom>
// Headers: Authorization (ACS3 style; presence checked, signature not verified),
//
//	x-acs-action, x-acs-version, content-type, User-Agent, etc.
func moderationHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Optional auth presence check (e2e uses mock AK/SK; no ACS3 verification).
	if r.Header.Get("Authorization") == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Missing required request header"})
		return
	}

	service := r.URL.Query().Get("Service")
	action := r.Header.Get("x-acs-action")

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Error reading request body"})
		return
	}
	_ = r.Body.Close()

	content := extractContent(body)
	snippet := content
	if len(snippet) > 128 {
		snippet = snippet[:128] + "..."
	}
	log.Printf("Method=%s Path=%s Service=%s Action=%s Content=%q", r.Method, r.URL.Path, service, action, snippet)

	resp := buildResponse(content)
	writeJSON(w, http.StatusOK, resp)
}

func extractContent(rawBody []byte) string {
	values, err := url.ParseQuery(string(rawBody))
	if err != nil {
		return ""
	}
	sp := values.Get("ServiceParameters")
	if sp == "" {
		return ""
	}
	var params serviceParameters
	if err := json.Unmarshal([]byte(sp), &params); err == nil {
		return params.Content
	}
	return sp
}

// buildResponse applies keyword-based risk simulation:
//   - content containing BLOCK / 违规 / illegal → RiskLevel high, Suggestion block
//   - content containing MASK / 敏感 → Detail with Suggestion mask (sensitiveData / S3)
//   - otherwise → RiskLevel none (pass)
//
// Block keywords take precedence when both match.
func buildResponse(content string) Response {
	reqID := newRequestID()
	resp := Response{
		Code:      200,
		Message:   "Success",
		RequestId: reqID,
	}

	lower := strings.ToLower(content)
	block := strings.Contains(content, "BLOCK") ||
		strings.Contains(content, "违规") ||
		strings.Contains(lower, "illegal")
	mask := strings.Contains(content, "MASK") ||
		strings.Contains(content, "敏感")

	switch {
	case block:
		resp.Data = Data{
			RiskLevel:  "high",
			Suggestion: "block",
			Advice: []Advice{
				{Answer: "很抱歉，我无法回答您的问题"},
			},
			Detail: []Detail{
				{
					Suggestion: "block",
					Type:       "contentModeration",
					Level:      "high",
					Result: []Result{
						{Label: "mock_block", Description: "keyword BLOCK/违规/illegal matched", Confidence: 1.0},
					},
				},
			},
		}
	case mask:
		masked := "[MASKED]"
		resp.Data = Data{
			RiskLevel:  "none",
			Suggestion: "mask",
			Detail: []Detail{
				{
					Suggestion: "mask",
					Type:       "sensitiveData",
					Level:      "S3",
					Result: []Result{
						{
							Label:       "mock_mask",
							Description: "keyword MASK/敏感 matched",
							Confidence:  1.0,
							Ext:         &Ext{Desensitization: masked},
						},
					},
				},
			},
		}
	default:
		resp.Data = Data{
			RiskLevel:  "none",
			Suggestion: "pass",
		}
	}
	return resp
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}
