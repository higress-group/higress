// Copyright (c) 2023 Alibaba Group Holding Ltd.
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

package handler

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
)

func TestActionForVerifiedConsumerUsesConfigRuleSetSnapshot(t *testing.T) {
	action, resume := actionForVerifiedConsumer(config.JWTAuthConfig{
		RuleSet: true,
		Allow:   []string{"allowed-consumer"},
	}, "other-consumer", &testLogger{T: t})

	if resume {
		t.Fatalf("expected route-level allow list to deny non-allowed consumer")
	}
	if action == nil {
		t.Fatalf("expected a deny action for non-allowed consumer")
	}
}
