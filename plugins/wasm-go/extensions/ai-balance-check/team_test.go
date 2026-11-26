package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
)

func TestTeamAdmission(t *testing.T) {
	config := []byte(`{"teamBilling":true,"apiUrl":"https://example.com/legacy","teamApiUrl":"https://example.com/check","serviceName":"admission"}`)
	test.RunTest(t, func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			upstream  string
			body      string
			wantError uint32
			wantCode  string
		}{
			{"accepted", "200", `{"teamId":"42"}`, 0, ""},
			{"not a member", "403", "", 422, "team_access_denied"},
			{"negative balance", "402", "", 422, "team_credits_unavailable"},
			{"missing internal id", "200", "{}", 503, "team_billing_unavailable"},
			{"upstream failure", "500", "", 503, "team_billing_unavailable"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				host, status := test.NewTestHost(config)
				defer host.Reset()
				if status != types.OnPluginStartStatusOK {
					t.Fatal(status)
				}
				const uuid = "9943d4d9-d20b-49db-809d-88586f950fc7"
				if action := host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "example.com"},
					{OriginalAPIKey, "unit-test-key"},
					{teamUUIDHeader, uuid},
				}); action != types.HeaderStopAllIterationAndWatermark {
					t.Fatalf("action = %v", action)
				}
				calls := host.GetHttpCalloutAttributes()
				if len(calls) != 1 || !hasCalloutHeader(calls[0].Headers, ":path", "/check") || string(calls[0].Body) != fmt.Sprintf(`{"teamUuid":%q}`, uuid) {
					t.Fatalf("unexpected admission calls: %+v", calls)
				}
				host.CallOnHttpCall([][2]string{{":status", tc.upstream}}, []byte(tc.body))
				local := host.GetLocalResponse()
				if tc.wantError == 0 {
					value, err := proxywasm.GetProperty([]string{teamIDProperty})
					if local != nil || err != nil || string(value) != "42" {
						t.Fatalf("expected validated ID and no local error: %q %v %+v", value, err, local)
					}
				} else if local == nil || local.StatusCode != tc.wantError {
					t.Fatalf("expected status %d, got %+v", tc.wantError, local)
				} else {
					var response OpenAIError
					if err := json.Unmarshal(local.Data, &response); err != nil || response.Error.Code != tc.wantCode {
						t.Fatalf("expected error code %q, got %+v (%v)", tc.wantCode, response, err)
					}
				}
				host.CompleteHttp()
			})
		}
	})
}

func TestMissingTeamSelectorUsesLegacyBalance(t *testing.T) {
	config := []byte(`{"teamBilling":true,"apiUrl":"https://example.com/legacy","teamApiUrl":"https://example.com/check","serviceName":"admission"}`)
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(config)
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatal(status)
		}
		if action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{OriginalAPIKey, "unit-test-key"},
		}); action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("action = %v", action)
		}
		calls := host.GetHttpCalloutAttributes()
		if len(calls) != 1 || len(calls[0].Body) != 0 ||
			!hasCalloutHeader(calls[0].Headers, ":method", "GET") || !hasCalloutHeader(calls[0].Headers, ":path", "/legacy") {
			t.Fatalf("unexpected legacy balance request: %+v", calls)
		}
		host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte("true"))
		if _, err := proxywasm.GetProperty([]string{teamIDProperty}); err != types.ErrorStatusNotFound || host.GetLocalResponse() != nil {
			t.Fatalf("legacy request unexpectedly acquired a team ID or failed")
		}
		host.CompleteHttp()
	})
}

func hasCalloutHeader(headers [][2]string, name, value string) bool {
	for _, header := range headers {
		if header[0] == name && header[1] == value {
			return true
		}
	}
	return false
}

