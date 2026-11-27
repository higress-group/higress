package jsonbody

import "testing"

func TestDuplicateTopLevelMember(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "single member is accepted",
			body: `{"model":"qwen-max","messages":[{"role":"user","content":"hi"}]}`,
			want: false,
		},
		{
			name: "duplicated member is rejected",
			body: `{"model":"qwen-max","model":"gpt-4o","messages":[]}`,
			want: true,
		},
		{
			name: "duplicated member with identical values is rejected",
			body: `{"model":"qwen-max","stream":true,"model":"qwen-max"}`,
			want: true,
		},
		{
			name: "triple member is rejected",
			body: `{"model":"a","model":"b","model":"c"}`,
			want: true,
		},
		{
			name: "key spelling inside a string value is not a member",
			body: `{"messages":[{"role":"user","content":"set \"model\" to gpt-4 and \"model\" again"}],"model":"qwen-max"}`,
			want: false,
		},
		{
			name: "key spelling inside a top level string value is not a member",
			body: `{"prompt":"the \"model\" member and one more \"model\" mention","model":"qwen-max"}`,
			want: false,
		},
		{
			name: "nested object member is not a top level member",
			body: `{"tools":[{"type":"function","function":{"name":"model","parameters":{"model":"a","model":"b"}}}],"model":"qwen-max"}`,
			want: false,
		},
		{
			name: "duplicate only in nested object is not rejected",
			body: `{"model":"qwen-max","response_format":{"type":"json_schema","schema":{"model":"x","model":"y"}}}`,
			want: false,
		},
		{
			name: "key inside a nested array element is not a top level member",
			body: `{"model":"qwen-max","messages":[{"role":"user","content":[{"type":"text","model":"a","model":"b"}]}]}`,
			want: false,
		},
		{
			name: "member names are matched exactly",
			body: `{"models":"a","models":"b","model_name":"c","model_name":"d","model":"qwen-max"}`,
			want: false,
		},
		{
			name: "escaped member spelling counts as the same member",
			body: `{"model":"a","mo\u0064el":"b"}`,
			want: true,
		},
		{
			name: "escaped member spelling alone is not a duplicate",
			body: `{"mo\u0064el":"b","models":"a"}`,
			want: false,
		},
		{
			name: "whitespace and newlines between members are tolerated",
			body: "{\n  \"model\" : \"a\" ,\n  \"model\" : \"b\"\n}",
			want: true,
		},
		{
			name: "member order does not matter",
			body: `{"stream":false,"messages":[],"model":"a","temperature":0,"model":"b"}`,
			want: true,
		},
		{
			name: "empty object is accepted",
			body: `{}`,
			want: false,
		},
		{
			name: "empty body is accepted",
			body: ``,
			want: false,
		},
		{
			name: "top level array is accepted",
			body: `[{"model":"a","model":"b"}]`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DuplicateTopLevelMember([]byte(tt.body), "model")
			if err != nil {
				t.Fatalf("DuplicateTopLevelMember() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("DuplicateTopLevelMember() = %v, want %v for body %s", got, tt.want, tt.body)
			}
		})
	}
}

func TestDuplicateTopLevelMemberOtherKey(t *testing.T) {
	got, err := DuplicateTopLevelMember([]byte(`{"messages":[],"messages":[]}`), "messages")
	if err != nil {
		t.Fatalf("DuplicateTopLevelMember() error = %v", err)
	}
	if !got {
		t.Error("DuplicateTopLevelMember() = false, want true for duplicated messages member")
	}
}

func TestDuplicateTopLevelMemberMalformed(t *testing.T) {
	bodies := []string{
		`{"model":"a"`,
		`{"model":"a" ` + "\n",
		`{"model":"unterminated`,
		`{"model":"a"}}`,
		`{"model":"a"\`,
		`{"model":[1,2`,
	}
	for _, body := range bodies {
		got, err := DuplicateTopLevelMember([]byte(body), "model")
		if err == nil {
			t.Errorf("DuplicateTopLevelMember(%q) error = nil, want ErrMalformedJSON", body)
		}
		if got {
			t.Errorf("DuplicateTopLevelMember(%q) = true, want false alongside the error", body)
		}
	}
}

func TestDuplicateTopLevelMemberLeavesGrammarToCallers(t *testing.T) {
	// The scan tracks structure only, so a body that is unparseable for other
	// reasons must not be turned into a duplicate-member reject: the plugin's
	// existing body validation already answers those requests.
	bodies := []string{
		`{"model":"a",}`,
		`{"model":}`,
		`not json at all`,
	}
	for _, body := range bodies {
		got, err := DuplicateTopLevelMember([]byte(body), "model")
		if err != nil {
			t.Errorf("DuplicateTopLevelMember(%q) error = %v, want nil", body, err)
		}
		if got {
			t.Errorf("DuplicateTopLevelMember(%q) = true, want false", body)
		}
	}

	// A missing value is a grammar defect, but the duplicated member name is
	// still structural and must still be rejected.
	got, err := DuplicateTopLevelMember([]byte(`{"model":"a","model":}`), "model")
	if err != nil {
		t.Fatalf(`DuplicateTopLevelMember("{\"model\":\"a\",\"model\":}") error = %v`, err)
	}
	if !got {
		t.Error(`DuplicateTopLevelMember("{\"model\":\"a\",\"model\":}") = false, want true`)
	}
}

func TestDuplicateMemberMessage(t *testing.T) {
	want := `duplicate "model" member in request body`
	if got := DuplicateMemberMessage("model"); got != want {
		t.Errorf("DuplicateMemberMessage() = %q, want %q", got, want)
	}
}
