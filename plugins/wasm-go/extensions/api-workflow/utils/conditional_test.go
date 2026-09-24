package utils

import (
	"reflect"
	"testing"
)

func TestExecConditionalStr(t *testing.T) {

	tests := []struct {
		name    string
		args    string
		want    bool
		wantErr bool
	}{
		{"eq int true", "eq 1 1", true, false},
		{"eq int false", "eq 1 2", false, false},
		{"eq str true", "eq foo foo", true, false},
		{"eq str false", "eq foo boo", false, false},
		{"eq float true", "eq 0.99 0.99", true, false},
		{"eq float false", "eq 1.1 2.2", false, false},
		{"eq float int  false", "eq 1.0 1", false, false},
		{"eq float str  false", "eq 1.0 foo", false, false},
		{"lt true", "lt 1.1 2", true, false},
		{"lt false", "lt 2 1", false, false},
		{"le true", "le 1 2", true, false},
		{"le false", "le 2 1", false, false},
		{"gt true", "gt 2 1", true, false},
		{"gt false", "gt 1 2", false, false},
		{"ge true", "ge 2 1", true, false},
		{"ge false", "ge 1 2", false, false},
		{"and true", "and true true", true, false},
		{"and false", "and true false", false, false},
		{"or true", "or true false", true, false},
		{"or false", "or false false", false, false},
		{"contain true", "contain helloworld world", true, false},
		{"contain false", "contain helloworld moon", false, false},
		{"invalid input", "invalid", false, true},
		{"nested expression 1", "and (eq 1 1) (lt 2 3)", true, false},
		{"nested expression 2", "or (eq 1 2) (and (eq 1 1) (gt 2 3))", false, false},
		{"nested expression error", "or (eq 1 2) (and (eq 1 1) (gt 2 3)))", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExecConditionalStr(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExecConditionalStr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExecConditionalStr() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTmplStr(t *testing.T) {
	type args struct {
		tmpl string
	}
	tests := []struct {
		name string
		args string
		want map[string]string
	}{
		{"normal", "{{foo}}", map[string]string{"{{foo}}": "foo"}},
		{"single", "{foo}", map[string]string{}},
		{"empty", "foo", map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTmplStr(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseTmplStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplacedStr(t *testing.T) {
	type args struct {
		tmpl string
		kvs  map[string]string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"normal", args{tmpl: "hello,{{foo}}", kvs: map[string]string{"{{foo}}": "bot"}}, "hello,bot"},
		{"empty", args{tmpl: "hello,foo", kvs: map[string]string{"{{foo}}": "bot"}}, "hello,foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReplacedStr(tt.args.tmpl, tt.args.kvs); got != tt.want {
				t.Errorf("ReplacedStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkExecConditionalStr(b *testing.B) {
	const condition = "or (eq production staging) (and (ge 42 10) (contain higress-gateway gateway))"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		got, err := ExecConditionalStr(condition)
		if err != nil || !got {
			b.Fatalf("ExecConditionalStr() = %v, %v", got, err)
		}
	}
}

func BenchmarkParseTmplStr(b *testing.B) {
	const tmpl = `{"path":"{{request.path}}","method":"{{request.method}}","authorization":"{{request.header.authorization}}","body":"{{request.body}}"}`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		got := ParseTmplStr(tmpl)
		if len(got) != 4 {
			b.Fatalf("ParseTmplStr() returned %d entries", len(got))
		}
	}
}
