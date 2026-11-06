package pkg

import (
	"reflect"
	"testing"
)

func newTestRef(t, name string) *Ref {
	return &Ref{Type: t, Name: name}
}

func TestEditorContext_CommandSetAndExecutors(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	cmdSet := &CommandSet{}
	ctx.SetEffectiveCommandSet(cmdSet)
	if ctx.GetEffectiveCommandSet() != cmdSet {
		t.Errorf("EffectiveCommandSet not set/get correctly")
	}

	executors := []Executor{nil, nil}
	ctx.SetCommandExecutors(executors)
	if !reflect.DeepEqual(ctx.GetCommandExecutors(), executors) {
		t.Errorf("CommandExecutors not set/get correctly")
	}
}

func TestEditorContext_Stage(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	ctx.SetCurrentStage(StageRequestHeaders)
	if ctx.GetCurrentStage() != StageRequestHeaders {
		t.Errorf("CurrentStage not set/get correctly")
	}
}

func TestEditorContext_RequestPath(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	ctx.SetRequestPath("/foo/bar")
	if ctx.GetRequestPath() != "/foo/bar" {
		t.Errorf("RequestPath not set/get correctly")
	}
}

func TestEditorContext_RequestHeaders(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	headers := map[string][]string{"foo": {"bar"}, "baz": {"qux"}}
	ctx.SetRequestHeaders(headers)
	if !reflect.DeepEqual(ctx.GetRequestHeaders(), headers) {
		t.Errorf("RequestHeaders not set/get correctly")
	}
	if !ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestHeadersDirty not set correctly")
	}
	if got := ctx.GetRequestHeader("foo"); !reflect.DeepEqual(got, []string{"bar"}) {
		t.Errorf("GetRequestHeader failed")
	}
}

func TestEditorContext_RequestQueries(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	queries := map[string][]string{"foo": {"bar"}, "baz": {"qux"}}
	ctx.SetRequestQueries(queries)
	if !reflect.DeepEqual(ctx.GetRequestQueries(), queries) {
		t.Errorf("RequestQueries not set/get correctly")
	}
	if !ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestHeadersDirty not set correctly")
	}
	if got := ctx.GetRequestQuery("foo"); !reflect.DeepEqual(got, []string{"bar"}) {
		t.Errorf("GetRequestQuery failed")
	}
}

func TestEditorContext_ResponseHeaders(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	headers := map[string][]string{"foo": {"bar"}, "baz": {"qux"}}
	ctx.SetResponseHeaders(headers)
	if !reflect.DeepEqual(ctx.GetResponseHeaders(), headers) {
		t.Errorf("ResponseHeaders not set/get correctly")
	}
	if !ctx.IsResponseHeadersDirty() {
		t.Errorf("ResponseHeadersDirty not set correctly")
	}
	if got := ctx.GetResponseHeader("foo"); !reflect.DeepEqual(got, []string{"bar"}) {
		t.Errorf("GetResponseHeader failed")
	}
}

func TestEditorContext_RefValueAndValues(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	rh := newTestRef(RefTypeRequestHeader, "foo")
	rq := newTestRef(RefTypeRequestQuery, "bar")
	rh2 := newTestRef(RefTypeResponseHeader, "baz")

	ctx.SetRefValue(rh, "v1")
	ctx.SetRefValues(rq, []string{"v2", "v3"})
	ctx.SetRefValues(rh2, []string{"v4"})

	if v := ctx.GetRefValue(rh); v != "v1" {
		t.Errorf("GetRefValue(RequestHeader) failed: %v", v)
	}
	if v := ctx.GetRefValues(rq); !reflect.DeepEqual(v, []string{"v2", "v3"}) {
		t.Errorf("GetRefValues(RequestQuery) failed: %v", v)
	}
	if v := ctx.GetRefValues(rh2); !reflect.DeepEqual(v, []string{"v4"}) {
		t.Errorf("GetRefValues(ResponseHeader) failed: %v", v)
	}
}

func TestEditorContext_DeleteRefValues(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	rh := newTestRef(RefTypeRequestHeader, "foo")
	rq := newTestRef(RefTypeRequestQuery, "bar")
	rh2 := newTestRef(RefTypeResponseHeader, "baz")

	ctx.SetRefValue(rh, "v1")
	ctx.SetRefValues(rq, []string{"v2", "v3"})
	ctx.SetRefValues(rh2, []string{"v4"})

	ctx.DeleteRefValues(rh)
	ctx.DeleteRefValues(rq)
	ctx.DeleteRefValues(rh2)

	if v := ctx.GetRefValues(rh); len(v) != 0 {
		t.Errorf("DeleteRefValues(RequestHeader) failed: %v", v)
	}
	if v := ctx.GetRefValues(rq); len(v) != 0 {
		t.Errorf("DeleteRefValues(RequestQuery) failed: %v", v)
	}
	if v := ctx.GetRefValues(rh2); len(v) != 0 {
		t.Errorf("DeleteRefValues(ResponseHeader) failed: %v", v)
	}
}

func TestEditorContext_ResetDirtyFlags(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	ctx.SetRequestHeaders(map[string][]string{"foo": {"bar"}})
	ctx.SetRequestQueries(map[string][]string{"foo": {"bar"}})
	ctx.SetResponseHeaders(map[string][]string{"foo": {"bar"}})
	ctx.ResetDirtyFlags()
	if ctx.IsRequestHeadersDirty() || ctx.IsRequestHeadersDirty() || ctx.IsResponseHeadersDirty() {
		t.Errorf("ResetDirtyFlags failed")
	}
}

