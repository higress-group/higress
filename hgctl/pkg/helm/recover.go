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

package helm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/util"
	"sigs.k8s.io/yaml"
)

const (
	RecoverySeverityWarning = "warning"
	RecoverySeverityError   = "error"

	RecoveryDiagnosticBestEffort          = "best-effort-source-version"
	RecoveryDiagnosticUnsupportedValue    = "unsupported-retained-value"
	RecoveryDiagnosticAmbiguousInstall    = "ambiguous-install-mode"
	RecoveryDiagnosticInvalidRetainedType = "invalid-retained-value-type"
)

// HelmReleaseCandidate is structured Helm release metadata used by recovered
// upgrade mode. ChartName is the parent chart identity, not a dependency name.
type HelmReleaseCandidate struct {
	Name         string
	Namespace    string
	ChartName    string
	ChartVersion string
	AppVersion   string
	Status       string
}

type CompatibilityClass string

const (
	CompatibilityGuaranteed CompatibilityClass = "guaranteed"
	CompatibilityBestEffort CompatibilityClass = "best-effort"
)

type RecoveryDiagnostic struct {
	Severity string
	Code     string
	Path     string
	Message  string
}

func (d RecoveryDiagnostic) String() string {
	if d.Path == "" {
		return fmt.Sprintf("%s: %s", d.Code, d.Message)
	}
	return fmt.Sprintf("%s at %s: %s", d.Code, d.Path, d.Message)
}

// ClassifyRecoveredHelmSourceVersion returns whether the source chart falls in
// the reviewed support window. Exact Higress releases outside the window remain
// candidates and are handled best-effort by callers.
func ClassifyRecoveredHelmSourceVersion(version string) CompatibilityClass {
	v, ok := parseChartVersion(version)
	if !ok || v.prerelease {
		return CompatibilityBestEffort
	}
	if compareVersion(v, chartVersion{major: 2, minor: 1, patch: 0}) >= 0 &&
		compareVersion(v, chartVersion{major: 2, minor: 3, patch: 0}) < 0 {
		return CompatibilityGuaranteed
	}
	return CompatibilityBestEffort
}

// ReconstructProfileFromHelm converts one selected Helm release and retained
// user values into an ephemeral Profile. It never logs, persists, renders, or
// mutates external state.
func ReconstructProfileFromHelm(release HelmReleaseCandidate, retainedValues map[string]any) (*Profile, []RecoveryDiagnostic, error) {
	diagnostics := make([]RecoveryDiagnostic, 0)
	if ClassifyRecoveredHelmSourceVersion(release.ChartVersion) == CompatibilityBestEffort {
		diagnostics = append(diagnostics, RecoveryDiagnostic{
			Severity: RecoverySeverityWarning,
			Code:     RecoveryDiagnosticBestEffort,
			Message:  fmt.Sprintf("source chart version %q is outside the guaranteed >=2.1.0,<2.3.0 recovery window; continuing only as best effort", release.ChartVersion),
		})
	}

	profileName, err := recoveredProfileName(retainedValues)
	if err != nil {
		return nil, diagnostics, err
	}
	profileYAML, err := GetProfileYAML("", profileName)
	if err != nil {
		return nil, diagnostics, err
	}
	profile, err := UnmarshalProfile(profileYAML)
	if err != nil {
		return nil, diagnostics, fmt.Errorf("load recovered profile baseline %q: %w", profileName, err)
	}
	profile.Global.Namespace = release.Namespace
	profile.Values = deepCopyMap(retainedValues)
	profile.Charts.Higress.Name = "higress"
	if profile.Profile == "" {
		profile.Profile = profileName
	}

	for _, path := range unsupportedRetainedValuePaths(profile.Values) {
		diagnostics = append(diagnostics, RecoveryDiagnostic{
			Severity: RecoverySeverityWarning,
			Code:     RecoveryDiagnosticUnsupportedValue,
			Path:     path,
			Message:  "retained value is preserved as raw Helm input because hgctl has no typed recovery mapping for it",
		})
	}

	if err := NormalizeRecoveredProfileValues(profile); err != nil {
		return nil, diagnostics, err
	}
	profile.Global.Namespace = release.Namespace
	return profile, diagnostics, nil
}

// OverlayRecoveredProfile applies one already-normalized Profile overlay and
// immediately reconciles typed/raw ownership.
func OverlayRecoveredProfile(profile *Profile, overlayYAML string) error {
	if strings.TrimSpace(overlayYAML) == "" {
		return NormalizeRecoveredProfileValues(profile)
	}
	outYAML, err := util.OverlayYAML(util.ToYAML(profile), overlayYAML)
	if err != nil {
		return fmt.Errorf("could not overlay recovered profile: %w", err)
	}
	next, err := unmarshalRecoveredProfile(outYAML)
	if err != nil {
		return err
	}
	*profile = *next
	return NormalizeRecoveredProfileValues(profile)
}

