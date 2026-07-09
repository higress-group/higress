// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ollama routes to the configured server: CreateProvider derives the upstream host:port that
// TransformRequestHeaders sets as the request authority. The mock is host-agnostic so the e2e
// can't check this; lock the derivation here.
func TestOllamaProvider_CreateProviderDerivesUpstreamHostPort(t *testing.T) {
	p, err := (&ollamaProviderInitializer{}).CreateProvider(ProviderConfig{
		ollamaServerHost: "ollama-backend.internal",
		ollamaServerPort: 11434,
	})
	require.NoError(t, err)
	assert.Equal(t, "ollama-backend.internal:11434", p.(*ollamaProvider).serviceDomain)
}

func TestOllamaProvider_ValidateConfig(t *testing.T) {
	init := &ollamaProviderInitializer{}
	require.Error(t, init.ValidateConfig(&ProviderConfig{ollamaServerPort: 11434})) // missing host
	require.Error(t, init.ValidateConfig(&ProviderConfig{ollamaServerHost: "x"}))   // missing port
	require.NoError(t, init.ValidateConfig(&ProviderConfig{ollamaServerHost: "x", ollamaServerPort: 11434}))
}
