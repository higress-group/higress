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

func TestDeeplProvider_TransformResponseBody(t *testing.T) {
	provider := &deeplProvider{}
	ctx := newMockMultipartHttpContext()

	body := `{"translations":[{"detected_source_language":"EN","text":"你好"}]}`
	out, err := provider.TransformResponseBody(ctx, ApiNameChatCompletion, []byte(body))
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `"content":"你好"`)
	assert.Contains(t, s, `"name":"EN"`) // detected_source_language -> message.name
	assert.Contains(t, s, `"object":"chat.completion"`)
}

func TestDeeplProvider_OriginalProtocolPassthrough(t *testing.T) {
	provider := &deeplProvider{config: ProviderConfig{protocol: protocolOriginal}}
	ctx := newMockMultipartHttpContext()

	out, err := provider.TransformResponseBody(ctx, ApiNameChatCompletion, []byte(`{"translations":[{"text":"x"}]}`))
	require.NoError(t, err)
	assert.Equal(t, `{"translations":[{"text":"x"}]}`, string(out))
}
