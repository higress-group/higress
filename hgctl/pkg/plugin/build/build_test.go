// Copyright (c) 2022 Alibaba Group Holding Ltd.
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

package build

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/higress/hgctl/pkg/plugin/utils"
)

func TestCleanupForErrorPreservesOutput(t *testing.T) {
	for _, outputType := range []string{"files", "image"} {
		t.Run(outputType, func(t *testing.T) {
			root := t.TempDir()
			output := filepath.Join(root, "output")
			nested := filepath.Join(output, "nested")
			if err := os.MkdirAll(nested, 0755); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(nested, "user-file")
			if err := os.WriteFile(sentinel, []byte("original content"), 0600); err != nil {
				t.Fatal(err)
			}
			temp := filepath.Join(root, "owned-temp")
			if err := os.Mkdir(temp, 0755); err != nil {
				t.Fatal(err)
			}
			entry := filepath.Join(root, "owned-entrypoint")
			if err := os.WriteFile(entry, []byte("temporary"), 0600); err != nil {
				t.Fatal(err)
			}
			b := &Builder{tempDir: temp, dockerEntrypoint: entry, Debugger: utils.NewDefaultDebugger(false, io.Discard)}
			b.Output.Type = outputType
			b.Output.Dest = output
			b.CleanupForError()
			b.CleanupForError() // Repeated failure/interrupt cleanup remains safe.
			content, err := os.ReadFile(sentinel)
			if err != nil || string(content) != "original content" {
				t.Fatalf("caller file changed: content=%q error=%v", content, err)
			}
			for _, path := range []string{temp, entry} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("owned path %s remains: %v", path, err)
				}
			}
		})
	}
}