func unmarshalRecoveredProfile(profileYAML string) (*Profile, error) {
	profile := &Profile{}
	if err := yaml.Unmarshal([]byte(profileYAML), profile); err != nil {
		return nil, fmt.Errorf("invalid recovered profile overlay")
	}
	return profile, nil
}

// NormalizeRecoveredProfileValues keeps the typed Profile field and equivalent
// raw Helm values from competing during ValuesYaml rendering. The raw path is
// consumed into the typed field and then removed; unowned raw paths survive.
func NormalizeRecoveredProfileValues(profile *Profile) error {
	if profile.Values == nil {
		profile.Values = make(map[string]any)
	}
	for _, owner := range recoveredOwnership {
		value, ok := getNested(profile.Values, owner.helmPath)
		if !ok {
			continue
		}
		if err := owner.setTyped(profile, value); err != nil {
			return fmt.Errorf("recover %s: %w", owner.helmPath, err)
		}
		deleteNested(profile.Values, owner.helmPath)
	}
	deleteEmptyMaps(profile.Values)
	return nil
}

type recoveredValueOwner struct {
	helmPath string
	setTyped func(*Profile, any) error
}

var recoveredOwnership = []recoveredValueOwner{
	{helmPath: "global.ingressClass", setTyped: func(p *Profile, v any) error {
		s, ok := v.(string)
		if !ok {
			return invalidType("string")
		}
		p.Global.IngressClass = s
		return nil
	}},
	{helmPath: "global.namespace", setTyped: func(p *Profile, v any) error {
		s, ok := v.(string)
		if !ok {
			return invalidType("string")
		}
		p.Global.Namespace = s
		return nil
	}},
	{helmPath: "global.enableIstioAPI", setTyped: func(p *Profile, v any) error {
		b, ok := v.(bool)
		if !ok {
			return invalidType("bool")
		}
		p.Global.EnableIstioAPI = b
		return nil
	}},
	{helmPath: "global.enableGatewayAPI", setTyped: func(p *Profile, v any) error {
		b, ok := v.(bool)
		if !ok {
			return invalidType("bool")
		}
		p.Global.EnableGatewayAPI = b
		return nil
	}},
	{helmPath: "global.local", setTyped: func(p *Profile, v any) error {
		b, ok := v.(bool)
		if !ok {
			return invalidType("bool")
		}
		if b {
			p.Global.Install = InstallLocalK8s
			p.Profile = "local-k8s"
		} else {
			p.Global.Install = InstallK8s
			p.Profile = "k8s"
		}
		return nil
	}},
	{helmPath: "global.kind", setTyped: func(p *Profile, v any) error {
		b, ok := v.(bool)
		if !ok {
			return invalidType("bool")
		}
		if b {
			p.Global.Install = InstallLocalK8s
			p.Profile = "local-k8s"
		} else {
			p.Global.Install = InstallK8s
			p.Profile = "k8s"
		}
		return nil
	}},
	{helmPath: "higress-console.replicaCount", setTyped: func(p *Profile, v any) error {
		n, err := uint32Value(v)
		if err != nil {
			return err
		}
		p.Console.Replicas = n
		return nil
	}},
	{helmPath: "higress-console.o11y.enabled", setTyped: func(p *Profile, v any) error {
		b, ok := v.(bool)
		if !ok {
			return invalidType("bool")
		}
		p.Console.O11yEnabled = b
		return nil
	}},
	{helmPath: "higress-core.gateway.replicas", setTyped: func(p *Profile, v any) error {
		n, err := uint32Value(v)
		if err != nil {
			return err
		}
		p.Gateway.Replicas = n
		return nil
	}},
	{helmPath: "higress-core.controller.replicas", setTyped: func(p *Profile, v any) error {
		n, err := uint32Value(v)
		if err != nil {
			return err
		}
		p.Controller.Replicas = n
		return nil
	}},
	{helmPath: "higress-core.controller.resources.requests.cpu", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Controller.Resources.Requests.CPU)
	}},
	{helmPath: "higress-core.controller.resources.requests.memory", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Controller.Resources.Requests.Memory)
	}},
	{helmPath: "higress-core.controller.resources.limits.cpu", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Controller.Resources.Limits.CPU)
	}},
	{helmPath: "higress-core.controller.resources.limits.memory", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Controller.Resources.Limits.Memory)
	}},
	{helmPath: "higress-core.gateway.resources.requests.cpu", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Gateway.Resources.Requests.CPU)
	}},
	{helmPath: "higress-core.gateway.resources.requests.memory", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Gateway.Resources.Requests.Memory)
	}},
	{helmPath: "higress-core.gateway.resources.limits.cpu", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Gateway.Resources.Limits.CPU)
	}},
	{helmPath: "higress-core.gateway.resources.limits.memory", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Gateway.Resources.Limits.Memory)
	}},
	{helmPath: "higress-console.resources.requests.cpu", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Console.Resources.Requests.CPU)
	}},
	{helmPath: "higress-console.resources.requests.memory", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Console.Resources.Requests.Memory)
	}},
	{helmPath: "higress-console.resources.limits.cpu", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Console.Resources.Limits.CPU)
	}},
	{helmPath: "higress-console.resources.limits.memory", setTyped: func(p *Profile, v any) error {
		return setString(v, &p.Console.Resources.Limits.Memory)
	}},
}

