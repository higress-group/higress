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

package util

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// StripPrefix removes the given prefix from prefix.
func StripPrefix(path, prefix string) string {
	pl := len(strings.Split(prefix, "/"))
	pv := strings.Split(path, "/")
	return strings.Join(pv[pl:], "/")
}

func SplitSetFlag(flag string) (string, string) {
	// Split after the first '=' only, so a value that contains '=' is kept.
	items := strings.SplitN(flag, "=", 2)
	if len(items) != 2 {
		return flag, ""
	}
	return strings.TrimSpace(items[0]), strings.TrimSpace(items[1])
}

// IsFilePath reports whether the given URL is a local file path.
func IsFilePath(path string) bool {
	return strings.Contains(path, "/") || strings.Contains(path, ".")
}

// IsHTTPURL checks whether the given URL is a HTTP URL.
func IsHTTPURL(path string) (bool, error) {
	u, err := url.Parse(path)
	valid := err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
	if strings.HasPrefix(path, "http") && !valid {
		return false, fmt.Errorf("%s starts with http but is not a valid URL: %s", path, err)
	}
	return valid, nil
}

// StringBoolMapToSlice creates and returns a slice of all the map keys with true.
func StringBoolMapToSlice(m map[string]bool) []string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			s = append(s, k)
		}
	}
	return s
}

// ParseValue parses string into a value
func ParseValue(valueStr string) any {
	var value any
	if v, err := strconv.Atoi(valueStr); err == nil {
		value = v
	} else if v, err := strconv.ParseFloat(valueStr, 64); err == nil {
		value = v
	} else if v, err := strconv.ParseBool(valueStr); err == nil {
		value = v
	} else {
		value = strings.ReplaceAll(valueStr, "\\,", ",")
	}
	return value
}

// WriteFileString write string content to file
func WriteFileString(fileName string, content string, perm os.FileMode) error {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString(content); err != nil {
		return err
	}
	writer.Flush()
	return nil
}

// This function return ~/.hgctl file_path string (Currently Linux only)
func GetHomeHgctlDir() string {
	homeDir, _ := os.UserHomeDir()
	targetDir := filepath.Join(homeDir, ".hgctl")
	return targetDir
}

func GetSpecificAgentDir(name string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	targetDir := filepath.Join(homeDir, ".hgctl", "agents", name)

	info, err := os.Stat(targetDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("agent %q does not exist", name)
		}
		return "", fmt.Errorf("failed to stat agent directory %q: %w", targetDir, err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("agent %q exists but is not a directory", name)
	}

	return targetDir, nil
}
