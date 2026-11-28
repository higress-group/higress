package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestManagedHistoryNonprodWorkflowContract(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/verify-plugin-server-snapshot.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	trigger := regexp.MustCompile(`(?m)^on:\n((?:[ \t].*\n|\n)*)`).FindString(workflow)
	if !strings.Contains(trigger, "  workflow_dispatch:\n") ||
		len(regexp.MustCompile(`(?m)^  [a-z_]+:`).FindAllString(trigger, -1)) != 1 {
		t.Fatal("non-production acceptance must be workflow_dispatch only")
	}
	for _, input := range []string{"higress_commit", "snapshot_path", "snapshot_sha256", "plugin_server_commit"} {
		pattern := `(?m)^      ` + input + `:\n        description: [^\n]+\n        required: true\n        type: string$`
		if !regexp.MustCompile(pattern).MatchString(trigger) {
			t.Fatalf("missing required string input %s", input)
		}
	}
	if len(regexp.MustCompile(`(?m)^      [a-z0-9_]+:`).FindAllString(trigger, -1)) != 4 {
		t.Fatal("acceptance must expose exactly four pinned inputs")
	}
	for _, required := range []string{
		"runs-on: ubuntu-22.04", "timeout-minutes: 120", "contents: read", "persist-credentials: false",
		"ref: ${{ github.workflow_sha }}", "fetch-depth: 0", "go-version: '1.24.0'", "version: 1.2.3",
		"platforms: arm64,amd64", "name: nonprod-plugin-server", "driver: docker-container",
		"--committed-source \"$HIGRESS_COMMIT\" --resolve --oci-source candidate", "--plan", "--previous",
		"timeout 10m python3 -m unittest test_pull_plugins.py", "timeout 40m go run -p=1 .", "nonprod_plugin_server.py prepare",
		"--snapshot-sha256 \"$SNAPSHOT_SHA256\" --out \"$CTX\"", "--load", "USE_LOCAL_PLUGINS=true",
		"docker buildx build --platform linux/amd64", "docker buildx build --platform linux/arm64",
		"higress-plugin-server-nonprod:amd64", "higress-plugin-server-nonprod:arm64",
		"SNAPSHOT_SHA256=$SNAPSHOT_SHA256", "HIGRESS_SOURCE_COMMIT=$HIGRESS_COMMIT",
		"PLUGIN_SERVER_SOURCE_COMMIT=$PLUGIN_SERVER_COMMIT", "GATEWAY_VERSION=$GATEWAY_VERSION",
		"docker inspect --type image", "docker run -d --rm --platform", "127.0.0.1:$port:8080", "trap cleanup EXIT",
		"org.opencontainers.image.revision", "io.higress.higress-source-commit",
		"io.higress.plugin-snapshot-sha256", "io.higress.gateway-version", "snapshot-inventory.json",
		"python3 tools/plugin-release/verify_snapshot_http.py", "--current-source candidate --all-managed",
		"len(cpp_history_entries) != 9", "len(canonical_cpp_rows) != 18", "mapping[\"inventoryKey\"]",
		"mapping.get(\"aliases\", [])", "for version in 1.0.0 2.0.0", "cmp ", "NONPROD_MANAGED_HISTORY_OK",
		"report-", "nonprod-context.json", "inventorySha256", "imageId", "mcpResults", "SHA256SUMS",
		"if: always()", "actions/upload-artifact@",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("non-production acceptance lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"docker push", "--push", "imagetools create", "environment:", "contents: write", "packages: write",
		"oras push", "oras tag", "oras manifest push", "docker login", "oras login", "docker tag",
		"repository_dispatch", "gh workflow run", "client_payload", ":latest", "secrets.",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("non-production acceptance contains forbidden mutation or privilege %q", forbidden)
		}
	}
	for _, line := range strings.Split(workflow, "\n") {
		if strings.Contains(line, "oras cp") && !strings.Contains(line, "--to-oci-layout") {
			t.Fatalf("ORAS copies must have a local layout destination: %s", line)
		}
	}
	reader := strings.Index(workflow, "timeout 10m python3 -m unittest test_pull_plugins.py")
	prepare := strings.Index(workflow, "nonprod_plugin_server.py prepare")
	build := strings.Index(workflow, "docker buildx build --platform")
	if reader < 0 || prepare < reader || build < prepare {
		t.Fatal("unchanged reader tests must precede scratch preparation and local builds")
	}
	if strings.LastIndex(workflow, "NONPROD_MANAGED_HISTORY_OK") < strings.Index(workflow, "Upload non-production") {
		t.Fatal("success marker must follow both platform reports and evidence upload")
	}
}
