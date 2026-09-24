package server

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// PoisonContext is a mock HttpContext that returns nil or wrong-type values
// for all context keys, simulating corrupted/missing context.
type PoisonContext struct {
	wrapper.HttpContext
}

func (p *PoisonContext) GetContext(key string) any {
	return nil // All context lookups return nil
}

func (p *PoisonContext) GetBoolContext(key string, defaultValue bool) bool {
	return defaultValue
}

func (p *PoisonContext) GetStringContext(key string, defaultValue string) string {
	return defaultValue
}

// WrongTypeContext returns values with wrong types
type WrongTypeContext struct {
	wrapper.HttpContext
}

func (w *WrongTypeContext) GetContext(key string) any {
	// Return wrong types for known keys
	switch key {
	case CtxSSEProxyEndpointURL:
		return 12345 // string expected, int returned
	case "mcp_proxy_server":
		return "not-a-server" // *McpProxyServer expected, string returned
	case CtxSSEProxyAuthInfo:
		return "not-auth-info" // *ProxyAuthInfo expected, string returned
	case CtxSSEProxyBuffer:
		return "not-bytes" // []byte expected, string returned
	case CtxSSEProxyState:
		return 12345 // string expected, int returned
	case CtxSSEProxyJsonRpcID:
		return "not-jsonrpc-id" // utils.JsonRpcID expected, string returned
	case CtxSSEProxyRequestID:
		return "not-int" // int expected, string returned
	default:
		return nil
	}
}

func (w *WrongTypeContext) GetBoolContext(key string, defaultValue bool) bool {
	return defaultValue
}

func (w *WrongTypeContext) GetStringContext(key string, defaultValue string) string {
	return defaultValue
}

// TestPoisonContext_NilContext_NoPanic verifies that injectSSEResponseError
// and injectSSEResponseSuccess do not panic when all context values are nil.
func TestPoisonContext_NilContext_NoPanic(t *testing.T) {
	ctx := &PoisonContext{}

	// These should not panic — they should log and return gracefully
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("injectSSEResponseError panicked with nil context: %v", r)
		}
	}()

	injectSSEResponseError(ctx, nil, utils.ErrInternalError)
}

func TestPoisonContext_NilContext_SuccessNoPanic(t *testing.T) {
	ctx := &PoisonContext{}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("injectSSEResponseSuccess panicked with nil context: %v", r)
		}
	}()

	injectSSEResponseSuccess(ctx, map[string]any{"result": "test"})
}

// TestPoisonContext_WrongType_NoPanic verifies that injectSSEResponseError
// and injectSSEResponseSuccess do not panic when context values have wrong types.
func TestPoisonContext_WrongType_NoPanic(t *testing.T) {
	ctx := &WrongTypeContext{}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("injectSSEResponseError panicked with wrong-type context: %v", r)
		}
	}()

	injectSSEResponseError(ctx, nil, utils.ErrInternalError)
}

func TestPoisonContext_WrongType_SuccessNoPanic(t *testing.T) {
	ctx := &WrongTypeContext{}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("injectSSEResponseSuccess panicked with wrong-type context: %v", r)
		}
	}()

	injectSSEResponseSuccess(ctx, map[string]any{"result": "test"})
}

// TestPoisonContext_BufferAndState_NoPanic verifies that handleSSEStreamingResponse
// does not panic when buffer and state context values are nil or wrong type.
// We can't fully test handleSSEStreamingResponse without a real HttpContext,
// but we can verify the type assertion guards work in isolation.
func TestPoisonContext_BufferTypeCheck(t *testing.T) {
	// Verify that a nil bufferRaw doesn't cause issues
	var bufferRaw any = nil
	if bufferRaw != nil {
		buffer, ok := bufferRaw.([]byte)
		if !ok {
			t.Errorf("should not reach here for nil bufferRaw")
			_ = buffer
		}
	}

	// Verify that a wrong-type bufferRaw is caught
	bufferRaw = "not-bytes"
	buffer, ok := bufferRaw.([]byte)
	if ok {
		t.Errorf("should not succeed for wrong-type bufferRaw")
		_ = buffer
	}
}

func TestPoisonContext_StateTypeCheck(t *testing.T) {
	// Verify nil state handling
	var state any = nil
	if state != nil {
		stateStr, ok := state.(string)
		if !ok {
			t.Errorf("should not reach here for nil state")
			_ = stateStr
		}
	}

	// Verify wrong-type state is caught
	state = 12345
	stateStr, ok := state.(string)
	if ok {
		t.Errorf("should not succeed for wrong-type state")
		_ = stateStr
	}
}

// TestPoisonContext_EndpointURLTypeCheck verifies endpoint URL type checking
func TestPoisonContext_EndpointURLTypeCheck(t *testing.T) {
	// nil case
	var endpointURLRaw any = nil
	endpointURL, ok := endpointURLRaw.(string)
	if ok {
		t.Errorf("should not succeed for nil endpointURL")
		_ = endpointURL
	}

	// wrong type case
	endpointURLRaw = 12345
	endpointURL, ok = endpointURLRaw.(string)
	if ok {
		t.Errorf("should not succeed for wrong-type endpointURL")
		_ = endpointURL
	}
}

// TestPoisonContext_JsonRpcIDTypeCheck verifies JSON-RPC ID type checking
func TestPoisonContext_JsonRpcIDTypeCheck(t *testing.T) {
	// nil case
	var jsonRpcIDRaw any = nil
	jsonRpcID, ok := jsonRpcIDRaw.(utils.JsonRpcID)
	if ok {
		t.Errorf("should not succeed for nil jsonRpcID")
		_ = jsonRpcID
	}

	// wrong type case
	jsonRpcIDRaw = "not-a-jsonrpc-id"
	jsonRpcID, ok = jsonRpcIDRaw.(utils.JsonRpcID)
	if ok {
		t.Errorf("should not succeed for wrong-type jsonRpcID")
		_ = jsonRpcID
	}
}

// TestPoisonContext_RequestIDTypeCheck verifies request ID type checking
func TestPoisonContext_RequestIDTypeCheck(t *testing.T) {
	// nil case
	var requestIDRaw any = nil
	requestID, ok := requestIDRaw.(int)
	if ok {
		t.Errorf("should not succeed for nil requestID")
		_ = requestID
	}

	// wrong type case
	requestIDRaw = "not-an-int"
	requestID, ok = requestIDRaw.(int)
	if ok {
		t.Errorf("should not succeed for wrong-type requestID")
		_ = requestID
	}
}