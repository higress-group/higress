package main

import (
	"testing"

	"github.com/tidwall/gjson"
)

func Test_parseIPNets(t *testing.T) {
	type args struct {
		array []gjson.Result
	}
	tests := []struct {
		name    string
		args    args
		wantVal bool
		wantErr bool
	}{
		{
			name: "",
			args: args{
				array: gjson.Parse(`["127.0.0.1/30","10.0.0.1"]`).Array(),
			},
			wantVal: true,
			wantErr: false,
		},
		{
			name: "",
			args: args{
				array: gjson.Parse(``).Array(),
			},
			wantVal: false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIPNets(tt.args.array)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIPNets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantVal && got == nil {
				return
			}
			if _, found, _ := got.GetByString("10.0.0.1"); found != tt.wantVal {
				t.Errorf("parseIPNets() got = %v, want %v", found, tt.wantVal)
				return
			}
		})
	}
}

func Test_parseIP(t *testing.T) {
	type args struct {
		source     string
		fromHeader bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{
			name: "case 1",
			args: args{
				"127.0.0.1",
				false,
			},
			want: "127.0.0.1",
		},
		{
			name: "case 2",
			args: args{
				"127.0.0.1:12",
				false,
			},
			want: "127.0.0.1",
		},
		{
			name: "case 3",
			args: args{
				"fe80::14d5:8aff:fed9:2114",
				false,
			},
			want: "fe80::14d5:8aff:fed9:2114",
		},
		{
			name: "case 4",
			args: args{
				"[fe80::14d5:8aff:fed9:2114]:123",
				false,
			},
			want: "fe80::14d5:8aff:fed9:2114",
		},
		{
			name: "case 5",
			args: args{
				"127.0.0.1:12,[fe80::14d5:8aff:fed9:2114]:123",
				true,
			},
			want: "127.0.0.1",
		},
		{
			name: "case 6",
			args: args{
				"127.0.0.1,[fe80::14d5:8aff:fed9:2114]:123",
				true,
			},
			want: "127.0.0.1",
		},
		{
			name: "case 7",
			args: args{
				"[fe80::14d5:8aff:fed9:2114]:123,127.0.0.1",
				true,
			},
			want: "fe80::14d5:8aff:fed9:2114",
		},
		{
			name: "case 8",
			args: args{
				"127.0.0.1 , [fe80::14d5:8aff:fed9:2114]:123",
				true,
			},
			want: "127.0.0.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseIP(tt.args.source, tt.args.fromHeader); got != tt.want {
				t.Errorf("parseIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseIPMalformedHeaderDoesNotPanic(t *testing.T) {
	// X-Forwarded-For is attacker controlled when ip_source_type is "header".
	for _, source := range []string{"]", "]:80", "]:80, 127.0.0.1", "[::1]"} {
		t.Run(source, func(t *testing.T) {
			got := parseIP(source, true)
			if source == "[::1]" && got != "::1" {
				t.Errorf("parseIP(%q, true) = %q, want %q", source, got, "::1")
			}
		})
	}
}
