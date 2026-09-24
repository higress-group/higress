package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDigestReferencePreservesRegistryPort(t *testing.T) {
	got, err := digestReference("oci://registry.example:5000/plugins/demo:1.2.3", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil || got != "registry.example:5000/plugins/demo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("digestReference=%q, %v", got, err)
	}
}

func TestResolveOCIManifestUsesORAS123CompatibleDescriptorInvocation(t *testing.T) {
	ref := "registry.example/plugins/demo:1.2.3"
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var calls [][]string
	withORASRunner(t, func(args ...string) ([]byte, string, error) {
		calls = append(calls, append([]string(nil), args...))
		switch len(calls) {
		case 1:
			return []byte(`{"digest":"` + digest + `"}`), "", nil
		case 2:
			return []byte(`{"annotations":{"org.opencontainers.image.version":"1.2.3"}}`), "", nil
		default:
			t.Fatalf("unexpected ORAS invocation %v", args)
			return nil, "", nil
		}
	})

	manifest, err := resolveOCIManifest(ref)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Digest != digest || manifest.Annotations["org.opencontainers.image.version"] != "1.2.3" {
		t.Fatalf("unexpected manifest %#v", manifest)
	}
	want := [][]string{
		{"manifest", "fetch", ref, "--descriptor"},
		{"manifest", "fetch", ref},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("ORAS calls = %#v, want %#v", calls, want)
	}
}

func TestResolveOCIManifestReportsSanitizedORASStderr(t *testing.T) {
	withORASRunner(t, func(args ...string) ([]byte, string, error) {
		return nil, "denied: token=fixture-token authorization: Bearer fixture-auth authorization: Basic Zml4dHVyZS11c2VyOmZpeHR1cmUtcGFzc3dvcmQ= https://user:fixture-password@registry.example", errors.New("exit status 1")
	})

	_, err := resolveOCIManifest("registry.example/plugins/demo:1.2.3")
	if err == nil {
		t.Fatal("expected descriptor resolution failure")
	}
	message := err.Error()
	for _, required := range []string{"oras manifest fetch --descriptor failed", "denied:", "token=[REDACTED]", "authorization: [REDACTED]", "https://[REDACTED]@registry.example"} {
		if !strings.Contains(message, required) {
			t.Fatalf("error %q lacks %q", message, required)
		}
	}
	for _, secret := range []string{"fixture-token", "fixture-auth", "Zml4dHVyZS11c2VyOmZpeHR1cmUtcGFzc3dvcmQ=", "fixture-password"} {
		if strings.Contains(message, secret) {
			t.Fatalf("error leaked test credential %q: %q", secret, message)
		}
	}
}

func withORASRunner(t *testing.T, runner func(args ...string) ([]byte, string, error)) {
	t.Helper()
	previous := orasRunner
	orasRunner = runner
	t.Cleanup(func() { orasRunner = previous })
}

// TestReleaseWorkflowsNeverCombineDescriptorAndFormat statically proves no
// release-chain workflow invokes oras manifest fetch with the mutually
// exclusive --descriptor and --format flags on the same command line.
func TestReleaseWorkflowsNeverCombineDescriptorAndFormat(t *testing.T) {
	workflows, err := filepath.Glob("../../.github/workflows/*.yaml")
	if err != nil || len(workflows) == 0 {
		t.Fatalf("workflow glob failed: %v", err)
	}
	for _, path := range workflows {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.Lines(string(data)) {
			if strings.Contains(line, "--descriptor") && strings.Contains(line, "--format") {
				t.Fatalf("%s combines mutually exclusive --descriptor and --format: %s", path, strings.TrimSpace(line))
			}
		}
	}
}

// TestORAS123RejectsDescriptorCombinedWithFormat executes the real pinned
// ORAS 1.2.3 CLI: the combined --descriptor --format form must fail at flag
// parsing, while the canonical descriptor-only form every workflow uses must
// parse cleanly (a later registry/network failure is expected and proves the
// flags were accepted). Skipped when no oras binary is installed.
func TestORAS123RejectsDescriptorCombinedWithFormat(t *testing.T) {
	binary, err := exec.LookPath("oras")
	if err != nil {
		t.Skip("oras binary not installed; run with the workflow-pinned ORAS 1.2.3 CLI")
	}
	version, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("oras version failed: %v: %s", err, version)
	}
	if !strings.Contains(string(version), "1.2.3") {
		t.Skipf("executable descriptor contract requires the pinned ORAS 1.2.3, got %s", strings.TrimSpace(string(version)))
	}
	ref := "registry.invalid/plugins/demo:1.2.3"
	combined, combinedErr := exec.Command(binary, "manifest", "fetch", ref, "--descriptor", "--format", "json").CombinedOutput()
	if combinedErr == nil {
		t.Fatal("ORAS 1.2.3 must reject the mutually exclusive --descriptor --format combination")
	}
	canonical, canonicalErr := exec.Command(binary, "manifest", "fetch", ref, "--descriptor").CombinedOutput()
	if canonicalErr == nil {
		t.Fatal("an unresolvable reference must still fail after flag parsing succeeds")
	}
	if strings.Contains(string(canonical), "unknown flag") || strings.Contains(string(canonical), "mutually exclusive") {
		t.Fatalf("canonical descriptor form must parse under ORAS 1.2.3, got %s", canonical)
	}
	t.Logf("combined form rejected: %s", strings.TrimSpace(string(combined)))
	t.Logf("canonical form parsed, registry failure as expected: %s", strings.TrimSpace(string(canonical)))
}
