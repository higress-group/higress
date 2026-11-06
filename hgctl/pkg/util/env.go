// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

package util

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func GetPythonVersion() (string, error) {
	re := regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

	for _, cmd := range []string{"python3", "python"} {
		out, err := exec.Command(cmd, "--version").CombinedOutput()
		if err != nil {
			continue
		}

		version := strings.TrimSpace(string(out))
		match := re.FindString(version)
		if match != "" {
			return match, nil
		}
	}

	return "", fmt.Errorf("python not found")
}

// compareVersions compares two version strings like "3.11.2" and "3.10".
// Returns:
//
//	 1  if v1 > v2
//	 0  if v1 == v2
//	-1  if v1 < v2
func CompareVersions(v1, v2 string) int {
	// Take the leading dotted version only (e.g. "3.12.0b1" → "3.12.0"), so a
	// pre-release suffix is not read as an extra, higher component.
	re := regexp.MustCompile(`\d+(?:\.\d+)*`)
	nums1 := splitVersionParts(re.FindString(v1))
	nums2 := splitVersionParts(re.FindString(v2))

	maxLen := len(nums1)
	if len(nums2) > maxLen {
		maxLen = len(nums2)
	}

	// Compare each part
	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(nums1) {
			n1, _ = strconv.Atoi(nums1[i])
		}
		if i < len(nums2) {
			n2, _ = strconv.Atoi(nums2[i])
		}

		if n1 > n2 {
			return 1
		} else if n1 < n2 {
			return -1
		}
	}

	return 0
}

// splitVersionParts turns "3.12.0" into its dotted parts; an input without any
// digits yields no parts, which compares as zero everywhere.
func splitVersionParts(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ".")
}
