package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const admittedTeamHeader = "X-Wujie-Admitted-Team-Id"
const admittedTeamUUIDHeader = "X-Wujie-Admitted-Team-Uuid"
const signatureHeader = "X-Wujie-Team-Signature"
const timestampHeader = "X-Wujie-Team-Timestamp"

const teamUUIDHeader = "X-Wujie-Team-Id"
const teamIDProperty = "wasm.teamId"
const personalOwnerProperty = "wasm.teamPersonalOwnerUid"

var canonicalTeamUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func checkTeamAdmission(ctx wrapper.HttpContext, config BalanceCheckConfig, teamUUID *string) types.Action {
	apiKey, _ := proxywasm.GetHttpRequestHeader(OriginalAPIKey)
	if apiKey == "" {
		return teamAdmissionError(http.StatusUnauthorized)
	}
	endpoint := config.teamEndpoint
	body, _ := json.Marshal(struct {
		TeamUUID *string `json:"teamUuid,omitempty"`
	}{TeamUUID: teamUUID})
	// The wrapper logs headers and bodies at info level. Keep this callout private.
	_, err := proxywasm.DispatchHttpCall(config.checkClient.ClusterName(), [][2]string{
		{":method", http.MethodPost},
		{":authority", endpoint.Hostname()},
		{":path", endpoint.RequestURI()},
		{"Authorization", "Bearer " + apiKey},
		{"Content-Type", "application/json"},
	}, body, nil, uint32(config.apiTimeout), func(_, bodySize, _ int) {
		status := teamAdmissionStatus()
		if status != http.StatusOK {
			teamAdmissionError(status)
			return
		}
		body, err := proxywasm.GetHttpCallResponseBody(0, bodySize)
		if err != nil {
			teamAdmissionError(http.StatusServiceUnavailable)
			return
		}
		var result struct {
			TeamID           string `json:"teamId"`
			TeamUUID         string `json:"teamUuid"`
			PersonalOwnerUID string `json:"personalOwnerUid"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			teamAdmissionError(http.StatusServiceUnavailable)
			return
		}
		if config.teamRouteMode != "" && (!canonicalTeamUUID.MatchString(result.TeamUUID) ||
			(teamUUID != nil && !strings.EqualFold(*teamUUID, result.TeamUUID))) {
			teamAdmissionError(http.StatusServiceUnavailable)
			return
		}
		teamID, err := strconv.ParseInt(result.TeamID, 10, 64)
		if err != nil || teamID <= 0 {
			teamAdmissionError(http.StatusServiceUnavailable)
			return
		}
		if result.PersonalOwnerUID != "" {
			if config.teamRouteMode != "" {
				teamAdmissionError(http.StatusServiceUnavailable)
				return
			}
			owner, err := strconv.ParseInt(result.PersonalOwnerUID, 10, 64)
			if err != nil || owner <= 0 || proxywasm.SetProperty(
				[]string{personalOwnerProperty}, []byte(result.PersonalOwnerUID)) != nil {
				teamAdmissionError(http.StatusServiceUnavailable)
				return
			}
		}
		if (config.teamSettlement == "image" || config.teamSettlement == "search") && proxywasm.SetProperty([]string{"wasm.teamBillingPerCallModel"}, []byte(config.teamImageBillingModel)) != nil {
			teamAdmissionError(http.StatusServiceUnavailable)
			return
		}
		if config.teamFallback == "source" && !snapshotModelFallback(config.teamSigningKey, result.TeamUUID, apiKey) {
			teamAdmissionError(http.StatusServiceUnavailable)
			return
		}
		if signedOperation(config.teamSettlement) && !forwardAdmittedTeam(config.teamSigningKey, result.TeamID, result.TeamUUID, apiKey, config.teamSettlement) {
			teamAdmissionError(http.StatusServiceUnavailable)
			return
		}
		if err := proxywasm.RemoveHttpRequestHeader(teamUUIDHeader); err != nil && err != types.ErrorStatusNotFound {
			teamAdmissionError(http.StatusServiceUnavailable)
			return
		}
		if config.teamSettlement == "forward" {
			if proxywasm.AddHttpRequestHeader(teamUUIDHeader, result.TeamUUID) != nil ||
				proxywasm.SetProperty([]string{"wasm.teamBillingSettlement"}, []byte("forward")) != nil {
				teamAdmissionError(http.StatusServiceUnavailable)
				return
			}
		}
		if err := proxywasm.SetProperty([]string{teamIDProperty}, []byte(strconv.FormatInt(teamID, 10))); err != nil {
			teamAdmissionError(http.StatusServiceUnavailable)
			return
		}
		ctx.SetContext(CtxKeyBalanceChecked, true)
		_ = proxywasm.ResumeHttpRequest()
	})
	if err != nil {
		return teamAdmissionError(http.StatusServiceUnavailable)
	}
	return types.ActionPause
}

const fallbackProofHeader = "X-Wujie-Model-Fallback-Proof"

func fallbackSignature(secret, timestamp, requestID, team, key, model string) []byte {
	digest := sha256.Sum256([]byte(key))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join([]string{"v1", timestamp, requestID, team, hex.EncodeToString(digest[:]), model, "model_fallback"}, "\n")))
	return mac.Sum(nil)
}

func validFallbackRequestID(id string) bool {
	return id != "" && id != "-" && len(id) <= 191 && strings.IndexFunc(id, func(r rune) bool { return r < 33 || r > 126 }) < 0
}

func validFallbackProof(secret, requestID, team, key, model, proof string) bool {
	if !validFallbackRequestID(requestID) || !validFallbackRequestID(model) || key == "" || !canonicalTeamUUID.MatchString(team) {
		return false
	}
	timestamp, encoded, ok := strings.Cut(proof, ".")
	issued, err := strconv.ParseInt(timestamp, 10, 64)
	now := time.Now().Unix()
	if !ok || err != nil || issued > now+60 || issued < now-86400 {
		return false
	}
	actual, err := hex.DecodeString(encoded)
	return err == nil && hmac.Equal(actual, fallbackSignature(secret, timestamp, requestID, team, key, model))
}

func snapshotModelFallback(secret, team, key string) bool {
	id, _ := proxywasm.GetHttpRequestHeader("request-id")
	if id == "" {
		id, _ = proxywasm.GetHttpRequestHeader("x-request-id")
	}
	model, _ := proxywasm.GetHttpRequestHeader("x-higress-llm-model")
	if !validFallbackRequestID(id) || key == "" || !validFallbackRequestID(model) {
		return false
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	proof := timestamp + "." + hex.EncodeToString(fallbackSignature(secret, timestamp, id, team, key, model))
	// RedirectPolicy reads these before recreateStream. They are never added to the
	// primary provider request. Envoy prefixes proxy-wasm stream state with "wasm.".
	for _, item := range [][2]string{
		{"wjFallbackTeam", team}, {"wjFallbackRequest", id}, {"wjFallbackKey", key}, {"wjFallbackModel", model},
		{"wjFallbackProof", proof},
	} {
		if proxywasm.SetProperty([]string{item[0]}, []byte(item[1])) != nil {
			return false
		}
	}
	return true
}

func signedOperation(settlement string) bool {
	return settlement == "task_create" || settlement == "task_create_sync" || settlement == "mirror_create"
}

// The upstream verifies the signature and uses request-id as its idempotency key.
func forwardAdmittedTeam(secret, teamID, teamUUID, apiKey, operation string) bool {
	requestID, _ := proxywasm.GetHttpRequestHeader("request-id")
	if requestID == "" {
		requestID, _ = proxywasm.GetHttpRequestHeader("x-request-id")
	}
	if requestID == "" || requestID == "-" || len(requestID) > 191 || strings.ContainsAny(requestID, "\r\n") {
		return false
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	hash := sha256.Sum256([]byte(apiKey))
	input := strings.Join([]string{"v1", timestamp, requestID, teamID, teamUUID, hex.EncodeToString(hash[:]), operation}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(input))
	if err := proxywasm.RemoveHttpRequestHeader("request-id"); err != nil && err != types.ErrorStatusNotFound {
		return false
	}
	for _, header := range [][2]string{
		{"request-id", requestID}, {admittedTeamHeader, teamID}, {admittedTeamUUIDHeader, teamUUID}, {timestampHeader, timestamp},
		{signatureHeader, hex.EncodeToString(mac.Sum(nil))},
	} {
		if err := proxywasm.AddHttpRequestHeader(header[0], header[1]); err != nil {
			return false
		}
	}
	return proxywasm.SetProperty([]string{"wasm.teamBillingSettlement"}, []byte(operation)) == nil
}

func teamAdmissionStatus() int {
	headers, _ := proxywasm.GetHttpCallResponseHeaders()
	for _, h := range headers {
		if h[0] == ":status" {
			status, _ := strconv.Atoi(h[1])
			return status
		}
	}
	return http.StatusServiceUnavailable
}

func teamAdmissionError(status int) types.Action {
	var code string
	message := http.StatusText(status)
	switch status {
	case http.StatusBadRequest:
		code = "invalid_team"
	case http.StatusUnauthorized:
		code = "authentication_required"
	case http.StatusForbidden:
		code = "team_access_denied"
		message = "当前账户无法在此团队发起任务"
		status = http.StatusUnprocessableEntity
	case http.StatusPaymentRequired:
		// These are request-level denials. API clients treat 402/403 as
		// failures of the shared upstream credentials.
		code = "team_credits_unavailable"
		message = "当前团队积分不足，请切换团队或补充积分"
		status = http.StatusUnprocessableEntity
	default:
		status = http.StatusServiceUnavailable
		code = "team_billing_unavailable"
		message = http.StatusText(status)
	}
	var response OpenAIError
	response.Error.Message = message
	response.Error.Type = code
	response.Error.Code = code
	body, _ := json.Marshal(response)
	_ = proxywasm.SendHttpResponse(uint32(status), [][2]string{{"Content-Type", "application/json"}}, body, -1)
	return types.HeaderStopAllIterationAndWatermark
}
