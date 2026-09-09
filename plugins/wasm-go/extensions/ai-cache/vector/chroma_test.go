// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vector

import (
	"reflect"
	"strings"
	"testing"
)

type noopChromaLog struct{}

func (noopChromaLog) Trace(string)                     {}
func (noopChromaLog) Tracef(string, ...interface{})    {}
func (noopChromaLog) Debug(string)                     {}
func (noopChromaLog) Debugf(string, ...interface{})    {}
func (noopChromaLog) Info(string)                      {}
func (noopChromaLog) Infof(string, ...interface{})     {}
func (noopChromaLog) Warn(string)                      {}
func (noopChromaLog) Warnf(string, ...interface{})     {}
func (noopChromaLog) Error(string)                     {}
func (noopChromaLog) Errorf(string, ...interface{})    {}
func (noopChromaLog) Critical(string)                  {}
func (noopChromaLog) Criticalf(string, ...interface{}) {}
func (noopChromaLog) ResetID(string)                   {}

func TestChromaProvider_ParseQueryResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		response    string
		want        []QueryResult
		wantErrText string
	}{
		{
			name:     "valid response",
			response: `{"ids":[["question-1","question-2"]],"distances":[[0.1,0.2]],"documents":[["answer-1","answer-2"]]}`,
			want: []QueryResult{
				{Text: "question-1", Score: 0.1, Answer: "answer-1"},
				{Text: "question-2", Score: 0.2, Answer: "answer-2"},
			},
		},
		{
			name:        "empty result",
			response:    `{"ids":[[]]}`,
			wantErrText: "no query results found in response",
		},
		{
			name:        "empty ids batches",
			response:    `{"ids":[]}`,
			wantErrText: "ids",
		},
		{
			name:        "multiple ids batches",
			response:    `{"ids":[["question-1"],["question-2"]],"distances":[[0.1],[0.2]],"documents":[["answer-1"],["answer-2"]]}`,
			wantErrText: "ids",
		},
		{
			name:        "missing distances batch",
			response:    `{"ids":[["question"]],"documents":[["answer"]]}`,
			wantErrText: "distances",
		},
		{
			name:        "missing documents batch",
			response:    `{"ids":[["question"]],"distances":[[0.1]]}`,
			wantErrText: "documents",
		},
		{
			name:        "distance count mismatch",
			response:    `{"ids":[["question-1","question-2"]],"distances":[[0.1]],"documents":[["answer-1","answer-2"]]}`,
			wantErrText: "distances",
		},
		{
			name:        "document count mismatch",
			response:    `{"ids":[["question-1","question-2"]],"distances":[[0.1,0.2]],"documents":[["answer-1"]]}`,
			wantErrText: "documents",
		},
	}

	provider := &ChromaProvider{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := provider.parseQueryResponse([]byte(tt.response), noopChromaLog{})
			if tt.wantErrText != "" {
				if err == nil {
					t.Fatalf("parseQueryResponse() error = nil, want error containing %q", tt.wantErrText)
				}
				if !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("parseQueryResponse() error = %q, want error containing %q", err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseQueryResponse() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseQueryResponse() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
