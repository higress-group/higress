package configmap

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidTracing(t *testing.T) {
	// nil tracing
	assert.NoError(t, validTracing(nil))

	// timeout <= 0
	tr := &Tracing{Enable: true, Timeout: 0, Sampling: 50, Zipkin: &Zipkin{Service: "svc", Port: "9411"}}
	assert.Error(t, validTracing(tr))

	// sampling < 0
	tr.Timeout = 100
	tr.Sampling = -1
	assert.Error(t, validTracing(tr))

	// sampling > 100
	tr.Sampling = 101
	assert.Error(t, validTracing(tr))

	// multiple tracers
	tr.Sampling = 50
	tr.Zipkin = &Zipkin{Service: "svc", Port: "9411"}
	tr.Skywalking = &Skywalking{Service: "svc", Port: "11800"}
	assert.Error(t, validTracing(tr))

	// valid zipkin
	tr.Skywalking = nil
	assert.NoError(t, validTracing(tr))

	// valid skywalking
	tr.Zipkin = nil
	tr.Skywalking = &Skywalking{Service: "svc", Port: "11800"}
	assert.NoError(t, validTracing(tr))

	// valid opentelemetry
	tr.Skywalking = nil
	tr.OpenTelemetry = &OpenTelemetry{Service: "svc", Port: "4317"}
	assert.NoError(t, validTracing(tr))

	// custom tag duplicate
	tr.CustomTag = []CustomTag{{Tag: "foo", Literal: "bar"}, {Tag: "foo", Literal: "baz"}}
	assert.Error(t, validTracing(tr))
}

func TestValidCustomTag(t *testing.T) {
	// empty tag name
	tag := CustomTag{Tag: "", Literal: "val"}
	assert.Error(t, validCustomTag(tag))

	// empty value
	tag = CustomTag{Tag: "foo"}
	assert.Error(t, validCustomTag(tag))

	// multiple values
	tag = CustomTag{Tag: "foo", Literal: "val", Environment: &CustomTagValue{Key: "env"}}
	assert.Error(t, validCustomTag(tag))

	// valid literal
	tag = CustomTag{Tag: "foo", Literal: "val"}
	assert.NoError(t, validCustomTag(tag))

	// valid environment
	tag = CustomTag{Tag: "foo", Environment: &CustomTagValue{Key: "env"}}
	assert.NoError(t, validCustomTag(tag))

	// valid requestHeader
	tag = CustomTag{Tag: "foo", RequestHeader: &CustomTagValue{Key: "header"}}
	assert.NoError(t, validCustomTag(tag))
}

func TestCompareTracing(t *testing.T) {
	old := &Tracing{Enable: true, Timeout: 100, Sampling: 50}
	new := &Tracing{Enable: true, Timeout: 100, Sampling: 50}
	res, err := compareTracing(old, new)
	assert.NoError(t, err)
	assert.Equal(t, ResultNothing, res)

	res, err = compareTracing(nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, ResultNothing, res)

	res, err = compareTracing(old, nil)
	assert.NoError(t, err)
	assert.Equal(t, ResultDelete, res)

	new.Timeout = 200
	res, err = compareTracing(old, new)
	assert.NoError(t, err)
	assert.Equal(t, ResultReplace, res)
}

func TestDeepCopyTracing(t *testing.T) {
	tr := &Tracing{Enable: true, Timeout: 100, Sampling: 50}
	copy, err := deepCopyTracing(tr)
	assert.NoError(t, err)
	assert.Equal(t, tr, copy)
	copy.Timeout = 200
	assert.NotEqual(t, tr.Timeout, copy.Timeout)
}

func TestNewDefaultTracing(t *testing.T) {
	tr := NewDefaultTracing()
	assert.False(t, tr.Enable)
	assert.Equal(t, int32(500), tr.Timeout)
	assert.Equal(t, 100.0, tr.Sampling)
}

func TestConstructCustomTagsJsonOutput(t *testing.T) {
	tags := []CustomTag{
		{Tag: "foo", Literal: "bar"},
		{Tag: "env", Environment: &CustomTagValue{Key: "ENV", DefaultValue: "def"}},
		{Tag: "header", RequestHeader: &CustomTagValue{Key: "X-Header", DefaultValue: "def"}},
	}
	jsonStr := constructJsonCustomTags(tags)
	var result []EnvoyCustomTag
	assert.NoError(t, json.Unmarshal([]byte(jsonStr), &result))
	assert.Equal(t, 3, len(result))
	assert.Equal(t, "foo", result[0].Tag)
	assert.Equal(t, "bar", result[0].Literal.Value)
	assert.Equal(t, "env", result[1].Tag)
	assert.Equal(t, "ENV", result[1].Environment.Name)
	assert.Equal(t, "def", result[1].Environment.DefaultValue)
	assert.Equal(t, "header", result[2].Tag)
	assert.Equal(t, "X-Header", result[2].RequestHeader.Name)
	assert.Equal(t, "def", result[2].RequestHeader.DefaultValue)
}

func TestConstructCustomTags_EmptyValues(t *testing.T) {
	var tags []CustomTag
	jsonStr := constructJsonCustomTags(tags)
	assert.Equal(t, "[]", jsonStr)
}
