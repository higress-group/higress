// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type semver struct {
	major, minor, patch int
	prerelease          string
}

func parseSemver(raw string) (semver, error) {
	m := semverPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return semver{}, fmt.Errorf("invalid semantic version %q", raw)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return semver{major: major, minor: minor, patch: patch, prerelease: m[4]}, nil
}

func (v semver) stableString() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

func compareSemver(a, b semver) int {
	if a.major != b.major {
		return compareInt(a.major, b.major)
	}
	if a.minor != b.minor {
		return compareInt(a.minor, b.minor)
	}
	if a.patch != b.patch {
		return compareInt(a.patch, b.patch)
	}
	if a.prerelease == b.prerelease {
		return 0
	}
	if a.prerelease == "" {
		return 1
	}
	if b.prerelease == "" {
		return -1
	}
	return comparePrerelease(a.prerelease, b.prerelease)
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func comparePrerelease(a, b string) int {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(aa) && i < len(bb); i++ {
		if aa[i] == bb[i] {
			continue
		}
		ai, ae := strconv.Atoi(aa[i])
		bi, be := strconv.Atoi(bb[i])
		switch {
		case ae == nil && be == nil:
			return compareInt(ai, bi)
		case ae == nil:
			return -1
		case be == nil:
			return 1
		case aa[i] < bb[i]:
			return -1
		default:
			return 1
		}
	}
	return compareInt(len(aa), len(bb))
}

// isAlphaPrerelease reports whether the first dot-separated prerelease
// identifier is "alpha" (for example 1.0.0-alpha or 1.0.0-alpha.1). Alpha
// builds are development artifacts deferred from release selection; other
// prerelease families keep their existing promotion or fail-closed behavior.
func isAlphaPrerelease(prerelease string) bool {
	first, _, _ := strings.Cut(prerelease, ".")
	return first == "alpha"
}

func nextVersion(previous, current string) (string, error) {
	prev, err := parseSemver(previous)
	if err != nil {
		return "", fmt.Errorf("previous version: %w", err)
	}
	if prev.prerelease != "" {
		return "", fmt.Errorf("previous snapshot version %q is not stable", previous)
	}
	cur, err := parseSemver(current)
	if err != nil {
		return "", fmt.Errorf("current VERSION: %w", err)
	}
	if cur.prerelease == "" && compareSemver(cur, prev) > 0 {
		return cur.stableString(), nil
	}
	stableCur := cur
	stableCur.prerelease = ""
	if cur.prerelease != "" && compareSemver(stableCur, prev) > 0 {
		return stableCur.stableString(), nil
	}
	return fmt.Sprintf("%d.%d.%d", prev.major, prev.minor, prev.patch+1), nil
}

// nextBootstrapVersion is the one-time bridge for a reviewed public baseline
// that still exposes a prerelease tag. Ordinary managed snapshots require a
// stable previous version, but the first managed release promotes the current
// prerelease base to its stable form instead of inventing a patch beyond it.
func nextBootstrapVersion(previous, current string) (string, error) {
	prev, err := parseSemver(previous)
	if err != nil {
		return "", fmt.Errorf("previous version: %w", err)
	}
	if prev.prerelease == "" {
		return nextVersion(previous, current)
	}
	cur, err := parseSemver(current)
	if err != nil {
		return "", fmt.Errorf("current VERSION: %w", err)
	}
	if compareSemver(cur, prev) < 0 {
		return "", fmt.Errorf("current VERSION %q is older than bootstrap public version %q", current, previous)
	}
	stableCur := cur
	stableCur.prerelease = ""
	return stableCur.stableString(), nil
}
