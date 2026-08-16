// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPluginInputMatching(t *testing.T) {
	c := Catalog{SharedInputGroups: map[string][]string{"go": {"plugins/wasm-go/Makefile", "plugins/wasm-go/pkg/mcp/**"}}}
	p := Plugin{SourceDir: "plugins/wasm-go/extensions/mcp-server", ArtifactInputs: []string{"plugins/wasm-go/extensions/mcp-server/**"}, SharedInputGroups: []string{"go"}}
	for _, path := range []string{"plugins/wasm-go/extensions/mcp-server/main.go", "plugins/wasm-go/Makefile", "plugins/wasm-go/pkg/mcp/client.go"} {
		if !pluginInputMatches(c, p, path) {
			t.Fatalf("expected %s to match", path)
		}
	}
	for _, path := range []string{"plugins/wasm-go/extensions/mcp-server/README.md", "plugins/wasm-go/examples/mcp-server/main.go", "plugins/wasm-cpp/BUILD"} {
		if pluginInputMatches(c, p, path) {
			t.Fatalf("expected %s to be excluded", path)
		}
	}
}

func TestChangedPathsAndInputHash(t *testing.T) {
	root := t.TempDir()
	mustRun(t, root, "git", "init", "-q")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"), "1.0.0\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/README.md"), "one\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "base")
	base, _ := resolveCommit(root, "HEAD")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/README.md"), "two\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "docs")
	target, _ := resolveCommit(root, "HEAD")
	paths, err := changedPaths(root, base, target)
	if err != nil || len(paths) != 1 {
		t.Fatalf("changedPaths = %v, %v", paths, err)
	}
	c := Catalog{}
	p := Plugin{SourceDir: "plugins/wasm-go/extensions/demo", ArtifactInputs: []string{"plugins/wasm-go/extensions/demo/**"}}
	if pluginInputMatches(c, p, paths[0]) {
		t.Fatal("documentation-only change must not affect the artifact")
	}
	h1, err := inputHash(root, base, "1.0.0", c, p)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := inputHash(root, target, "1.0.0", c, p)
	if err != nil || h1 != h2 {
		t.Fatalf("documentation changed input hash: %s != %s (%v)", h1, h2, err)
	}
}

func mustWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRun(t *testing.T, root, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
}
