//go:build scale

// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package scale

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	defaultIngressClass     = "higress"
	defaultBaseIngresses    = 200
	defaultRatioMax         = 3.0
	defaultEventMaxSeconds  = 60.0
	defaultNamespace        = "scale-test"
	eventsPerPhase          = 3
	backendName             = "scale-backend"
	backendImage            = "higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/httpbin:1.0.2"
	backendPort             = 80
	pollInterval            = 100 * time.Millisecond
	httpRequestTimeout      = 3 * time.Second
	setupTimeout            = 5 * time.Minute
	namespaceCleanupTimeout = 3 * time.Minute
)

type scaleConfig struct {
	address      string
	ingressClass string
	base         int
	ratioMax     float64
	eventMax     time.Duration
	namespace    string
}

type phaseResult struct {
	name           string
	baseIngresses  int
	totalIngresses int
	samples        []float64
	timedOut       bool
	err            error
}

// TestScaleIngressPropagation measures how long a newly-created Ingress takes
// to become routable at two cluster sizes. Doubling the number of Ingresses
// should not multiply the incremental propagation latency by roughly four.
func TestScaleIngressPropagation(t *testing.T) {
	cfg := loadScaleConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()

	restConfig, err := config.GetConfig()
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	// Creating several hundred Ingresses is setup work, not part of the
	// measured propagation latency. Avoid client-side throttling dominating it.
	restConfig.QPS = 50
	restConfig.Burst = 100

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("register core API scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("register apps API scheme: %v", err)
	}
	if err := networkingv1.AddToScheme(scheme); err != nil {
		t.Fatalf("register networking API scheme: %v", err)
	}

	kube, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("create Kubernetes client: %v", err)
	}

	if err := prepareNamespace(ctx, kube, cfg.namespace); err != nil {
		t.Fatalf("prepare namespace: %v", err)
	}
	defer cleanupNamespace(t, kube, cfg.namespace)

	if err := createBackend(ctx, kube, cfg.namespace); err != nil {
		t.Fatalf("create backend: %v", err)
	}
	if err := waitForDeployment(ctx, kube, cfg.namespace, backendName, setupTimeout); err != nil {
		t.Fatalf("wait for backend: %v", err)
	}

	t.Logf("scale config: address=%q ingressClass=%q base=%d ratioMax=%.2f eventMax=%s namespace=%q",
		cfg.address, cfg.ingressClass, cfg.base, cfg.ratioMax, cfg.eventMax, cfg.namespace)

	resultA := runPhase(ctx, t, kube, cfg, "A", "scale-a", cfg.base, cfg.base)
	if resultA.err != nil {
		reportPhaseFailure(t, cfg, resultA)
		t.FailNow()
	}

	resultB := runPhase(ctx, t, kube, cfg, "B", "scale-b", cfg.base, 2*cfg.base)
	evaluateResults(t, cfg, resultA, resultB)
}

func loadScaleConfig(t *testing.T) scaleConfig {
	t.Helper()

	cfg := scaleConfig{
		address:      strings.TrimSpace(os.Getenv("E2E_SCALE_ADDRESS")),
		ingressClass: envString("E2E_SCALE_INGRESS_CLASS", defaultIngressClass),
		base:         envInt(t, "E2E_SCALE_BASE", defaultBaseIngresses),
		ratioMax:     envFloat(t, "E2E_SCALE_RATIO_MAX", defaultRatioMax),
		namespace:    envString("E2E_SCALE_NAMESPACE", defaultNamespace),
	}
	cfg.eventMax = time.Duration(envFloat(t, "E2E_SCALE_EVENT_MAX_SECONDS", defaultEventMaxSeconds) * float64(time.Second))

	if cfg.address == "" {
		t.Fatal("E2E_SCALE_ADDRESS must be set to the Higress gateway address, for example 172.18.0.2:80")
	}
	if cfg.base < 1 {
		t.Fatalf("E2E_SCALE_BASE must be greater than zero, got %d", cfg.base)
	}
	if cfg.ratioMax <= 0 {
		t.Fatalf("E2E_SCALE_RATIO_MAX must be greater than zero, got %v", cfg.ratioMax)
	}
	if cfg.eventMax <= 0 {
		t.Fatalf("E2E_SCALE_EVENT_MAX_SECONDS must be greater than zero, got %s", cfg.eventMax)
	}
	if cfg.namespace == "" {
		t.Fatal("E2E_SCALE_NAMESPACE must not be empty")
	}

	return cfg
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(t *testing.T, key string, fallback int) int {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		t.Fatalf("%s must be a positive integer, got %q", key, raw)
	}
	return value
}

