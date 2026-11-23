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
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/helm/tpath"
	"github.com/alibaba/higress/hgctl/pkg/util"
	"sigs.k8s.io/yaml"
)

// GetProfileFromFlags get profile name from flags.
func GetProfileFromFlags(setFlags []string) (string, error) {
	profileName := DefaultProfileName
	// The profile coming from --set flag has the highest precedence.
	psf := GetValueForSetFlag(setFlags, "profile")
	if psf != "" {
		profileName = psf
	}
	return profileName, nil
}

func GetValuesOverylayFromFiles(inFilenames []string) (string, error) {
	// Convert layeredYamls under values node in profile file to support helm values
	overLayYamls := ""
	// Get Overlays from files
	if len(inFilenames) > 0 {
		layeredYamls, err := ReadLayeredYAMLs(inFilenames)
		if err != nil {
			return "", err
		}
		vals := make(map[string]any)
		if err := yaml.Unmarshal([]byte(layeredYamls), &vals); err != nil {
			return "", fmt.Errorf("%s:\n\nYAML:\n%s", err, layeredYamls)
		}
		values := make(map[string]any)
		values["values"] = vals
		out, err := yaml.Marshal(values)
		if err != nil {
			return "", err
		}
		overLayYamls = string(out)
	}

	return overLayYamls, nil
}

func GetUninstallProfileName() string {
	return DefaultUninstallProfileName
}

func ReadLayeredYAMLs(filenames []string) (string, error) {
	return readLayeredYAMLs(filenames, os.Stdin)
}

func readLayeredYAMLs(filenames []string, stdinReader io.Reader) (string, error) {
	var ly string
	var stdin bool
	for _, fn := range filenames {
		var b []byte
		var err error
		if fn == "-" {
			if stdin {
				continue
			}
			stdin = true
			b, err = io.ReadAll(stdinReader)
		} else {
			b, err = os.ReadFile(strings.TrimSpace(fn))
		}
		if err != nil {
			return "", err
		}

		ly, err = util.OverlayYAML(ly, string(b))
		if err != nil {
			return "", err
		}
	}
	return ly, nil
}

// GetValueForSetFlag parses the passed set flags which have format key=value and if any set the given path,
// returns the corresponding value, otherwise returns the empty string. setFlags must have valid format.
func GetValueForSetFlag(setFlags []string, path string) string {
	ret := ""
	for _, sf := range setFlags {
		p, v := getPV(sf)
		if p == path {
			ret = v
		}
		// if set multiple times, return last set value
	}
	return ret
}

// getPV returns the path and value components for the given set flag string, which must be in path=value format.
func getPV(setFlag string) (path string, value string) {
	pv := strings.Split(setFlag, "=")
	if len(pv) != 2 {
		return setFlag, ""
	}
	path, value = strings.TrimSpace(pv[0]), strings.TrimSpace(pv[1])
	return
}

func GenerateConfig(inFilenames []string, setFlags []string) (string, *Profile, string, error) {
	if err := validateSetFlags(setFlags); err != nil {
		return "", nil, "", err
	}

	profileName, err := GetProfileFromFlags(setFlags)
	if err != nil {
		return "", nil, "", err
	}

	valuesOverlay, err := GetValuesOverylayFromFiles(inFilenames)
	if err != nil {
		return "", nil, "", err
	}

	profileString, profile, err := GenProfile(profileName, valuesOverlay, setFlags)
	if err != nil {
		return "", nil, "", err
	}

	return profileString, profile, profileName, nil
}

// validateSetFlags validates that setFlags all have path=value format.
func validateSetFlags(setFlags []string) error {
	for _, sf := range setFlags {
		pv := strings.Split(sf, "=")
		if len(pv) != 2 {
			return fmt.Errorf("set flag %s has incorrect format, must be path=value", sf)
		}
	}
	return nil
}

func overlaySetFlagValues(iopYAML string, setFlags []string) (string, error) {
	iop := make(map[string]any)
	if err := yaml.Unmarshal([]byte(iopYAML), &iop); err != nil {
		return "", err
	}
	// Unmarshal returns nil for empty manifests but we need something to insert into.
	if iop == nil {
		iop = make(map[string]any)
	}

	for _, sf := range setFlags {
		p, v := getPV(sf)
		inc, _, err := tpath.GetPathContext(iop, util.PathFromString(p), true)
		if err != nil {
			return "", err
		}
		// input value type is always string, transform it to correct type before setting.
		if err := tpath.WritePathContext(inc, util.ParseValue(v), false); err != nil {
			return "", err
		}
	}

	out, err := yaml.Marshal(iop)
	if err != nil {
		return "", err
	}

	return string(out), nil
}