func TestMigratedRouteBilling(t *testing.T) {
	const secret = "fixture-only-0123456789-0123456789"
	uuid := "3493d4d9-d20b-49db-809d-88586f950fc7"
	test.RunTest(t, func(t *testing.T) {
		for _, tc := range []struct {
			name, mode, settlement string
			team                   *string
			admission, wantBody    string
			wantStatus             uint32
		}{
			{"old client gets default team", "compat", "usage", nil, `{"teamId":"42","teamUuid":"` + uuid + `"}`, "{}", 0},
			{"versioned missing selector", "compat", "usage", nil, "", "", 400},
			{"versioned explicit team", "compat", "usage", &uuid, `{"teamId":"42","teamUuid":"` + uuid + `"}`, `{"teamUuid":"` + uuid + `"}`, 0},
			{"new missing selector", "strict", "usage", nil, "", "", 400},
			{"new explicit team", "strict", "usage", &uuid, `{"teamId":"42","teamUuid":"` + uuid + `"}`, `{"teamUuid":"` + uuid + `"}`, 0},
			{"legacy account service still deployed", "compat", "usage", nil, `{"teamId":"42","teamUuid":"` + uuid + `","personalOwnerUid":"7"}`, "{}", 503},
			{"proxy forwards validated default", "compat", "forward", nil, `{"teamId":"42","teamUuid":"` + uuid + `"}`, "{}", 0},
			{"async compat binds gateway id", "compat", "task_create", nil, `{"teamId":"42","teamUuid":"` + uuid + `"}`, "{}", 0},
			{"sync compat binds gateway id", "compat", "task_create_sync", nil, `{"teamId":"42","teamUuid":"` + uuid + `"}`, "{}", 0},
			{"mirror binds gateway id", "compat", "mirror_create", nil, `{"teamId":"42","teamUuid":"` + uuid + `"}`, "{}", 0},
			{"unconfigured settlement", "strict", "unavailable", &uuid, "", "", 503},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cfg := fmt.Sprintf(`{"apiUrl":"https://a.test/legacy","serviceName":"admission","teamBilling":true,"teamApiUrl":"https://a.test/admit","teamRouteMode":%q,"teamSettlement":%q,"teamSigningKey":%q}`, tc.mode, tc.settlement, secret)
				host, status := test.NewTestHost([]byte(cfg))
				defer host.Reset()
				if status != types.OnPluginStartStatusOK {
					t.Fatal(status)
				}
				headers := [][2]string{{":authority", "a.test"}, {":method", "POST"}, {"x-request-id", "request-42"}, {OriginalAPIKey, "fixture-key"}, {admittedTeamHeader, "999"}, {timestampHeader, "100"}, {signatureHeader, "invalid"}}
				if tc.name == "versioned missing selector" || tc.name == "versioned explicit team" {
					headers = append(headers, [2]string{"X-Wujie-Billing-Version", "2"})
				}
				if tc.team != nil {
					headers = append(headers, [2]string{teamUUIDHeader, *tc.team})
				}
				host.CallOnHttpRequestHeaders(headers)
				calls := host.GetHttpCalloutAttributes()
				if tc.wantBody != "" {
					if len(calls) != 1 || string(calls[0].Body) != tc.wantBody || !hasCalloutHeader(calls[0].Headers, ":path", "/admit") {
						t.Fatalf("unexpected admission call %+v", calls)
					}
					host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(tc.admission))
				} else if len(calls) != 0 {
					t.Fatalf("unexpected callouts %v", calls)
				}
				response := host.GetLocalResponse()
				if tc.wantStatus != 0 {
					if response == nil || response.StatusCode != tc.wantStatus {
						t.Fatalf("expected %v got %+v", tc.wantStatus, response)
					}
				} else {
					if response != nil {
						t.Fatal(response)
					}
					id, err := proxywasm.GetProperty([]string{teamIDProperty})
					if err != nil || string(id) != "42" {
						t.Fatal("invalid billing team", string(id), err)
					}
					if tc.settlement == "forward" {
						id, err := proxywasm.GetHttpRequestHeader(teamUUIDHeader)
						if err != nil || id != uuid {
							t.Fatal("validated team not forwarded")
						}
					}
					if signedOperation(tc.settlement) {
						got := map[string]string{}
						for _, header := range host.GetRequestHeaders() {
							got[strings.ToLower(header[0])] = header[1]
						}
						ts := got[strings.ToLower(timestampHeader)]
						hash := sha256.Sum256([]byte("fixture-key"))
						data := fmt.Sprintf("v1\n%s\nrequest-42\n42\n%s\n%x\n%s", ts, uuid, hash, tc.settlement)
						mac := hmac.New(sha256.New, []byte(secret))
						mac.Write([]byte(data))
						if got["request-id"] != "request-42" || got[strings.ToLower(admittedTeamHeader)] != "42" || got[strings.ToLower(admittedTeamUUIDHeader)] != uuid || got[strings.ToLower(signatureHeader)] != hex.EncodeToString(mac.Sum(nil)) {
							t.Fatal("invalid signed assertion")
						}
						v, err := proxywasm.GetProperty([]string{"wasm.teamBillingSettlement"})
						if err != nil || string(v) != tc.settlement {
							t.Fatal("missing async settlement marker")
						}
					} else if v, _ := proxywasm.GetHttpRequestHeader(admittedTeamHeader); v != "" {
						t.Fatalf("untrusted header reached upstream: %q", v)
					}
				}
				host.CompleteHttp()
			})
		}
	})
}

