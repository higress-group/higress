package utils

import (
	"errors"
	"fmt"

	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// counterReducer is the part of wrapper.RedisClient that the selection policies
// need to undo an increment their Lua script has already committed.
type counterReducer interface {
	HIncrBy(key, field string, delta int, callback wrapper.RedisResponseCallback) error
}

// CompensateCounter undoes one increment committed by a selection script.
//
// The script increments unconditionally, so every path that abandons the host
// it selected has to put that increment back. The compensation is best effort:
// the error is returned only so the caller can log it, because the request has
// already been resumed and must not change its response or its action.
func CompensateCounter(client counterReducer, key, host string) error {
	if host == "" {
		return errors.New("the incremented host is unknown")
	}
	if err := client.HIncrBy(key, host, -1, nil); err != nil {
		return fmt.Errorf("HIncrBy %s %s -1: %w", key, host, err)
	}
	return nil
}
