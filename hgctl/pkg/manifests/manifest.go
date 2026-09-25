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

package manifests

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

// FS embeds the manifests
//
//go:embed profiles/*
//go:embed gatewayapi/*
//go:embed istiobase/*
//go:embed agent/*
var FS embed.FS

// BuiltinOrDir returns a FS for the provided directory. If no directory is passed, the compiled in
// FS will be used
func BuiltinOrDir(dir string) fs.FS {
	if dir == "" {
		return FS
	}
	return os.DirFS(dir)
}

// This function will write the embed sourceDir's files to target dir
func ExtractEmbedFiles(fsys fs.FS, srcDir, targetDir string) error {
	return fs.WalkDir(fsys, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relDir, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relDir)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}

		// if this file already exists, then return
		existing, err := os.ReadFile(targetPath)
		if err == nil {
			if bytes.Equal(existing, data) {
				return nil
			}
		} else if !os.IsNotExist(err) {
			return err
		}

		return os.WriteFile(targetPath, data, 0644)
	})
}