func TestNativeFallbackKeepsAdmissionAcrossRecreatedStream(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		const secret = "fixture-only-fallback-0123456789abcdef"
		const team = "3493d4d9-d20b-49db-809d-88586f950fc7"
		const id = "original-request"
		const key = "fixture-key"
		cfg := func(role string) []byte {
			return []byte(fmt.Sprintf(`{"apiUrl":"https://a.test/legacy","serviceName":"admission","teamBilling":true,"teamApiUrl":"https://a.test/admit","teamRouteMode":"compat","teamSettlement":"usage","teamSigningKey":%q,"teamFallback":%q}`, secret, role))
		}
		host, start := test.NewTestHost(cfg("source"))
		if start != types.OnPluginStartStatusOK {
			t.Fatal(start)
		}
		host.CallOnHttpRequestHeaders([][2]string{{":authority", "a.test"}, {":method", "POST"}, {":path", "/v1/chat/completions"}, {"request-id", id}, {OriginalAPIKey, key}, {"x-higress-llm-model", "test-model"}, {teamUUIDHeader, team}})
		host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"teamId":"42","teamUuid":"`+team+`"}`))
		bridge := map[string]string{}
		for _, name := range []string{"wjFallbackTeam", "wjFallbackRequest", "wjFallbackKey", "wjFallbackModel", "wjFallbackProof"} {
			value, err := proxywasm.GetProperty([]string{name})
			if err != nil || len(value) == 0 {
				t.Fatal("missing fallback state", name)
			}
			bridge[name] = string(value)
		}
		if value, _ := proxywasm.GetHttpRequestHeader(fallbackProofHeader); value != "" {
			t.Fatal("proof reached primary provider")
		}
		if bridge["wjFallbackModel"] != "test-model" {
			t.Fatal("model match changed")
		}
		host.Reset()
		for _, tc := range []struct {
			name, team, request, credential, proof string
			valid                                  bool
		}{
			{"original execution", bridge["wjFallbackTeam"], bridge["wjFallbackRequest"], bridge["wjFallbackKey"], bridge["wjFallbackProof"], true},
			{"missing state", "", id, key, "", false},
			{"external fallback marker", team, id, key, "untrusted", false},
			{"changed request ID", team, "new-request", key, bridge["wjFallbackProof"], false},
			{"changed actor", team, id, "another-key", bridge["wjFallbackProof"], false},
			{"changed team", "f79d1512-4055-41f4-b170-1de2bdb800ff", id, key, bridge["wjFallbackProof"], false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				h, _ := test.NewTestHost(cfg("target"))
				defer h.Reset()
				h.CallOnHttpRequestHeaders([][2]string{{":authority", "a.test"}, {":method", "POST"}, {":path", "/v1/chat/completions"}, {"request-id", tc.request}, {OriginalAPIKey, tc.credential}, {teamUUIDHeader, tc.team}, {fallbackProofHeader, tc.proof}, {"x-higress-fallback-from", "main"}, {"x-higress-llm-model", "test-model"}})
				calls := h.GetHttpCalloutAttributes()
				if !tc.valid {
					if len(calls) != 0 || h.GetLocalResponse() == nil || h.GetLocalResponse().StatusCode != 503 {
						t.Fatal("unsafe fallback was admitted")
					}
					return
				}
				if len(calls) != 1 || string(calls[0].Body) != `{"teamUuid":"`+team+`"}` {
					t.Fatal("target did not explicitly read the admitted team")
				}
				h.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"teamId":"42","teamUuid":"`+team+`"}`))
				if value, _ := proxywasm.GetHttpRequestHeader(fallbackProofHeader); value != "" {
					t.Fatal("proof reached standby provider")
				}
				if value, _ := proxywasm.GetHttpRequestHeader("request-id"); value != id {
					t.Fatal("request ID changed")
				}
				if value, _ := proxywasm.GetProperty([]string{teamIDProperty}); string(value) != "42" {
					t.Fatal("target ledger changed")
				}
			})
		}
	})
}