func envFloat(t *testing.T, key string, fallback float64) float64 {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		t.Fatalf("%s must be a positive number, got %q", key, raw)
	}
	return value
}

func runPhase(ctx context.Context, t *testing.T, kube client.Client, cfg scaleConfig, phase, hostPrefix string, addCount, total int) phaseResult {
	t.Helper()

	result := phaseResult{
		name:           phase,
		baseIngresses:  cfg.base,
		totalIngresses: total,
	}

	t.Logf("phase %s: creating %d ingresses, configured total is %d", phase, addCount, total)
	for i := 0; i < addCount; i++ {
		host := fmt.Sprintf("%s-%d.example.com", hostPrefix, i)
		ingress := newIngress(fmt.Sprintf("%s-%d", hostPrefix, i), host, cfg)
		if err := kube.Create(ctx, ingress); err != nil {
			result.err = fmt.Errorf("create ingress %q: %w", ingress.Name, err)
			return result
		}
	}

	warmupHost := fmt.Sprintf("%s-warmup.example.com", hostPrefix)
	warmup := newIngress(fmt.Sprintf("%s-warmup", hostPrefix), warmupHost, cfg)
	warmupStart := time.Now()
	if err := kube.Create(ctx, warmup); err != nil {
		result.err = fmt.Errorf("create warm-up ingress %q: %w", warmup.Name, err)
		return result
	}
	if err := waitForHost(ctx, cfg, warmupHost, cfg.eventMax); err != nil {
		result.timedOut = true
		result.err = fmt.Errorf("warm-up ingress did not propagate within %s: %w", cfg.eventMax, err)
		return result
	}
	t.Logf("phase %s: warm-up ingress propagated in %.3fs", phase, time.Since(warmupStart).Seconds())

	for i := 0; i < eventsPerPhase; i++ {
		host := fmt.Sprintf("%s-event-%d.example.com", hostPrefix, i)
		ingress := newIngress(fmt.Sprintf("%s-event-%d", hostPrefix, i), host, cfg)

		start := time.Now()
		if err := kube.Create(ctx, ingress); err != nil {
			result.err = fmt.Errorf("create measured ingress %q: %w", ingress.Name, err)
			return result
		}
		if err := waitForHost(ctx, cfg, host, cfg.eventMax); err != nil {
			elapsed := time.Since(start).Seconds()
			result.samples = append(result.samples, elapsed)
			result.timedOut = true
			result.err = fmt.Errorf("measured ingress %q did not propagate within %s: %w", ingress.Name, cfg.eventMax, err)
			return result
		}

		elapsed := time.Since(start).Seconds()
		result.samples = append(result.samples, elapsed)
		t.Logf("phase %s: event %d propagated in %.3fs (host=%s)", phase, i+1, elapsed, host)
	}

	return result
}

func newIngress(name, host string, cfg scaleConfig) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cfg.namespace,
			Labels: map[string]string{
				"app": "higress-scale-test",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPointer(cfg.ingressClass),
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: backendName,
											Port: networkingv1.ServiceBackendPort{Number: backendPort},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func stringPointer(value string) *string {
	return &value
}

func prepareNamespace(ctx context.Context, kube client.Client, namespace string) error {
	if isProtectedNamespace(namespace) {
		return fmt.Errorf("namespace %q is protected and must not be used by the scale test", namespace)
	}

	err := kube.Create(ctx, namespaceObject(namespace))
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %q: %w", namespace, err)
	}

	// A previous interrupted run may have left resources behind. Recreate the
	// dedicated test namespace so ingress names and hosts start from a clean state.
	if err := kube.Delete(ctx, namespaceObject(namespace)); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete stale namespace %q: %w", namespace, err)
	}
	if err := waitForNamespaceDeleted(ctx, kube, namespace, namespaceCleanupTimeout); err != nil {
		return err
	}
	if err := kube.Create(ctx, namespaceObject(namespace)); err != nil {
		return fmt.Errorf("recreate namespace %q: %w", namespace, err)
	}
	return nil
}

