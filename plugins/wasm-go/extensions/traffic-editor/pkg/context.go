package pkg

import (
	"maps"
	"net/url"
	"strings"

	"github.com/higress-group/wasm-go/pkg/log"
)

type Stage int

const (
	StageInvalid Stage = iota
	StageRequestHeaders
	StageRequestBody
	StageResponseHeaders
	StageResponseBody

	pathHeader = ":path"
)

var (
	OrderedStages = []Stage{
		StageRequestHeaders,
		StageRequestBody,
		StageResponseHeaders,
		StageResponseBody,
	}
	Stage2String = map[Stage]string{
		StageRequestHeaders:  "request_headers",
		StageRequestBody:     "request_body",
		StageResponseHeaders: "response_headers",
		StageResponseBody:    "response_body",
	}
)

type EditorContext interface {
	GetEffectiveCommandSet() *CommandSet
	SetEffectiveCommandSet(cmdSet *CommandSet)
	GetCommandExecutors() []Executor
	SetCommandExecutors(executors []Executor)
	GetCurrentStage() Stage
	SetCurrentStage(stage Stage)

	GetRequestPath() string
	SetRequestPath(path string)
	GetRequestHeader(key string) []string
	GetRequestHeaders() map[string][]string
	SetRequestHeaders(map[string][]string)
	GetRequestQuery(key string) []string
	GetRequestQueries() map[string][]string
	SetRequestQueries(map[string][]string)
	GetResponseHeader(key string) []string
	GetResponseHeaders() map[string][]string
	SetResponseHeaders(map[string][]string)

	GetRefValue(ref *Ref) string
	GetRefValues(ref *Ref) []string
	SetRefValue(ref *Ref, value string)
	SetRefValues(ref *Ref, values []string)
	DeleteRefValues(ref *Ref)

	IsRequestHeadersDirty() bool
	IsResponseHeadersDirty() bool
	ResetDirtyFlags()
}

func NewEditorContext() EditorContext {
	return &editorContext{}
}

type editorContext struct {
	effectiveCommandSet *CommandSet
	commandExecutors    []Executor

	currentStage Stage

	requestPath     string
	requestHeaders  map[string][]string
	requestQueries  map[string][]string
	responseHeaders map[string][]string

	requestHeadersDirty  bool
	responseHeadersDirty bool
}

func (ctx *editorContext) GetEffectiveCommandSet() *CommandSet {
	return ctx.effectiveCommandSet
}

func (ctx *editorContext) SetEffectiveCommandSet(cmdSet *CommandSet) {
	ctx.effectiveCommandSet = cmdSet
}

func (ctx *editorContext) GetCommandExecutors() []Executor {
	return ctx.commandExecutors
}

func (ctx *editorContext) SetCommandExecutors(executors []Executor) {
	ctx.commandExecutors = executors
}

func (ctx *editorContext) GetCurrentStage() Stage {
	return ctx.currentStage
}

func (ctx *editorContext) SetCurrentStage(stage Stage) {
	ctx.currentStage = stage
}

func (ctx *editorContext) GetRequestPath() string {
	return ctx.requestPath
}

func (ctx *editorContext) SetRequestPath(path string) {
	ctx.requestPath = path
	ctx.savePathToHeader()
}

func (ctx *editorContext) GetRequestHeader(key string) []string {
	if ctx.requestHeaders == nil {
		return nil
	}
	return ctx.requestHeaders[strings.ToLower(key)]
}

func (ctx *editorContext) GetRequestHeaders() map[string][]string {
	return maps.Clone(ctx.requestHeaders)
}

func (ctx *editorContext) SetRequestHeaders(headers map[string][]string) {
	ctx.requestHeaders = headers
	ctx.loadPathFromHeader()
	ctx.requestHeadersDirty = true
}

func (ctx *editorContext) GetRequestQuery(key string) []string {
	if ctx.requestQueries == nil {
		return nil
	}
	return ctx.requestQueries[key]
}

func (ctx *editorContext) GetRequestQueries() map[string][]string {
	return maps.Clone(ctx.requestQueries)
}

func (ctx *editorContext) SetRequestQueries(queries map[string][]string) {
	ctx.requestQueries = queries
	ctx.savePathToHeader()
}

func (ctx *editorContext) GetResponseHeader(key string) []string {
	if ctx.responseHeaders == nil {
		return nil
	}
	return ctx.responseHeaders[strings.ToLower(key)]
}

