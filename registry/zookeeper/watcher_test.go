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

package zookeeper

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSpringCloudConfig(t *testing.T) {
	var w watcher
	w.seMux = &sync.Mutex{}
	cases := []struct {
		name           string
		interfaceName  string
		content        []byte
		expectedHost   string
		expectedConfig InterfaceConfig
	}{
		{
			name:          "normal",
			interfaceName: "service-provider.services",
			content:       []byte(`{"name":"service-provider","id":"e479f40a-8f91-42a1-98e6-9377d224b360","address":"10.0.0.0","port":8071,"sslPort":null,"payload":{"@class":"org.springframework.cloud.zookeeper.discovery.ZookeeperInstance","id":"application-1","name":"service-provider","metadata":{"version":"1"}},"registrationTimeUTC":1663145171645,"serviceType":"DYNAMIC","uriSpec":{"parts":[{"value":"scheme","variable":true},{"value":"://","variable":false},{"value":"address","variable":true},{"value":":","variable":false},{"value":"port","variable":true}]}}`),
			expectedHost:  "service-provider.services",
			expectedConfig: InterfaceConfig{
				Host:        "service-provider.services",
				Protocol:    "HTTP",
				ServiceType: SpringCloudService,
				Endpoints: []Endpoint{
					{
						Ip:   "10.0.0.0",
						Port: "8071",
						Metadata: map[string]string{
							"version": "1",
						},
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actualHost, actualConfig, err := w.GetSpringCloudConfig(c.interfaceName, c.content)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, c.expectedHost, actualHost)
			assert.Equal(t, c.expectedConfig, *actualConfig)
		})
	}
}

func TestGetDubboConfig(t *testing.T) {
	var w watcher
	w.seMux = &sync.Mutex{}
	cases := []struct {
		name           string
		url            string
		expectedHost   string
		expectedConfig InterfaceConfig
	}{
		{
			name:         "no version",
			url:          `/dubbo/org.apache.dubbo.samples.api.GreetingService/providers/dubbo%3A%2F%2F10.0.0.0%3A20880%2Fcom.alibaba.adrive.business.contract.service.UserVipService%3Fzone%3Dcn-shanghai-g%26dubbo%3D2.0.2`,
			expectedHost: "providers:org.apache.dubbo.samples.api.GreetingService:0.0.0:",
			expectedConfig: InterfaceConfig{
				Host:        "providers:org.apache.dubbo.samples.api.GreetingService:0.0.0:",
				Protocol:    "dubbo",
				ServiceType: DubboService,
				Endpoints: []Endpoint{
					{
						Ip:   "10.0.0.0",
						Port: "20880",
						Metadata: map[string]string{
							"zone":     "cn-shanghai-g",
							"dubbo":    "2.0.2",
							"protocol": "dubbo",
						},
					},
				},
			},
		},
		{
			name:         "has version",
			url:          `/dubbo/org.apache.dubbo.samples.api.GreetingService/providers/dubbo%3A%2F%2F10.0.0.0%3A20880%2Fcom.alibaba.adrive.business.contract.service.UserVipService%3Fzone%3Dcn-shanghai-g%26dubbo%3D2.0.2%26version%3D1.0.0`,
			expectedHost: "providers:org.apache.dubbo.samples.api.GreetingService:1.0.0:",
			expectedConfig: InterfaceConfig{
				Host:        "providers:org.apache.dubbo.samples.api.GreetingService:1.0.0:",
				Protocol:    "dubbo",
				ServiceType: DubboService,
				Endpoints: []Endpoint{
					{
						Ip:   "10.0.0.0",
						Port: "20880",
						Metadata: map[string]string{
							"zone":     "cn-shanghai-g",
							"dubbo":    "2.0.2",
							"protocol": "dubbo",
							"version":  "1.0.0",
						},
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actualHost, actualConfig, err := w.GetDubboConfig(c.url)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, c.expectedHost, actualHost)
			assert.Equal(t, c.expectedConfig, *actualConfig)
		})
	}
}

func TestGenerateServiceEntryRejectsInvalidPorts(t *testing.T) {
	w := &watcher{}
	serviceEntry := w.generateServiceEntry(InterfaceConfig{
		Host: "example.service",
		Endpoints: []Endpoint{
			{Ip: "10.0.0.1", Port: "8080"},
			{Ip: "10.0.0.2", Port: "-1"},
			{Ip: "10.0.0.3", Port: "0"},
			{Ip: "10.0.0.4", Port: "65536"},
			{Ip: "10.0.0.5", Port: "4294967296"},
			{Ip: "10.0.0.6", Port: "invalid"},
		},
	})

	assert.Len(t, serviceEntry.Ports, 1)
	assert.Equal(t, uint32(8080), serviceEntry.Ports[0].Number)
	assert.Len(t, serviceEntry.Endpoints, 1)
	assert.Equal(t, "10.0.0.1", serviceEntry.Endpoints[0].Address)
}