// GetProfileOverlay normalizes file and --set input into one Profile overlay.
// Callers that need to validate user intent must do so before Profile defaulting.
func GetProfileOverlay(fileOverlayYAML string, setFlags []string) (string, error) {
	return overlaySetFlagValues(fileOverlayYAML, setFlags)
}

// UnsupportedLocalDockerUpgradeOverlayPaths returns caller-requested fields that
// would not be consumed by a local-docker upgrade. The comparison uses the
// stored typed Profile as the baseline, while the overlay retains only caller
// input and therefore excludes fields injected by profile generation.
func UnsupportedLocalDockerUpgradeOverlayPaths(baseline *Profile, overlayYAML string) ([]string, error) {
	if strings.TrimSpace(overlayYAML) == "" {
		return nil, nil
	}

	baselineValues := make(map[string]any)
	if err := yaml.Unmarshal([]byte(util.ToYAML(baseline)), &baselineValues); err != nil {
		return nil, err
	}
	overlayValues := make(map[string]any)
	if err := yaml.Unmarshal([]byte(overlayYAML), &overlayValues); err != nil {
		return nil, err
	}

	paths := make([]string, 0)
	collectOverlayDiffPaths(baselineValues, overlayValues, "", &paths)
	sort.Strings(paths)

	unsupported := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != "installPackagePath" && path != "charts.standalone.url" {
			unsupported = append(unsupported, path)
		}
	}
	if len(unsupported) == 0 {
		return nil, nil
	}
	return unsupported, nil
}

func collectOverlayDiffPaths(baseline, overlay any, prefix string, paths *[]string) {
	overlayMap, isMap := overlay.(map[string]any)
	if !isMap {
		if !reflect.DeepEqual(baseline, overlay) {
			*paths = append(*paths, prefix)
		}
		return
	}

	baselineMap, _ := baseline.(map[string]any)
	keys := make([]string, 0, len(overlayMap))
	for key := range overlayMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		collectOverlayDiffPaths(baselineMap[key], overlayMap[key], path, paths)
	}
}

// getInstallPackagePath returns the installPackagePath in the given IstioOperator YAML string.
func getInstallPackagePath(profileYAML string) (string, error) {
	profile, err := UnmarshalProfile(profileYAML)
	if err != nil {
		return "", err
	}
	if profile == nil {
		return "", nil
	}
	return profile.InstallPackagePath, nil
}

// GetProfileYAML returns the YAML for the given profile name, using the given profileOrPath string, which may be either
// a profile label or a file path.
func GetProfileYAML(installPackagePath, profileOrPath string) (string, error) {
	if profileOrPath == "" {
		profileOrPath = DefaultProfileFilename
	}
	profiles, err := readProfiles(installPackagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read profiles: %v", err)
	}
	// If charts are a file path and profile is a name like default, transform it to the file path.
	if profiles[profileOrPath] && installPackagePath != "" {
		profileOrPath = filepath.Join(installPackagePath, "profiles", profileOrPath+".yaml")
	}
	// This contains the IstioOperator CR.
	baseCRYAML, err := ReadProfileYAML(profileOrPath, installPackagePath)
	if err != nil {
		return "", err
	}

	//if !IsDefaultProfile(profileOrPath) {
	//	// Profile definitions are relative to the default profileOrPath, so read that first.
	//	dfn := DefaultFilenameForProfile(profileOrPath)
	//	defaultYAML, err := ReadProfileYAML(dfn, installPackagePath)
	//	if err != nil {
	//		return "", err
	//	}
	//	baseCRYAML, err = util.OverlayYAML(defaultYAML, baseCRYAML)
	//	if err != nil {
	//		return "", err
	//	}
	//}
	return baseCRYAML, nil
}

// IsDefaultProfile reports whether the given profile is the default profile.
func IsDefaultProfile(profile string) bool {
	return profile == "" || profile == DefaultProfileName || filepath.Base(profile) == DefaultProfileFilename
}