func namespaceObject(namespace string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
}

func isProtectedNamespace(namespace string) bool {
	switch namespace {
	case "default", "kube-system", "kube-public", "kube-node-lease", "higress-system", "metallb-system":
		return true
	default:
		return strings.HasPrefix(namespace, "kube-")
	}
}

func waitForNamespaceDeleted(ctx context.Context, kube client.Client, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := kube.Get(ctx, types.NamespacedName{Name: namespace}, namespaceObject(namespace))
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get namespace %q: %w", namespace, err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("namespace %q was still terminating after %s", namespace, timeout)
		}
		if err := sleepContext(ctx, time.Second); err != nil {
			return err
		}
	}
}

func cleanupNamespace(t *testing.T, kube client.Client, namespace string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), namespaceCleanupTimeout)
	defer cancel()

	if err := kube.Delete(ctx, namespaceObject(namespace)); err != nil && !apierrors.IsNotFound(err) {
		t.Logf("cleanup: delete namespace %q: %v", namespace, err)
		return
	}
	if err := waitForNamespaceDeleted(ctx, kube, namespace, namespaceCleanupTimeout); err != nil {
		t.Logf("cleanup: namespace %q deletion did not complete: %v", namespace, err)
	}
}

func createBackend(ctx context.Context, kube client.Client, namespace string) error {
	labels := map[string]string{"app": backendName}
	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backendName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "backend",
							Image:           backendImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: backendPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromInt(backendPort),
									},
								},
								InitialDelaySeconds: 2,
								PeriodSeconds:       2,
							},
						},
					},
				},
			},
		},
	}
	if err := kube.Create(ctx, deployment); err != nil {
		return fmt.Errorf("create deployment %q: %w", deployment.Name, err)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backendName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       backendPort,
					TargetPort: intstr.FromInt(backendPort),
				},
			},
		},
	}
	if err := kube.Create(ctx, service); err != nil {
		return fmt.Errorf("create service %q: %w", service.Name, err)
	}
	return nil
}

func waitForDeployment(ctx context.Context, kube client.Client, namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string

	for {
		deployment := &appsv1.Deployment{}
		err := kube.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deployment)
		if err != nil {
			lastState = err.Error()
		} else if deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.AvailableReplicas >= 1 {
			return nil
		} else {
			lastState = fmt.Sprintf("generation=%d observedGeneration=%d ready=%d available=%d",
				deployment.Generation, deployment.Status.ObservedGeneration,
				deployment.Status.ReadyReplicas, deployment.Status.AvailableReplicas)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("deployment %s/%s was not available within %s: %s", namespace, name, timeout, lastState)
		}
		if err := sleepContext(ctx, time.Second); err != nil {
			return err
		}
	}
}

func waitForHost(ctx context.Context, cfg scaleConfig, host string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := gatewayURL(cfg.address)
	httpClient := &http.Client{Timeout: httpRequestTimeout}
	var lastErr error

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("last response or error: %v", lastErr)
		}

		requestTimeout := httpRequestTimeout
		if remaining < requestTimeout {
			requestTimeout = remaining
		}
		requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("build request for host %q: %w", host, err)
		}
		request.Host = host

		response, err := httpClient.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				cancel()
				return nil
			}
			lastErr = fmt.Errorf("HTTP %d", response.StatusCode)
		} else {
			lastErr = err
		}
		cancel()

		if err := sleepContext(ctx, pollInterval); err != nil {
			return err
		}
	}
}

