package rag

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/mark3labs/mcp-go/mcp"
)

// HandleCreateChunkFromText handles the creation of knowledge chunks from text input
func HandleCreateChunkFromText(ragClient *RAGClient) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := request.Params.Arguments
		text, ok1 := arguments["text"].(string)
		title, ok2 := arguments["title"].(string)
		if !ok1 {
			return nil, fmt.Errorf("invalid text argument")
		}
		if !ok2 {
			return nil, fmt.Errorf("invalid title argument")
		}
		// Create knowledge chunks
		docs, err := ragClient.CreateChunkFromText(text, title)
		if err != nil {
			return nil, fmt.Errorf("create chunk failed, err: %w", err)
		}

		result := map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("chunks created from text, title: %s", title),
			"data":    docs,
		}

		return buildCallToolResult(result)
	}
}

// HandleListChunks handles the listing of knowledge chunks
func HandleListChunks(ragClient *RAGClient) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		chunks, err := ragClient.ListChunks()
		if err != nil {
			return nil, fmt.Errorf("list chunks failed, err: %w", err)
		}
		return buildCallToolResult(chunks)
	}
}

// HandleDeleteChunk handles the deletion of a knowledge chunk
func HandleDeleteChunk(ragClient *RAGClient) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := request.Params.Arguments
		id, ok := arguments["id"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid id argument")
		}

		if err := ragClient.DeleteChunk(id); err != nil {
			return nil, fmt.Errorf("delete chunk failed, err: %w", err)
		}

		result := map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("chunk deleted, id: %s", id),
		}

		return buildCallToolResult(result)
	}
}

// HandleCreateSession handles the creation of a chat session
func HandleCreateSession(ragClient *RAGClient) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// TODO: Implement chat session creation logic
		result := map[string]interface{}{
			"session_id": "session-1",
			"created_at": "2024-01-01T00:00:00Z",
		}

		return buildCallToolResult(result)
	}
}

// HandleGetSession handles retrieving session details
func HandleGetSession(ragClient *RAGClient) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := request.Params.Arguments
		sessionId, ok := arguments["session_id"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid session_id argument")
		}

		// TODO: Implement session details retrieval logic
		result := map[string]interface{}{
			"session_id": sessionId,
			"messages":   []interface{}{},
		}

		return buildCallToolResult(result)
	}
}

// HandleListSessions handles listing all sessions
func HandleListSessions(ragClient *RAGClient) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// TODO: Implement session listing logic
		result := map[string]interface{}{
			"sessions": []interface{}{},
			"total":    0,
		}

		return buildCallToolResult(result)
	}
}

// HandleDeleteSession handles the deletion of a session
func HandleDeleteSession(ragClient *RAGClient) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := request.Params.Arguments
		sessionId, ok := arguments["session_id"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid session_id argument")
		}

		// TODO: Implement session deletion logic
		result := map[string]interface{}{
			"success":    true,
			"message":    "Session deleted",
			"session_id": sessionId,
		}

		return buildCallToolResult(result)
	}
}

// HandleSearch handles semantic search functionality
func HandleSearch(ragClient *RAGClient) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := request.Params.Arguments
		query, ok := arguments["query"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid query argument")
		}
		topK, ok := arguments["topk"].(float64)
		if !ok {
			topK = float64(ragClient.config.RAG.TopK)
		}

		threshold, ok := arguments["threshold"].(float64)
		if !ok {
			threshold = ragClient.config.RAG.Threshold
		}

		searchResult, err := ragClient.SearchChunks(query, int(topK), threshold)
		if err != nil {
			return nil, fmt.Errorf("search chunks failed, err: %w", err)
		}
		return buildCallToolResult(searchResult)
	}
}

// HandleChat handles chat interactions using LLM
func HandleChat(ragClient *RAGClient) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := request.Params.Arguments
		query, ok := arguments["query"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid query argument")
		}
		// check llm provider
		if ragClient.llmProvider == nil {
			return nil, fmt.Errorf("llm provider is empty, please check the llm configuration")
		}
		// Generate response using RAGClient's LLM
		reply, err := ragClient.Chat(query)
		if err != nil {
			return nil, fmt.Errorf("chat failed, err: %w", err)
		}

		return buildCallToolResult(reply)
	}
}

// buildCallToolResult builds the call tool result
func buildCallToolResult(results any) (*mcp.CallToolResult, error) {
	jsonData, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal results: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

// Schema functions

// GetCreateChunkFromTextSchema returns the schema for create chunk from text tool
func GetCreateChunkFromTextSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {
				"type": "string",
				"description": "The text content to create chunks from"
			},
			"title": {
				"type": "string",
				"description": "The title of text content"
			}
		},
		"required": ["text", "title"]
	}`)
}

// GetListKnowledgeSchema returns the schema for list knowledge tool
func GetListKnowledgeSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

// GetGetKnowledgeSchema returns the schema for get knowledge tool
func GetGetKnowledgeSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"description": "The knowledge ID"
			}
		},
		"required": ["id"]
	}`)
}

// GetDeleteKnowledgeSchema returns the schema for delete knowledge tool
func GetDeleteKnowledgeSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"description": "The knowledge ID to delete"
			}
		},
		"required": ["id"]
	}`)
}

// GetListChunksSchema returns the schema for list chunks tool
func GetListChunksSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

// GetDeleteChunkSchema returns the schema for delete chunk tool
func GetDeleteChunkSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"description": "The chunk ID to delete"
			}
		},
		"required": ["id"]
	}`)
}

// GetCreateSessionSchema returns the schema for create session tool
func GetCreateSessionSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

// GetGetSessionSchema returns the schema for get session tool
func GetGetSessionSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {
				"type": "string",
				"description": "The session ID"
			}
		},
		"required": ["session_id"]
	}`)
}

// GetListSessionsSchema returns the schema for list sessions tool
func GetListSessionsSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

// GetDeleteSessionSchema returns the schema for delete session tool
func GetDeleteSessionSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {
				"type": "string",
				"description": "The session ID to delete"
			}
		},
		"required": ["session_id"]
	}`)
}

// GetSearchSchema returns the schema for search tool
func GetSearchSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query"
			},
			"topk": {
                "type": "integer",
                "description": "The number of top results to return (optional, default 10)"
            },
            "threshold": {
                "type": "number",
                "description": "The relevance score threshold for filtering results (optional, default 0.5)"
            }
		},
		"required": ["query"]
	}`)
}

// GetChatSchema returns the schema for chat tool
func GetChatSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "User query"
			}
		},
		"required": ["query"]
	}`)
}