func TestVersionOnlyGuard(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, marked := range []bool{false, true} {
			t.Run(fmt.Sprint(marked), func(t *testing.T) {
				host, status := test.NewTestHost([]byte(`{"teamVersionGuardOnly":true}`))
				defer host.Reset()
				if status != types.OnPluginStartStatusOK {
					t.Fatal(status)
				}
				headers := [][2]string{{":authority", "a.test"}, {":path", "/v1/test"}}
				if marked {
					headers = append(headers, [2]string{"X-Wujie-Billing-Version", "2"})
				}
				host.CallOnHttpRequestHeaders(headers)
				got := host.GetLocalResponse()
				if marked && (got == nil || got.StatusCode != 503) || !marked && got != nil || len(host.GetHttpCalloutAttributes()) != 0 {
					t.Fatal("unexpected guard decision", marked, got)
				}
				host.CompleteHttp()
			})
		}
	})
}

func TestPaidPathScope(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		const legacy = `{"apiUrl":"https://a.test/old-check","serviceName":"billing"}`
		for _, tc := range []struct {
			name, path, previous, version, call string
			status                              uint32
		}{
			{"unlisted read with original check disabled", "/v1/models?all=true", "{}", "", "", 0},
			{"unlisted read with original check enabled", "/v1/models", legacy, "", "/old-check", 0},
			{"unlisted versioned request", "/v1/unknown", legacy, "2", "", 503},
			{"listed paid request still needs team", "/v1/chat/completions?stream=true", legacy, "2", "", 400},
			{"listed compatible request admits default team", "/v1/chat/completions", legacy, "", "/admit", 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cfg := fmt.Sprintf(`{"teamPaidPaths":["/v1/chat/completions"],"teamUnlistedPathConfig":%s,"apiUrl":"https://a.test/legacy","teamBilling":true,"teamApiUrl":"https://a.test/admit","teamRouteMode":"compat","teamSettlement":"usage","serviceName":"billing"}`, tc.previous)
				host, start := test.NewTestHost([]byte(cfg))
				defer host.Reset()
				if start != types.OnPluginStartStatusOK {
					t.Fatal(start)
				}
				method := "POST"
				if strings.HasPrefix(tc.path, "/v1/models") {
					method = "GET"
				}
				headers := [][2]string{{":authority", "a.test"}, {":method", method}, {":path", tc.path}, {OriginalAPIKey, "fixture"}}
				if tc.version != "" {
					headers = append(headers, [2]string{"X-Wujie-Billing-Version", tc.version})
				}
				host.CallOnHttpRequestHeaders(headers)
				local := host.GetLocalResponse()
				if tc.status != 0 && (local == nil || local.StatusCode != tc.status) || tc.status == 0 && local != nil {
					t.Fatalf("local response = %+v", local)
				}
				calls := host.GetHttpCalloutAttributes()
				if tc.call == "" && len(calls) != 0 || tc.call != "" && (len(calls) != 1 || !hasCalloutHeader(calls[0].Headers, ":path", tc.call)) {
					t.Fatalf("unexpected admission calls: %d", len(calls))
				}
				if tc.status == 0 && strings.HasPrefix(tc.path, "/v1/models") {
					v, e := proxywasm.GetProperty([]string{"wasm.teamBillingUnlistedPath"})
					if e != nil || string(v) != "1" {
						t.Fatal("missing original policy selection")
					}
				}
			})
		}
	})
}
