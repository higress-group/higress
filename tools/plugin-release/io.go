// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func readJSON(path string, out any) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return data, nil
}

func writeCanonical(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func runGit(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func resolveCommit(root, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	return runGit(root, "rev-parse", "--verify", ref+"^{commit}")
}

func requireAncestor(root, ancestor, descendant string) error {
	cmd := exec.Command("git", "-C", root, "merge-base", "--is-ancestor", ancestor, descendant)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("comparison base %s is not an ancestor of target %s: %w: %s", ancestor, descendant, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func changedPaths(root, base, target string) ([]string, error) {
	if base == "" {
		return nil, nil
	}
	out, err := runGit(root, "diff", "--name-only", "--diff-filter=ACDMRTUXB", base, target)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	paths := strings.Split(out, "\n")
	sort.Strings(paths)
	return paths, nil
}

func filesAtCommit(root, commit string) ([]string, error) {
	out, err := runGit(root, "ls-tree", "-r", "--name-only", commit)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func blobID(root, commit, path string) (string, error) {
	return runGit(root, "rev-parse", commit+":"+path)
}

func fileAtCommit(root, commit, path string) (string, error) {
	return runGit(root, "show", commit+":"+path)
}

func patternMatches(pattern, path string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	path = filepath.ToSlash(path)
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "**")
		return strings.HasPrefix(path, prefix)
	}
	return path == pattern
}

func isDocumentationPath(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	return strings.HasSuffix(lower, ".md") || strings.HasPrefix(lower, "readme") ||
		base == "VERSION" || strings.Contains(filepath.ToSlash(path), "/design/")
}

func pluginInputMatches(c Catalog, p Plugin, path string) bool {
	for _, pattern := range p.ArtifactInputs {
		if patternMatches(pattern, path) && !isDocumentationPath(path) {
			return true
		}
	}
	for _, group := range p.SharedInputGroups {
		for _, pattern := range c.SharedInputGroups[group] {
			if patternMatches(pattern, path) {
				return true
			}
		}
	}
	return false
}

func inputHash(root, commit, version string, c Catalog, p Plugin) (string, error) {
	files, err := filesAtCommit(root, commit)
	if err != nil {
		return "", err
	}
	selected := make([]string, 0)
	for _, path := range files {
		if pluginInputMatches(c, p, path) {
			selected = append(selected, path)
		}
	}
	sort.Strings(selected)
	h := sha256.New()
	for _, path := range selected {
		blob, err := blobID(root, commit, path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%s\x00", path, blob)
	}
	fmt.Fprintf(h, "VERSION\x00%s\x00", version)
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
