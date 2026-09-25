package aiclientcontext

import (
	"encoding/json"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
)

type CodexClient struct{}

type codex_metadataType struct {
	XCodexInstallationId string `json:"x-codex-installation-id,omitempty"`
	TurnId               string `json:"turn_id,omitempty"`
	XCodexTurnMetadata   string `json:"x-codex-turn-metadata,omitempty"`
	ThreadId             string `json:"thread_id,omitempty"`
	SessionId            string `json:"session_id,omitempty"`
	XCodexWindowId       string `json:"x-codex-window-id,omitempty"`
}

type codexMetadata struct {
	ClientMetadata codex_metadataType `json:"client_metadata,omitempty"`
}

func (c *CodexClient) Name() string {
	return "codex"
}

func (c *CodexClient) Match() bool {
	userAgent, err := proxywasm.GetHttpRequestHeader("User-Agent")
	if err != nil {
		originator, err := proxywasm.GetHttpRequestHeader("Originator")
		if err != nil {
			return false
		}
		strings.Contains(strings.ToLower(originator), "codex")
	}

	return strings.Contains(strings.ToLower(userAgent), "codex")
}

func (c *CodexClient) ExtractContext(body []byte) (*Context, bool) {
	metadata, _ := proxywasm.GetHttpRequestHeader("X-Codex-Turn-Metadata")
	var codexMetadataType codex_metadataType
	if err := json.Unmarshal([]byte(metadata), &codexMetadataType); err != nil || codexMetadataType.TurnId == "" {
		log.Warnf("Header X-Codex-Turn-Metadata parse failed, %s", err.Error())
		var codexBodyMetadata codexMetadata
		if err := json.Unmarshal(body, &codexBodyMetadata); err != nil || codexBodyMetadata.ClientMetadata.TurnId == "" {
			return &Context{}, false
		}
		return &Context{
			ClientType: "Codex",
			LoopID:     codexBodyMetadata.ClientMetadata.TurnId,
			LoopIDKey:  PrefixKey + codexBodyMetadata.ClientMetadata.TurnId,
		}, true
	}

	return &Context{
		ClientType: "Codex",
		LoopID:     codexMetadataType.TurnId,
		LoopIDKey:  PrefixKey + codexMetadataType.TurnId,
	}, true
}
