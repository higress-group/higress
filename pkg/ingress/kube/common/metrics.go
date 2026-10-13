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

package common

import (
	"istio.io/istio/pkg/cluster"
	"istio.io/pkg/monitoring"
)

type Event string

const (
	Normal Event = "normal"

	Unknown Event = "unknown"

	EmptyRule Event = "empty-rule"

	MissingSecret Event = "missing-secret"

	InvalidBackendService Event = "invalid-backend-service"

	DuplicatedRoute Event = "duplicated-route"

	DuplicatedTls Event = "duplicated-tls"

	PortNameResolveError Event = "port-name-resolve-error"

	InvalidPathRegex Event = "invalid-path-regex"
)

var (
	clusterTag  = monitoring.MustCreateLabel("cluster")
	invalidType = monitoring.MustCreateLabel("type")

	// totalIngresses tracks the total number of ingress
	totalIngresses = monitoring.NewGauge(
		"pilot_total_ingresses",
		"Total ingresses known to pilot.",
		monitoring.WithLabels(clusterTag),
	)

	totalInvalidIngress = monitoring.NewSum(
		"pilot_total_invalid_ingresses",
		"Total invalid ingresses known to pilot.",
		monitoring.WithLabels(clusterTag, invalidType),
	)
)

func init() {
	monitoring.MustRegister(totalIngresses)
	monitoring.MustRegister(totalInvalidIngress)
}

func RecordIngressNumber(cluster cluster.ID, number int) {
	totalIngresses.With(clusterTag.Value(cluster.String())).Record(float64(number))
}

func IncrementInvalidIngress(cluster cluster.ID, event Event) {
	totalInvalidIngress.With(clusterTag.Value(cluster.String()), invalidType.Value(string(event))).Increment()
}
