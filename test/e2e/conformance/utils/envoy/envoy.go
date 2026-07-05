/*
Copyright 2022 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package envoy

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/alibaba/higress/hgctl/cmd/hgctl/config"
	cfg "github.com/alibaba/higress/v2/test/e2e/conformance/utils/config"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
)

type CheckType string

const (
	// CheckTypeMatch checks if the actual value matches the expected value.
	CheckTypeMatch CheckType = "match"
	// CheckTypeExist checks if the actual value exists.
	CheckTypeExist CheckType = "exist"
	// CheckTypeNotExist checks if the actual value does not exist.
	CheckTypeNotExist CheckType = "notexist"

	// defaultSuccessThreshold is the default number of times the assertion must succeed in a row.
	defaultSuccessThreshold = 3
)

// Assertion defines the assertion to be made on the Envoy config.
// TODO: It can support localization judgment so that this configuration check function will be more universal.
// TODO: Can be used for general e2e tests, rather than just envoy filter scenarios.
type Assertion struct {
	// Path is the path of gjson to the value to be asserted.
	Path string
	// CheckType is the type of assertion to be made.
	CheckType CheckType
	// ExpectEnvoyConfig is the expected value of the Envoy config.
	ExpectEnvoyConfig map[string]interface{}
	// TargetNamespace is the namespace of the Envoy pod.
	TargetNamespace string
}

// AssertEnvoyConfig asserts the Envoy config.
func AssertEnvoyConfig(t *testing.T, timeoutConfig cfg.TimeoutConfig, expected Assertion) {
	options := config.NewDefaultGetEnvoyConfigOptions()
	options.PodNamespace = expected.TargetNamespace
	convertEnvoyConfig := convertNumbersToFloat64(expected.ExpectEnvoyConfig)
	if _, ok := convertEnvoyConfig.(map[string]interface{}); !ok {
		t.Errorf("failed to convert envoy config number to float64")
		return
	}
	expected.ExpectEnvoyConfig = convertEnvoyConfig.(map[string]interface{})
	waitForEnvoyConfig(t, timeoutConfig, options, expected)
}

func convertNumbersToFloat64(data interface{}) interface{} {
	switch val := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, v := range val {
			result[key] = convertNumbersToFloat64(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = convertNumbersToFloat64(v)
		}
		return result
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float32:
		return float64(val)
	default:
		return data
	}
}

// waitForEnvoyConfig waits for the Envoy config to be ready and asserts it.
func waitForEnvoyConfig(t *testing.T, timeoutConfig cfg.TimeoutConfig, options *config.GetEnvoyConfigOptions, expected Assertion) {
	awaitConvergence(t, defaultSuccessThreshold, timeoutConfig.MaxTimeToConsistency, func(elapsed time.Duration) bool {
		allEnvoyConfig := ""
		err := wait.Poll(1*time.Second, 10*time.Second, func() (bool, error) {
			out, err := config.GetEnvoyConfig(options)
			if err != nil {
				return false, err
			}
			allEnvoyConfig = string(out)
			return true, nil
		})
		if err != nil {
			return false
		}
		switch expected.CheckType {
		case CheckTypeMatch:
			err = assertEnvoyConfigMatch(t, allEnvoyConfig, expected)
		case CheckTypeExist:
			err = assertEnvoyConfigExist(t, allEnvoyConfig, expected)
		case CheckTypeNotExist:
			err = assertEnvoyConfigNotExist(t, allEnvoyConfig, expected)
		default:
			err = fmt.Errorf("unsupported check type %s", expected.CheckType)
		}
		if err != nil {
			return false
		}
		return true
	})
	t.Logf("✅ Envoy config checked")
}

// assertEnvoyConfigNotExist asserts the Envoy config does not exist.
func assertEnvoyConfigNotExist(t *testing.T, envoyConfig string, expected Assertion) error {
	result := gjson.Get(envoyConfig, expected.Path).Value()
	if result == nil {
		return nil
	}
	if !findMustNotExist(t, result, expected.ExpectEnvoyConfig) {
		return fmt.Errorf("the expected value %s exists in path '%s'", expected.ExpectEnvoyConfig, expected.Path)
	}
	return nil
}

// assertEnvoyConfigExist asserts the Envoy config exists.
func assertEnvoyConfigExist(t *testing.T, envoyConfig string, expected Assertion) error {
	result := gjson.Get(envoyConfig, expected.Path).Value()
	if result == nil {
		return fmt.Errorf("failed to get value from path '%s'", expected.Path)
	}
	if !findMustExist(t, result, expected.ExpectEnvoyConfig) {
		return fmt.Errorf("the expected value %s does not exist in path '%s'", expected.ExpectEnvoyConfig, expected.Path)
	}
	return nil
}

// assertEnvoyConfigMatch asserts the Envoy config matches the expected value.
func assertEnvoyConfigMatch(t *testing.T, envoyConfig string, expected Assertion) error {
	result := gjson.Get(envoyConfig, expected.Path).Value()
	if result == nil {
		return fmt.Errorf("failed to get value from path '%s'", expected.Path)
	}
	if !match(t, result, expected.ExpectEnvoyConfig) {
		return fmt.Errorf("failed to match value from path '%s'", expected.Path)
	}
	t.Logf("✅ Matched value %s in path '%s'", expected.ExpectEnvoyConfig, expected.Path)
	return nil
}

// awaitConvergence runs the given function until it returns 'true' `threshold` times in a row.
// Each failed attempt has a 1s delay; successful attempts have no delay.
func awaitConvergence(t *testing.T, threshold int, maxTimeToConsistency time.Duration, fn func(elapsed time.Duration) bool) {
	successes := 0
	attempts := 0
	start := time.Now()
	to := time.After(maxTimeToConsistency)
	delay := time.Second
	for {
		select {
		case <-to:
			t.Fatalf("timeout while waiting after %d attempts", attempts)
		default:
		}

		completed := fn(time.Now().Sub(start))
		attempts++
		if completed {
			successes++
			if successes >= threshold {
				return
			}
			// Skip delay if we have a success
			continue
		}

		successes = 0
		select {
		// Capture the overall timeout
		case <-to:
			t.Fatalf("timeout while waiting after %d attempts, %d/%d successes", attempts, successes, threshold)
			// And the per-try delay
		case <-time.After(delay):
		}
	}
}

// match
// 1. interface{} is a slice: if one of the slice elements matches, the assertion passes
// Notice: can recursively find slices
// 2. interface{} is a map: if all the map elements match, the assertion passes
// 3. interface{} is a field: if the field matches, the assertion passes
func match(t *testing.T, actual interface{}, expected map[string]interface{}) bool {
	reflectValue := reflect.ValueOf(actual)
	kind := reflectValue.Kind()
	switch kind {
	case reflect.Slice:
		actualValueSlice := actual.([]interface{})
		for _, v := range actualValueSlice {
			if match(t, v, expected) {
				return true
			}
		}
		return false
	case reflect.Map:
		actualValueMap := actual.(map[string]interface{})
		for key, expectValue := range expected {
			actualValue, ok := actualValueMap[key]
			if !ok {
				return false
			}
			if !reflect.DeepEqual(actualValue, expectValue) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(actual, expected)
	}
}

// findMustExist finds the value of the given path in the given Envoy config.
func findMustExist(t *testing.T, actual interface{}, expected map[string]interface{}) bool {
	for key, expectValue := range expected {
		// If the key does not exist, the assertion fails.
		t.Logf("🔍 Finding key %s", key)
		if !findKey(actual, key, expectValue) {
			t.Logf("❌ Not found key %s", key)
			return false
		}
		t.Logf("✅ Found key %s", key)
	}
	return true
}

// findMustNotExist finds the value of the given path in the given Envoy config.
func findMustNotExist(t *testing.T, actual interface{}, expected map[string]interface{}) bool {
	for key, expectValue := range expected {
		// If the key exists, the assertion fails.
		t.Logf("🔍 Finding key %s", key)
		if findKey(actual, key, expectValue) {
			t.Logf("❌ Found key %s", key)
			return false
		}
		t.Logf("✅ Not found key %s", key)
	}
	return true
}

// findKey finds the value of the given key in the given Envoy config.
func findKey(actual interface{}, key string, expectValue interface{}) bool {
	reflectValue := reflect.ValueOf(actual)
	kind := reflectValue.Kind()
	switch kind {
	case reflect.Slice:
		actualValueSlice := actual.([]interface{})
		for _, v := range actualValueSlice {
			if findKey(v, key, expectValue) {
				return true
			}
		}
		return false
	case reflect.Map:
		actualValueMap := actual.(map[string]interface{})
		for actualKey, actualValue := range actualValueMap {
			if actualKey == key && reflect.DeepEqual(actualValue, expectValue) {
				return true
			}
			if findKey(actualValue, key, expectValue) {
				return true
			}
		}
		return false
	default:
		if reflectValue.String() == key && reflect.DeepEqual(actual, expectValue) {
			return true
		}
		return false
	}
}
