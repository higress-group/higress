package config

import (
	"strings"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	SafecheckRequestsKey   = "safecheck_requests"
	SafecheckRequestIDsKey = "safecheck_request_ids"
	SafecheckRequestIDKey  = "safecheck_request_id"

	GuardrailPhaseRequest  = "request"
	GuardrailPhaseResponse = "response"

	GuardrailModalityText  = "text"
	GuardrailModalityImage = "image"
	GuardrailModalityMCP   = "mcp"

	GuardrailResultPass  = "pass"
	GuardrailResultDeny  = "deny"
	GuardrailResultMask  = "mask"
	GuardrailResultError = "error"
)

type GuardrailSubmissionEvent struct {
	RequestID string `json:"requestId,omitempty"`
	Phase     string `json:"phase"`
	Modality  string `json:"modality"`
	Result    string `json:"result"`
}

// BeginGuardrailSubmissionEvent appends a placeholder event so append order matches
// the current serial submission order. The event is flushed only after completion.
func BeginGuardrailSubmissionEvent(ctx wrapper.HttpContext, phase, modality string) int {
	events := getGuardrailSubmissionEvents(ctx)
	events = append(events, GuardrailSubmissionEvent{
		Phase:    phase,
		Modality: modality,
	})
	setGuardrailSubmissionEvents(ctx, events)
	return len(events) - 1
}

func CompleteGuardrailSubmissionEvent(ctx wrapper.HttpContext, index int, responseBody []byte, result string) {
	CompleteGuardrailSubmissionEventWithRequestID(ctx, index, ExtractValidRequestID(responseBody), result)
}

func CompleteGuardrailSubmissionEventWithRequestID(ctx wrapper.HttpContext, index int, requestID, result string) {
	events := getGuardrailSubmissionEvents(ctx)
	if index < 0 || index >= len(events) {
		return
	}
	events[index].Result = result
	if requestID != "" {
		events[index].RequestID = requestID
	}
	setGuardrailSubmissionEvents(ctx, events)
}

// WriteGuardrailLog writes current guardrail-related user attributes to the AI log.
// Call after submission events are updated; Complete* does not flush the log.
func WriteGuardrailLog(ctx wrapper.HttpContext) {
	ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
}

func ExtractValidRequestID(responseBody []byte) string {
	if len(responseBody) == 0 {
		return ""
	}
	requestID := gjson.GetBytes(responseBody, "RequestId")
	if !requestID.Exists() || requestID.Type != gjson.String {
		return ""
	}
	trimmed := strings.TrimSpace(requestID.String())
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func getGuardrailSubmissionEvents(ctx wrapper.HttpContext) []GuardrailSubmissionEvent {
	events, ok := ctx.GetUserAttribute(SafecheckRequestsKey).([]GuardrailSubmissionEvent)
	if !ok || events == nil {
		return []GuardrailSubmissionEvent{}
	}
	return events
}

func setGuardrailSubmissionEvents(ctx wrapper.HttpContext, events []GuardrailSubmissionEvent) {
	ctx.SetUserAttribute(SafecheckRequestsKey, events)

	requestIDs := make([]string, 0, len(events))
	for _, event := range events {
		if event.RequestID != "" {
			requestIDs = append(requestIDs, event.RequestID)
		}
	}
	ctx.SetUserAttribute(SafecheckRequestIDsKey, requestIDs)
	if len(requestIDs) > 0 {
		ctx.SetUserAttribute(SafecheckRequestIDKey, requestIDs[len(requestIDs)-1])
	}
}
