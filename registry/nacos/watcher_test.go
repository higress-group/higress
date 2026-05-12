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

package nacos

import (
	"testing"

	"github.com/nacos-group/nacos-sdk-go/model"
)

func Test_generateServiceEntry_Weight(t *testing.T) {
	w := &watcher{}

	testCases := []struct {
		name            string
		services        []model.SubscribeService
		expectedWeights []uint32
		description     string
	}{
		{
			name: "normal integer weights",
			services: []model.SubscribeService{
				{Ip: "192.168.1.1", Port: 8080, Weight: 5.0},
				{Ip: "192.168.1.2", Port: 8080, Weight: 3.0},
				{Ip: "192.168.1.3", Port: 8080, Weight: 2.0},
			},
			expectedWeights: []uint32{5, 3, 2},
			description:     "Integer weights should be converted correctly",
		},
		{
			name: "fractional weights with rounding",
			services: []model.SubscribeService{
				{Ip: "192.168.1.1", Port: 8080, Weight: 5.4},
				{Ip: "192.168.1.2", Port: 8080, Weight: 3.5},
				{Ip: "192.168.1.3", Port: 8080, Weight: 2.6},
			},
			expectedWeights: []uint32{5, 4, 3},
			description:     "Fractional weights should be rounded to nearest integer",
		},
		{
			name: "zero weight defaults to 1",
			services: []model.SubscribeService{
				{Ip: "192.168.1.1", Port: 8080, Weight: 0.0},
				{Ip: "192.168.1.2", Port: 8080, Weight: 5.0},
			},
			expectedWeights: []uint32{1, 5},
			description:     "Zero weight should default to 1",
		},
		{
			name: "negative weight defaults to 1",
			services: []model.SubscribeService{
				{Ip: "192.168.1.1", Port: 8080, Weight: -1.0},
				{Ip: "192.168.1.2", Port: 8080, Weight: 3.0},
			},
			expectedWeights: []uint32{1, 3},
			description:     "Negative weight should default to 1",
		},
		{
			name: "very small fractional weight rounds to 0 then defaults to 1",
			services: []model.SubscribeService{
				{Ip: "192.168.1.1", Port: 8080, Weight: 0.4},
				{Ip: "192.168.1.2", Port: 8080, Weight: 0.5},
				{Ip: "192.168.1.3", Port: 8080, Weight: 0.6},
			},
			expectedWeights: []uint32{1, 1, 1},
			description:     "Weights less than 0.5 round to 0, then default to 1; 0.5 and above round to 1",
		},
		{
			name: "large weights",
			services: []model.SubscribeService{
				{Ip: "192.168.1.1", Port: 8080, Weight: 100.0},
				{Ip: "192.168.1.2", Port: 8080, Weight: 50.5},
			},
			expectedWeights: []uint32{100, 51},
			description:     "Large weights should be handled correctly",
		},
		{
			name: "mixed weights",
			services: []model.SubscribeService{
				{Ip: "192.168.1.1", Port: 8080, Weight: 0.0},
				{Ip: "192.168.1.2", Port: 8080, Weight: 1.5},
				{Ip: "192.168.1.3", Port: 8080, Weight: -5.0},
				{Ip: "192.168.1.4", Port: 8080, Weight: 10.7},
			},
			expectedWeights: []uint32{1, 2, 1, 11},
			description:     "Mixed zero, negative, and fractional weights",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			se := w.generateServiceEntry("test-host", tc.services)

			if se == nil {
				t.Fatal("generateServiceEntry returned nil")
			}

			if len(se.Endpoints) != len(tc.expectedWeights) {
				t.Fatalf("expected %d endpoints, got %d", len(tc.expectedWeights), len(se.Endpoints))
			}

			for i, endpoint := range se.Endpoints {
				if endpoint.Weight != tc.expectedWeights[i] {
					t.Errorf("endpoint[%d]: expected weight %d, got %d (original weight: %f) - %s",
						i, tc.expectedWeights[i], endpoint.Weight, tc.services[i].Weight, tc.description)
				}
			}
		})
	}
}

func Test_generateServiceEntry_WeightFieldSet(t *testing.T) {
	w := &watcher{}

	services := []model.SubscribeService{
		{Ip: "192.168.1.1", Port: 8080, Weight: 5.0, Metadata: map[string]string{"zone": "a"}},
	}

	se := w.generateServiceEntry("test-host", services)

	if se == nil {
		t.Fatal("generateServiceEntry returned nil")
	}

	if len(se.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(se.Endpoints))
	}

	endpoint := se.Endpoints[0]

	// Verify all fields are set correctly
	if endpoint.Address != "192.168.1.1" {
		t.Errorf("expected address 192.168.1.1, got %s", endpoint.Address)
	}

	if endpoint.Weight != 5 {
		t.Errorf("expected weight 5, got %d", endpoint.Weight)
	}

	if endpoint.Labels == nil || endpoint.Labels["zone"] != "a" {
		t.Errorf("expected labels with zone=a, got %v", endpoint.Labels)
	}

	if endpoint.Ports == nil {
		t.Error("expected ports to be set")
	}
}

func Test_generateServiceEntry_EmptyServices(t *testing.T) {
	w := &watcher{}

	se := w.generateServiceEntry("test-host", []model.SubscribeService{})

	if se == nil {
		t.Fatal("generateServiceEntry returned nil")
	}

	if len(se.Endpoints) != 0 {
		t.Errorf("expected 0 endpoints for empty services, got %d", len(se.Endpoints))
	}
}
