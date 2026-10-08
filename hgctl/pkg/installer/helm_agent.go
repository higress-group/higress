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

package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/v2/pkg/cmd/options"
)

type HelmReleaseCandidate = helm.HelmReleaseCandidate

const helmListPageSize = 256

type HelmReleaseSelector struct {
	Name      string
	Namespace string
}

type HelmReleaseReader interface {
	ListReleases(HelmReleaseSelector) ([]HelmReleaseCandidate, error)
	GetRetainedValues(HelmReleaseCandidate) (map[string]any, error)
}

type HelmAgent struct {
	profile        *helm.Profile
	writer         io.Writer
	helmBinaryName string
	quiet          bool
}

// HelmAgentOption configures a HelmAgent.
type HelmAgentOption func(*HelmAgent)

// WithHelmBinaryName overrides the Helm executable used by the agent.
func WithHelmBinaryName(name string) HelmAgentOption {
	return func(agent *HelmAgent) {
		agent.helmBinaryName = name
	}
}

func NewHelmAgent(profile *helm.Profile, writer io.Writer, quiet bool, opts ...HelmAgentOption) *HelmAgent {
	agent := &HelmAgent{
		profile:        profile,
		writer:         writer,
		helmBinaryName: "helm",
		quiet:          quiet,
	}
	for _, opt := range opts {
		opt(agent)
	}
	return agent
}

func NewHelmReleaseReader(opts ...HelmAgentOption) HelmReleaseReader {
	return NewHelmAgent(nil, io.Discard, true, opts...)
}

type helmListRelease struct {
	AppVersion string `json:"app_version,omitempty"`
	Chart      string `json:"chart,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Status     string `json:"status,omitempty"`
}

func (h *HelmAgent) ListReleases(selector HelmReleaseSelector) ([]HelmReleaseCandidate, error) {
	candidates := make([]HelmReleaseCandidate, 0)
	for offset := 0; ; offset += helmListPageSize {
		args := h.listReleasesArgs(selector, offset)
		out, err := h.runHelm(args, "list releases")
		if err != nil {
			return nil, err
		}
		releases := make([]helmListRelease, 0)
		if err := json.Unmarshal(out, &releases); err != nil {
			return nil, fmt.Errorf("parse helm release list: %w", err)
		}
		for _, release := range releases {
			if selector.Name != "" && release.Name != selector.Name {
				continue
			}
			if !strings.EqualFold(release.Status, "deployed") {
				continue
			}
			chartName, chartVersion, ok := splitHelmChart(release.Chart)
			if !ok || chartName != "higress" {
				continue
			}
			candidates = append(candidates, HelmReleaseCandidate{
				Name:         release.Name,
				Namespace:    release.Namespace,
				ChartName:    chartName,
				ChartVersion: chartVersion,
				AppVersion:   release.AppVersion,
				Status:       release.Status,
			})
		}
		if len(releases) < helmListPageSize {
			return candidates, nil
		}
	}
}

func (h *HelmAgent) listReleasesArgs(selector HelmReleaseSelector, offset int) []string {
	args := []string{"list", "-A", "-o", "json"}
	if selector.Namespace != "" {
		args = []string{"list", "-n", selector.Namespace, "-o", "json"}
	}
	args = append(args, "--max", strconv.Itoa(helmListPageSize), "--offset", strconv.Itoa(offset))
	return h.appendKubeFlags(args)
}

func (h *HelmAgent) GetRetainedValues(release HelmReleaseCandidate) (map[string]any, error) {
	args := []string{"get", "values", release.Name, "-n", release.Namespace, "-o", "json"}
	args = h.appendKubeFlags(args)
	out, err := h.runHelm(args, "get retained values")
	if err != nil {
		return nil, err
	}
	values := make(map[string]any)
	if len(bytes.TrimSpace(out)) == 0 {
		return values, nil
	}
	if err := json.Unmarshal(out, &values); err != nil {
		return nil, fmt.Errorf("parse retained Helm values for release %s/%s: %w", release.Namespace, release.Name, err)
	}
	return values, nil
}

func (h *HelmAgent) appendKubeFlags(args []string) []string {
	if len(*options.DefaultConfigFlags.KubeConfig) > 0 {
		args = append(args, fmt.Sprintf("--kubeconfig=%s", *options.DefaultConfigFlags.KubeConfig))
	}
	if len(*options.DefaultConfigFlags.Context) > 0 {
		args = append(args, fmt.Sprintf("--kube-context=%s", *options.DefaultConfigFlags.Context))
	}
	return args
}

func (h *HelmAgent) runHelm(args []string, action string) ([]byte, error) {
	cmd := exec.Command(h.helmBinaryName, args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start helm %s: %w", action, err)
	}
	if err := cmd.Wait(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("helm %s failed: %w: %s", action, err, message)
		}
		return nil, fmt.Errorf("helm %s failed: %w", action, err)
	}
	return out.Bytes(), nil
}

func splitHelmChart(chart string) (string, string, bool) {
	const prefix = "higress-"
	if !strings.HasPrefix(chart, prefix) {
		return "", "", false
	}
	version := strings.TrimPrefix(chart, prefix)
	if !isHelmChartVersion(version) {
		return "", "", false
	}
	return "higress", version, true
}

func isHelmChartVersion(version string) bool {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 3 {
		return false
	}
	for _, part := range parts[:2] {
		if part == "" || !allDigits(part) {
			return false
		}
	}
	patch := parts[2]
	if idx := strings.IndexAny(patch, "-+"); idx >= 0 {
		patch = patch[:idx]
	}
	return patch != "" && allDigits(patch)
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func (h *HelmAgent) IsHigressInstalled() (bool, error) {
	args := []string{"list", "-n", h.profile.Global.Namespace, "-f", "higress"}
	args = h.appendKubeFlags(args)
	if !h.quiet {
		fmt.Fprintf(h.writer, "\n📦 Running command: %s  %s\n\n", h.helmBinaryName, strings.Join(args, "  "))
	}
	cmd := exec.Command(h.helmBinaryName, args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("start helm ownership check: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return false, fmt.Errorf("helm ownership check failed: %w: %s", err, message)
		}
		return false, fmt.Errorf("helm ownership check failed: %w", err)
	}
	content := out.String()
	if !h.quiet {
		fmt.Fprintf(h.writer, "\n%s\n", content)
	}
	if strings.Contains(content, "deployed") {
		return true, nil
	}
	return false, nil
}