func gatewayURL(address string) string {
	address = strings.TrimRight(address, "/")
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	return address + "/"
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func evaluateResults(t *testing.T, cfg scaleConfig, a, b phaseResult) {
	t.Helper()

	reportResults(t, cfg, a, b)

	failed := false
	for _, result := range []phaseResult{a, b} {
		if result.err != nil {
			t.Errorf("phase %s failed: %v", result.name, result.err)
			failed = true
		}
		if len(result.samples) != eventsPerPhase {
			t.Errorf("phase %s collected %d/%d measured events", result.name, len(result.samples), eventsPerPhase)
			failed = true
		}
		for index, sample := range result.samples {
			if sample > cfg.eventMax.Seconds() {
				t.Errorf("phase %s event %d took %.3fs, exceeding E2E_SCALE_EVENT_MAX_SECONDS=%.3fs",
					result.name, index+1, sample, cfg.eventMax.Seconds())
				failed = true
			}
		}
	}

	medianA, medianB, ratio, ok := resultStats(a, b)
	if !ok {
		t.Errorf("cannot compute median ratio: phase A samples=%d, phase B samples=%d", len(a.samples), len(b.samples))
		failed = true
	} else if ratio > cfg.ratioMax {
		t.Errorf("median propagation ratio %.2f exceeds E2E_SCALE_RATIO_MAX=%.2f (median A=%.3fs, median B=%.3fs)",
			ratio, cfg.ratioMax, medianA, medianB)
		failed = true
	}

	if failed {
		t.Errorf("diagnosis: %s", diagnosis(cfg, a, b, ratio, ok))
	}
}

func reportPhaseFailure(t *testing.T, cfg scaleConfig, result phaseResult) {
	t.Helper()

	reportResults(t, cfg, result)
	t.Errorf("phase %s failed: %v", result.name, result.err)
	t.Errorf("diagnosis: %s", diagnosis(cfg, result, phaseResult{}, 0, false))
}

func reportResults(t *testing.T, cfg scaleConfig, results ...phaseResult) {
	t.Helper()

	t.Logf("scale thresholds: E2E_SCALE_BASE=%d, E2E_SCALE_RATIO_MAX=%.2f, E2E_SCALE_EVENT_MAX_SECONDS=%.3f",
		cfg.base, cfg.ratioMax, cfg.eventMax.Seconds())
	for _, result := range results {
		if result.name == "" {
			continue
		}
		t.Logf("phase %s: baseIngresses=%d configuredTotalIngresses=%d samples=%d rawSamples=%s median=%.3fs error=%v",
			result.name, result.baseIngresses, result.totalIngresses, len(result.samples),
			formatSamples(result.samples), median(result.samples), result.err)
	}

	if len(results) >= 2 {
		if medianA, medianB, ratio, ok := resultStats(results[0], results[1]); ok {
			t.Logf("median A=%.3fs median B=%.3fs ratio B/A=%.2f", medianA, medianB, ratio)
		}
	}
}

func resultStats(a, b phaseResult) (float64, float64, float64, bool) {
	if len(a.samples) == 0 || len(b.samples) == 0 {
		return 0, 0, 0, false
	}
	medianA := median(a.samples)
	medianB := median(b.samples)
	if medianA <= 0 {
		return medianA, medianB, 0, false
	}
	return medianA, medianB, medianB / medianA, true
}

func median(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func formatSamples(samples []float64) string {
	if len(samples) == 0 {
		return "[]"
	}
	formatted := make([]string, len(samples))
	for index, sample := range samples {
		formatted[index] = fmt.Sprintf("%.3f", sample)
	}
	return "[" + strings.Join(formatted, ", ") + "]"
}

func diagnosis(cfg scaleConfig, a, b phaseResult, ratio float64, haveRatio bool) string {
	timedOut := a.timedOut || b.timedOut
	for _, result := range []phaseResult{a, b} {
		for _, sample := range result.samples {
			if sample >= cfg.eventMax.Seconds() {
				timedOut = true
			}
		}
	}
	if timedOut {
		return fmt.Sprintf("at least one incremental event hit the %.0fs ceiling; the controller or gateway is not keeping up. Check higress-controller CPU/restarts and route or config push latency before treating this as a ratio regression.", cfg.eventMax.Seconds())
	}
	if haveRatio {
		if ratio >= 3.5 {
			return "doubling the Ingress count increased median propagation by roughly 4x. That is consistent with per-event work and cache dependency/index size growing with the total host-route and merged-gateway set, i.e. a quadratic-style dependency regression rather than a roughly constant incremental propagation cost."
		}
		if ratio > cfg.ratioMax {
			return "median propagation grew faster than the configured allowance when the Ingress count doubled. Inspect the scope of host-route rebuilds and the size of the gateway dependency index."
		}
	}
	return "no absolute timeout or clearly superlinear ratio was observed; inspect the raw samples and controller logs for outliers."
}