func recoveredProfileName(values map[string]any) (string, error) {
	local, hasLocal, err := boolAt(values, "global.local")
	if err != nil {
		return "", fmt.Errorf("%s at global.local: expected bool", RecoveryDiagnosticInvalidRetainedType)
	}
	kind, hasKind, err := boolAt(values, "global.kind")
	if err != nil {
		return "", fmt.Errorf("%s at global.kind: expected bool", RecoveryDiagnosticInvalidRetainedType)
	}
	if hasLocal && hasKind && local != kind {
		return "", fmt.Errorf("%s: global.local and global.kind disagree", RecoveryDiagnosticAmbiguousInstall)
	}
	if hasLocal {
		if local {
			return "local-k8s", nil
		}
		return "k8s", nil
	}
	if hasKind && kind {
		return "local-k8s", nil
	}
	return "k8s", nil
}

func unsupportedRetainedValuePaths(values map[string]any) []string {
	owned := make(map[string]struct{}, len(recoveredOwnership))
	for _, owner := range recoveredOwnership {
		owned[owner.helmPath] = struct{}{}
	}
	paths := make([]string, 0)
	collectLeafPaths(values, "", owned, &paths)
	return paths
}

func collectLeafPaths(value any, prefix string, owned map[string]struct{}, paths *[]string) {
	m, ok := value.(map[string]any)
	if !ok {
		if _, isOwned := owned[prefix]; !isOwned && prefix != "" {
			*paths = append(*paths, prefix)
		}
		return
	}
	for k, v := range m {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		collectLeafPaths(v, path, owned, paths)
	}
}

func boolAt(values map[string]any, path string) (bool, bool, error) {
	v, ok := getNested(values, path)
	if !ok {
		return false, false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, true, invalidType("bool")
	}
	return b, true, nil
}

func getNested(values map[string]any, path string) (any, bool) {
	cur := any(values)
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func deleteNested(values map[string]any, path string) {
	parts := strings.Split(path, ".")
	cur := values
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		if !ok {
			return
		}
		cur = next
	}
	delete(cur, parts[len(parts)-1])
}

func deleteEmptyMaps(values map[string]any) bool {
	for key, value := range values {
		if child, ok := value.(map[string]any); ok && deleteEmptyMaps(child) {
			delete(values, key)
		}
	}
	return len(values) == 0
}

func deepCopyMap(in map[string]any) map[string]any {
	if in == nil {
		return make(map[string]any)
	}
	outBytes, err := yaml.Marshal(in)
	if err != nil {
		return make(map[string]any)
	}
	out := make(map[string]any)
	if err := yaml.Unmarshal(outBytes, &out); err != nil {
		return make(map[string]any)
	}
	return out
}

func invalidType(want string) error {
	return fmt.Errorf("%s: expected %s", RecoveryDiagnosticInvalidRetainedType, want)
}

func uint32Value(v any) (uint32, error) {
	switch t := v.(type) {
	case int:
		if t < 0 {
			return 0, invalidType("non-negative integer")
		}
		return uint32(t), nil
	case int64:
		if t < 0 {
			return 0, invalidType("non-negative integer")
		}
		return uint32(t), nil
	case uint64:
		return uint32(t), nil
	case float64:
		if t < 0 || t != float64(uint32(t)) {
			return 0, invalidType("non-negative integer")
		}
		return uint32(t), nil
	case string:
		n, err := strconv.ParseUint(t, 10, 32)
		if err != nil {
			return 0, invalidType("non-negative integer")
		}
		return uint32(n), nil
	default:
		return 0, invalidType("non-negative integer")
	}
}

func setString(v any, out *string) error {
	s, ok := v.(string)
	if !ok {
		return invalidType("string")
	}
	*out = s
	return nil
}

type chartVersion struct {
	major      int
	minor      int
	patch      int
	prerelease bool
}

func parseChartVersion(version string) (chartVersion, bool) {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	prerelease := false
	if idx := strings.Index(version, "+"); idx >= 0 {
		version = version[:idx]
	}
	if idx := strings.Index(version, "-"); idx >= 0 {
		prerelease = true
		version = version[:idx]
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return chartVersion{}, false
	}
	nums := []int{0, 0, 0}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return chartVersion{}, false
		}
		nums[i] = n
	}
	return chartVersion{major: nums[0], minor: nums[1], patch: nums[2], prerelease: prerelease}, true
}

func compareVersion(a, b chartVersion) int {
	switch {
	case a.major != b.major:
		return a.major - b.major
	case a.minor != b.minor:
		return a.minor - b.minor
	default:
		return a.patch - b.patch
	}
}