// DefaultFilenameForProfile returns the profile name of the default profile for the given profile.
func DefaultFilenameForProfile(profile string) string {
	switch {
	case util.IsFilePath(profile):
		return filepath.Join(filepath.Dir(profile), DefaultProfileFilename)
	default:
		return DefaultProfileName
	}
}

// ReadProfileYAML reads the YAML values associated with the given profile. It uses an appropriate reader for the
// profile format (compiled-in, file, HTTP, etc.).
func ReadProfileYAML(profile, manifestsPath string) (string, error) {
	var err error
	var globalValues string

	// Get global values from profile.
	switch {
	case util.IsFilePath(profile):
		if globalValues, err = readFile(profile); err != nil {
			return "", err
		}
	default:
		if globalValues, err = LoadValues(profile, manifestsPath); err != nil {
			return "", fmt.Errorf("failed to read profile %v from %v: %v", profile, manifestsPath, err)
		}
	}

	return globalValues, nil
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

// UnmarshalProfile unmarshals a string containing Profile as YAML.
func UnmarshalProfile(profileYAML string) (*Profile, error) {
	profile := &Profile{}
	if err := yaml.Unmarshal([]byte(profileYAML), profile); err != nil {
		return nil, fmt.Errorf("%s:\n\nYAML:\n%s", err, profileYAML)
	}
	return profile, nil
}

// GenProfile generates an Profile from the given profile name or path, and overlay YAMLs from user
// files and the --set flag. If successful, it returns an Profile string and struct.
func GenProfile(profileOrPath, fileOverlayYAML string, setFlags []string) (string, *Profile, error) {
	installPackagePath, err := getInstallPackagePath(fileOverlayYAML)
	if err != nil {
		return "", nil, err
	}
	if sfp := GetValueForSetFlag(setFlags, "installPackagePath"); sfp != "" {
		// set flag installPackagePath has the highest precedence, if set.
		installPackagePath = sfp
	}

	// To generate the base profileOrPath for overlaying with user values, we need the installPackagePath where the profiles
	// can be found, and the selected profileOrPath. Both of these can come from either the user overlay file or --set flag.
	outYAML, err := GetProfileYAML(installPackagePath, profileOrPath)
	if err != nil {
		return "", nil, err
	}

	// Combine file and --set overlays and translate any K8s settings in values to Profile format
	overlayYAML, err := GetProfileOverlay(fileOverlayYAML, setFlags)
	if err != nil {
		return "", nil, err
	}
	// Merge user file and --set flags.
	outYAML, err = util.OverlayYAML(outYAML, overlayYAML)
	if err != nil {
		return "", nil, fmt.Errorf("could not overlay user config over base: %s", err)
	}

	finalProfile, err := UnmarshalProfile(outYAML)
	if err != nil {
		return "", nil, err
	}

	if len(installPackagePath) > 0 {
		finalProfile.InstallPackagePath = installPackagePath
	}

	if finalProfile.Profile == "" {
		finalProfile.Profile = DefaultProfileName
	}
	return util.ToYAML(finalProfile), finalProfile, nil
}

func GenProfileFromProfileContent(profileContent, fileOverlayYAML string, setFlags []string) (string, *Profile, error) {
	installPackagePath, err := getInstallPackagePath(fileOverlayYAML)
	if err != nil {
		return "", nil, err
	}
	if sfp := GetValueForSetFlag(setFlags, "installPackagePath"); sfp != "" {
		// set flag installPackagePath has the highest precedence, if set.
		installPackagePath = sfp
	}

	// Combine file and --set overlays and translate any K8s settings in values to Profile format
	overlayYAML, err := GetProfileOverlay(fileOverlayYAML, setFlags)
	if err != nil {
		return "", nil, err
	}
	// Merge user file and --set flags.
	outYAML, err := util.OverlayYAML(profileContent, overlayYAML)
	if err != nil {
		return "", nil, fmt.Errorf("could not overlay user config over base: %s", err)
	}

	finalProfile, err := UnmarshalProfile(outYAML)
	if err != nil {
		return "", nil, err
	}

	if len(installPackagePath) > 0 {
		finalProfile.InstallPackagePath = installPackagePath
	}

	if finalProfile.Profile == "" {
		finalProfile.Profile = DefaultProfileName
	}
	return util.ToYAML(finalProfile), finalProfile, nil
}