func TestEditorContext_IsRequestHeadersDirty_SetHeaders(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	if ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestHeadersDirty should be false initially")
	}
	ctx.SetRequestHeaders(map[string][]string{"foo": {"bar"}})
	if !ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestHeadersDirty should be true after SetRequestHeaders")
	}
	ctx.ResetDirtyFlags()
	if ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestHeadersDirty should be false after ResetDirtyFlags")
	}
	ref := newTestRef(RefTypeRequestHeader, "foo")
	ctx.SetRefValue(ref, "baz")
	if !ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestHeadersDirty should be true after SetRefValue")
	}
	ctx.ResetDirtyFlags()
	ctx.DeleteRefValues(ref)
	if !ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestHeadersDirty should be true after DeleteRefValues")
	}
}

func TestEditorContext_IsRequestHeadersDirty_SetQueries(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	if ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestQueriesDirty should be false initially")
	}
	ctx.SetRequestQueries(map[string][]string{"foo": {"bar"}})
	if !ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestQueriesDirty should be true after SetRequestQueries")
	}
	ctx.ResetDirtyFlags()
	if ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestQueriesDirty should be false after ResetDirtyFlags")
	}
	ref := newTestRef(RefTypeRequestQuery, "foo")
	ctx.SetRefValues(ref, []string{"baz"})
	if !ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestQueriesDirty should be true after SetRefValues")
	}
	ctx.ResetDirtyFlags()
	ctx.DeleteRefValues(ref)
	if !ctx.IsRequestHeadersDirty() {
		t.Errorf("RequestQueriesDirty should be true after DeleteRefValues")
	}
}

func TestEditorContext_IsResponseHeadersDirty(t *testing.T) {
	ctx := NewEditorContext().(*editorContext)
	if ctx.IsResponseHeadersDirty() {
		t.Errorf("ResponseHeadersDirty should be false initially")
	}
	ctx.SetResponseHeaders(map[string][]string{"foo": {"bar"}})
	if !ctx.IsResponseHeadersDirty() {
		t.Errorf("ResponseHeadersDirty should be true after SetResponseHeaders")
	}
	ctx.ResetDirtyFlags()
	if ctx.IsResponseHeadersDirty() {
		t.Errorf("ResponseHeadersDirty should be false after ResetDirtyFlags")
	}
	ref := newTestRef(RefTypeResponseHeader, "foo")
	ctx.SetRefValues(ref, []string{"baz"})
	if !ctx.IsResponseHeadersDirty() {
		t.Errorf("ResponseHeadersDirty should be true after SetRefValues")
	}
	ctx.ResetDirtyFlags()
	ctx.DeleteRefValues(ref)
	if !ctx.IsResponseHeadersDirty() {
		t.Errorf("ResponseHeadersDirty should be true after DeleteRefValues")
	}
}

func TestEditorContext_QueryEditsPreserveEscapedPath(t *testing.T) {
	paths := []string{
		"/objects/a%2Fb",
		"/objects/a%2fb",
		"/objects/a%3Fb",
		"/objects/a%23b",
		"/objects/a%25b",
		"/objects/%252F",
		"/objects/hello%20world",
		"/objects/%E4%BD%A0%E5%A5%BD",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			ctx := NewEditorContext()
			ctx.SetRequestHeaders(map[string][]string{":path": {path + "?old=1"}})
			ctx.ResetDirtyFlags()

			ref := newTestRef(RefTypeRequestQuery, "new")
			ctx.SetRefValues(ref, []string{"2", "3"})
			if got, want := ctx.GetRequestHeader(":path")[0], path+"?new=2&new=3&old=1"; got != want {
				t.Fatalf("setting query changed escaped path: got %q, want %q", got, want)
			}
			if !ctx.IsRequestHeadersDirty() {
				t.Fatal("query edit did not mark request headers dirty")
			}

			ctx.DeleteRefValues(ref)
			if got, want := ctx.GetRequestHeader(":path")[0], path+"?old=1"; got != want {
				t.Fatalf("deleting query changed escaped path: got %q, want %q", got, want)
			}

			ctx.SetRequestQueries(map[string][]string{"replacement": {"a&b"}})
			if got, want := ctx.GetRequestHeader(":path")[0], path+"?replacement=a%26b"; got != want {
				t.Fatalf("replacing queries changed escaped path: got %q, want %q", got, want)
			}
		})
	}
}

func TestEditorContext_SetRequestPathPreservesEscapesAcrossQueryEdits(t *testing.T) {
	ctx := NewEditorContext()
	ctx.SetRequestHeaders(map[string][]string{":path": {"/old?keep=1"}})
	ctx.SetRequestPath("/new/a%2Fb%3Fc%23d%25")
	ctx.SetRefValue(newTestRef(RefTypeRequestQuery, "next"), "2")
	if got, want := ctx.GetRequestHeader(":path")[0], "/new/a%2Fb%3Fc%23d%25?keep=1&next=2"; got != want {
		t.Fatalf("query edit changed replacement path: got %q, want %q", got, want)
	}
}
