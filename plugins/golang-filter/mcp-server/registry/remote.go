package registry

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	HTTP_URL_TEMPLATE     = "%s://%s:%d%s"
	FIX_QUERY_TOKEN_KEY   = "key"
	FIX_QUERY_TOKEN_VALUE = "value"
	PROTOCOL_HTTP         = "http"
	PROTOCOL_HTTPS        = "https"
	DEFAULT_HTTP_METHOD   = "GET"
	DEFAULT_HTTP_PATH     = "/"
)

func getHttpCredentialHandle(name string) (func(*CredentialInfo, *HttpRemoteCallHandle) error, error) {
	if name == "fixed-query-token" {
		return FixedQueryToken, nil
	}

	return nil, fmt.Errorf("Unknown credential type")
}

type CommonRemoteCallHandle struct {
	Instance *Instance
}

type HttpRemoteCallHandle struct {
	CommonRemoteCallHandle
	Protocol string
	Headers  http.Header
	Body     *string
	Query    map[string]string
	Path     string
	Method   string
}

// http credentials handles
func FixedQueryToken(cred *CredentialInfo, h *HttpRemoteCallHandle) error {
	key, ok := cred.Credentials[FIX_QUERY_TOKEN_KEY]
	if !ok {
		return fmt.Errorf("missing %q in fixed-query-token credentials", FIX_QUERY_TOKEN_KEY)
	}
	value, ok := cred.Credentials[FIX_QUERY_TOKEN_VALUE]
	if !ok {
		return fmt.Errorf("missing %q in fixed-query-token credentials", FIX_QUERY_TOKEN_VALUE)
	}
	keyStr, ok := key.(string)
	if !ok {
		return fmt.Errorf("%q in fixed-query-token credentials is not a string", FIX_QUERY_TOKEN_KEY)
	}
	valueStr, ok := value.(string)
	if !ok {
		return fmt.Errorf("%q in fixed-query-token credentials is not a string", FIX_QUERY_TOKEN_VALUE)
	}
	h.Query[keyStr] = valueStr
	return nil
}

func newHttpRemoteCallHandle(ctx *RpcContext) (*HttpRemoteCallHandle, error) {
	instance, err := selectOneInstance(ctx)
	if err != nil {
		return nil, err
	}
	method, ok := ctx.ToolMeta.InvokeContext["method"]
	if !ok {
		method = DEFAULT_HTTP_METHOD
	}

	path, ok := ctx.ToolMeta.InvokeContext["path"]
	if !ok {
		path = DEFAULT_HTTP_PATH
	}

	return &HttpRemoteCallHandle{
		CommonRemoteCallHandle: CommonRemoteCallHandle{
			Instance: instance,
		},
		Protocol: ctx.Protocol,
		Headers:  http.Header{},
		Body:     nil,
		Query:    map[string]string{},
		Path:     path,
		Method:   method,
	}, nil
}

// http remote handle implementation
func (h *HttpRemoteCallHandle) HandleToolCall(ctx *RpcContext, parameters map[string]any) (*mcp.CallToolResult, error) {
	if ctx.Credential != nil {
		credentialHandle, err := getHttpCredentialHandle(ctx.Credential.CredentialType)
		if err != nil {
			return nil, err
		}
		if err := credentialHandle(ctx.Credential, h); err != nil {
			return nil, err
		}
	}

	err := h.handleParamMapping(&ctx.ToolMeta.ParametersMapping, parameters)
	if err != nil {
		return nil, err
	}

	response, err := h.doHttpCall()
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	responseType := "text"
	if respType, ok := ctx.ToolMeta.InvokeContext["responseType"]; ok {
		responseType = respType
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: responseType,
				Text: string(body),
			},
		},
	}, nil
}

func (h *HttpRemoteCallHandle) handleParamMapping(mapInfo *map[string]ParameterMapInfo, params map[string]any) error {
	paramMapInfo := *mapInfo
	for param, value := range params {
		if info, ok := paramMapInfo[param]; ok {
			if info.Position == "Query" {
				h.Query[info.BackendName] = fmt.Sprintf("%v", value)
			} else if info.Position == "Header" {
				h.Headers[info.BackendName] = []string{fmt.Sprintf("%v", value)}
			} else {
				return fmt.Errorf("Unsupport position for args %s, pos is %s", param, info.Position)
			}
		} else {
			h.Query[param] = fmt.Sprintf("%v", value)
		}
	}
	return nil
}

func (h *HttpRemoteCallHandle) doHttpCall() (*http.Response, error) {
	pathPrefix := fmt.Sprintf(HTTP_URL_TEMPLATE, h.Protocol, h.Instance.Host, h.Instance.Port, h.Path)
	queryString := ""
	queryGroup := []string{}
	for queryKey, queryValue := range h.Query {
		queryGroup = append(queryGroup, url.QueryEscape(queryKey)+"="+url.QueryEscape(queryValue))
	}

	if len(queryGroup) > 0 {
		queryString = "?" + strings.Join(queryGroup, "&")
	}
	fullUrl, err := url.Parse(pathPrefix + queryString)
	if err != nil {
		return nil, fmt.Errorf("Parse url error , url is %s", pathPrefix+queryString)
	}
	request := http.Request{
		URL:    fullUrl,
		Method: h.Method,
		Header: h.Headers,
	}

	if h.Body != nil {
		request.Body = io.NopCloser(strings.NewReader(*h.Body))
	}

	return http.DefaultClient.Do(&request)
}

func selectOneInstance(ctx *RpcContext) (*Instance, error) {
	instanceId := 0
	if ctx.Instances == nil || len(*ctx.Instances) == 0 {
		return nil, fmt.Errorf("No instance")
	}

	instances := *ctx.Instances
	if len(instances) > 1 {
		instanceId = rand.Intn(len(instances) - 1)
	}
	select_instance := instances[instanceId]
	return &select_instance, nil
}

func getRemoteCallHandle(ctx *RpcContext) (RemoteCallHandle, error) {
	if ctx.Protocol == PROTOCOL_HTTP || ctx.Protocol == PROTOCOL_HTTPS {
		return newHttpRemoteCallHandle(ctx)
	} else {
		return nil, nil
	}
}

// common remote call process
func CommonRemoteCall(reg McpServerRegistry, toolName string, parameters map[string]any) (*mcp.CallToolResult, error) {
	ctx, ok := reg.GetToolRpcContext(toolName)
	if !ok {
		return nil, fmt.Errorf("Unknown tool %s", toolName)
	}

	remoteHandle, err := getRemoteCallHandle(ctx)
	if remoteHandle == nil {
		return nil, fmt.Errorf("Unknown backend protocol %s", ctx.Protocol)
	}

	if err != nil {
		return nil, fmt.Errorf("Call backend server error: %w", err)
	}

	return remoteHandle.HandleToolCall(ctx, parameters)
}

func HandleRegistryToolsCall(reg McpServerRegistry) common.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := request.Params.Arguments
		return CommonRemoteCall(reg, request.Params.Name, arguments)
	}
}
