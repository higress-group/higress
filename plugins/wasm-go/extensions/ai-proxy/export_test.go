package main

import "github.com/higress-group/wasm-go/pkg/wrapper"

// NeedsClaudeResponseConversionForTest exposes needsClaudeResponseConversion for unit tests.
func NeedsClaudeResponseConversionForTest(ctx wrapper.HttpContext) bool {
	return needsClaudeResponseConversion(ctx)
}
