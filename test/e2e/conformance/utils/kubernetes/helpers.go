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

package kubernetes

import (
	"context"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/config"
)

// FilterStaleConditions returns the list of status condition whose observedGeneration does not
// match the objects metadata.Generation
func FilterStaleConditions(obj metav1.Object, conditions []metav1.Condition) []metav1.Condition {
	stale := make([]metav1.Condition, 0, len(conditions))
	for _, condition := range conditions {
		if obj.GetGeneration() != condition.ObservedGeneration {
			stale = append(stale, condition)
		}
	}
	return stale
}

// NamespacesMustBeAccepted waits until all Pods are marked ready
// in the provided namespaces. This will cause the test to
// halt if the specified timeout is exceeded.
func NamespacesMustBeAccepted(t *testing.T, c client.Client, timeoutConfig config.TimeoutConfig, namespaces []string) {
	t.Helper()

	waitErr := wait.PollImmediate(1*time.Second, timeoutConfig.NamespacesMustBeReady, func() (bool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, ns := range namespaces {
			podList := &v1.PodList{}
			err := c.List(ctx, podList, client.InNamespace(ns))
			if err != nil {
				t.Errorf("❌ Error listing Pods: %v", err)
			}
			for _, pod := range podList.Items {
				if !FindPodConditionInList(t, pod.Status.Conditions, "Ready", "True") &&
					pod.Status.Phase != v1.PodSucceeded {
					t.Logf("%s/%s Pod not ready yet", ns, pod.Name)
					return false, nil
				}
			}
		}

		t.Logf("✅ Gateways and Pods in %s namespaces ready", strings.Join(namespaces, ", "))
		return true, nil
	})
	require.NoErrorf(t, waitErr, "error waiting for %s namespaces to be ready", strings.Join(namespaces, ", "))
}

func ConditionsMatch(t *testing.T, expected, actual []metav1.Condition) bool {
	if len(actual) < len(expected) {
		t.Logf("⌛️ Expected more conditions to be present")
		return false
	}
	for _, condition := range expected {
		if !FindConditionInList(t, actual, condition.Type, string(condition.Status), condition.Reason) {
			return false
		}
	}

	t.Logf("✅ Conditions matched expectations")
	return true
}

// findConditionInList finds a condition in a list of Conditions, checking
// the Name, Value, and Reason. If an empty reason is passed, any Reason will match.
func FindConditionInList(t *testing.T, conditions []metav1.Condition, condName, expectedStatus, expectedReason string) bool {
	for _, cond := range conditions {
		if cond.Type == condName {
			if cond.Status == metav1.ConditionStatus(expectedStatus) {
				// an empty Reason string means "Match any reason".
				if expectedReason == "" || cond.Reason == expectedReason {
					return true
				}
				t.Logf("⌛️ %s condition Reason set to %s, expected %s", condName, cond.Reason, expectedReason)
			}

			t.Logf("⌛️ %s condition set to Status %s with Reason %v, expected Status %s", condName, cond.Status, cond.Reason, expectedStatus)
		}
	}

	t.Logf("⌛️ %s was not in conditions list", condName)
	return false
}

func FindPodConditionInList(t *testing.T, conditions []v1.PodCondition, condName, condValue string) bool {
	for _, cond := range conditions {
		if cond.Type == v1.PodConditionType(condName) {
			if cond.Status == v1.ConditionStatus(condValue) {
				return true
			}
			t.Logf("⌛️ %s condition set to %s, expected %s", condName, cond.Status, condValue)
		}
	}

	t.Logf("⌛️ %s was not in conditions list", condName)
	return false
}

func ApplyConfigmapDataWithYaml(t *testing.T, c client.Client, namespace string, name string, key string, val any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cm := &v1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cm); err != nil {
		return err
	}
	y, err := yaml.Marshal(val)
	if err != nil {
		return err
	}
	data := string(y)

	if cm.Data == nil {
		cm.Data = make(map[string]string, 0)
	}
	cm.Data[key] = data

	t.Logf("🏗 Updating %s %s", name, namespace)
	return c.Update(ctx, cm)
}

func ApplySecret(t *testing.T, c client.Client, namespace string, name string, key string, val string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cm := &v1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cm); err != nil {
		return err
	}
	cm.Data[key] = []byte(val)
	t.Logf("🏗 Updating Secret %s %s", name, namespace)
	return c.Update(ctx, cm)
}