func (ctx *editorContext) GetResponseHeaders() map[string][]string {
	return maps.Clone(ctx.responseHeaders)
}

func (ctx *editorContext) SetResponseHeaders(headers map[string][]string) {
	ctx.responseHeaders = headers
	ctx.responseHeadersDirty = true
}

func (ctx *editorContext) GetRefValue(ref *Ref) string {
	values := ctx.GetRefValues(ref)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (ctx *editorContext) GetRefValues(ref *Ref) []string {
	if ref == nil {
		return nil
	}
	switch ref.Type {
	case RefTypeRequestHeader:
		return ctx.GetRequestHeader(strings.ToLower(ref.Name))
	case RefTypeRequestQuery:
		return ctx.GetRequestQuery(ref.Name)
	case RefTypeResponseHeader:
		return ctx.GetResponseHeader(strings.ToLower(ref.Name))
	default:
		return nil
	}
}

func (ctx *editorContext) SetRefValue(ref *Ref, value string) {
	if ref == nil {
		return
	}
	ctx.SetRefValues(ref, []string{value})
}

func (ctx *editorContext) SetRefValues(ref *Ref, values []string) {
	if ref == nil {
		return
	}
	switch ref.Type {
	case RefTypeRequestHeader:
		if ctx.requestHeaders == nil {
			ctx.requestHeaders = make(map[string][]string)
		}
		loweredRefName := strings.ToLower(ref.Name)
		ctx.requestHeaders[loweredRefName] = values
		ctx.requestHeadersDirty = true
		if loweredRefName == pathHeader {
			ctx.loadPathFromHeader()
		}
		break
	case RefTypeRequestQuery:
		if ctx.requestQueries == nil {
			ctx.requestQueries = make(map[string][]string)
		}
		ctx.requestQueries[ref.Name] = values
		ctx.savePathToHeader()
		break
	case RefTypeResponseHeader:
		if ctx.responseHeaders == nil {
			ctx.responseHeaders = make(map[string][]string)
		}
		ctx.responseHeaders[strings.ToLower(ref.Name)] = values
		ctx.responseHeadersDirty = true
		break
	}
}

func (ctx *editorContext) DeleteRefValues(ref *Ref) {
	if ref == nil {
		return
	}
	switch ref.Type {
	case RefTypeRequestHeader:
		delete(ctx.requestHeaders, strings.ToLower(ref.Name))
		ctx.requestHeadersDirty = true
		break
	case RefTypeRequestQuery:
		delete(ctx.requestQueries, ref.Name)
		ctx.savePathToHeader()
		break
	case RefTypeResponseHeader:
		delete(ctx.responseHeaders, strings.ToLower(ref.Name))
		ctx.responseHeadersDirty = true
		break
	}
}

func (ctx *editorContext) IsRequestHeadersDirty() bool {
	return ctx.requestHeadersDirty
}

func (ctx *editorContext) IsResponseHeadersDirty() bool {
	return ctx.responseHeadersDirty
}

func (ctx *editorContext) ResetDirtyFlags() {
	ctx.requestHeadersDirty = false
	ctx.responseHeadersDirty = false
}

func (ctx *editorContext) savePathToHeader() {
	u, err := url.Parse(ctx.requestPath)
	if err != nil {
		log.Errorf("failed to build the new path with query strings: %v", err)
		return
	}

	query := url.Values{}
	for k, vs := range ctx.requestQueries {
		for _, v := range vs {
			query.Add(k, v)
		}
	}
	u.RawQuery = query.Encode()
	ctx.SetRefValue(&Ref{Type: RefTypeRequestHeader, Name: pathHeader}, u.String())
}

func (ctx *editorContext) loadPathFromHeader() {
	paths := ctx.GetRequestHeader(pathHeader)

	if len(paths) == 0 || paths[0] == "" {
		log.Warn("the request has an empty path")
		ctx.requestPath = ""
		ctx.requestQueries = make(map[string][]string)
		return
	}

	path := paths[0]
	queries := make(map[string][]string)

	u, err := url.Parse(path)
	if err != nil {
		log.Warnf("unable to parse the request path: %s", path)
		ctx.requestPath = ""
		ctx.requestQueries = make(map[string][]string)
		return
	}

	// Keep the encoded path so query edits do not turn escaped delimiters
	// into path separators, query strings, or fragments when reparsing it.
	ctx.requestPath = u.EscapedPath()
	for k, vs := range u.Query() {
		queries[k] = vs
	}
	ctx.requestQueries = queries
}
