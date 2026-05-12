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

package registry

import (
	"net"
	"strings"
	"time"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/log"
)

const (
	Zookeeper ServiceRegistryType = "zookeeper"
	Eureka    ServiceRegistryType = "eureka"
	Consul    ServiceRegistryType = "consul"
	Nacos     ServiceRegistryType = "nacos"
	Nacos2    ServiceRegistryType = "nacos2"
	Nacos3    ServiceRegistryType = "nacos3"
	Static    ServiceRegistryType = "static"
	DNS       ServiceRegistryType = "dns"
	Healthy   WatcherStatus       = "healthy"
	UnHealthy WatcherStatus       = "unhealthy"

	DefaultDialTimeout = time.Second * 3
)

type ServiceRegistryType string

func (srt *ServiceRegistryType) String() string {
	return string(*srt)
}

type WatcherStatus string

func (ws *WatcherStatus) String() string {
	return string(*ws)
}

type Watcher interface {
	Run()
	Stop()
	IsHealthy() bool
	IsReady() bool
	GetRegistryType() string
	AppendServiceUpdateHandler(f func())
	ReadyHandler(f func(bool))
}

type BaseWatcher struct {
	UpdateService ServiceUpdateHandler
	Ready         ReadyHandler
	ReadyStatus   bool
}

func (w *BaseWatcher) Run()                    {}
func (w *BaseWatcher) Stop()                   {}
func (w *BaseWatcher) IsHealthy() bool         { return true }
func (w *BaseWatcher) IsReady() bool           { return w.ReadyStatus }
func (w *BaseWatcher) GetRegistryType() string { return "" }
func (w *BaseWatcher) AppendServiceUpdateHandler(f func()) {
	w.UpdateService = f
}

func (w *BaseWatcher) ReadyHandler(f func(isReady bool)) {
	w.Ready = func(isReady bool) {
		w.ReadyStatus = isReady
		f(isReady)
	}
}

type (
	ServiceUpdateHandler func()
	ReadyHandler         func(bool)
)

func ProbeWatcherStatus(host string, port string) WatcherStatus {
	address := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", address, DefaultDialTimeout)
	if err != nil || conn == nil {
		return UnHealthy
	}
	_ = conn.Close()
	return Healthy
}

func GetServiceVport(host string, vport *apiv1.RegistryConfig_VPort) *v1alpha3.ServicePort {
	if vport == nil {
		log.Warnf("there is no vport exist for: %s, skip", host)
		return nil
	}
	for _, service := range vport.Services {
		if strings.EqualFold(service.Name, host) && isValidPort(service.Value) {
			log.Infof("service %s vport exist, use service vport %d", host, service.Value)
			return &v1alpha3.ServicePort{
				Number: service.Value,
			}
		}
	}
	if isValidPort(vport.Default) {
		log.Infof("there is only default vport exist, use default vport %d", vport.Default)
		return &v1alpha3.ServicePort{
			Number: vport.Default,
		}
	}
	return nil
}

func isValidPort(port uint32) bool {
	return port > 0 && port <= 65535
}
