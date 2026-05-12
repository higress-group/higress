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

package eureka

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/hudl/fargo"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/log"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	"github.com/alibaba/higress/v2/pkg/common"
	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	provider "github.com/alibaba/higress/v2/registry"
	. "github.com/alibaba/higress/v2/registry/eureka/client"
	"github.com/alibaba/higress/v2/registry/memory"
)

const (
	DefaultFullRefreshIntervalLimit = time.Second * 30
	suffix                          = "eureka"
)

type watcher struct {
	provider.BaseWatcher
	apiv1.RegistryConfig

	WatchingServices     map[string]*Plan             `json:"watching_services"`
	RegistryType         provider.ServiceRegistryType `json:"registry_type"`
	Status               provider.WatcherStatus       `json:"status"`
	cache                memory.Cache
	mutex                *sync.Mutex
	stop                 chan struct{}
	isStop               bool
	updateCacheWhenEmpty bool

	eurekaClient              EurekaHttpClient
	fullRefreshIntervalLimit  time.Duration
	deltaRefreshIntervalLimit time.Duration
}

type WatcherOption func(w *watcher)

func NewWatcher(cache memory.Cache, opts ...WatcherOption) (provider.Watcher, error) {
	w := &watcher{
		WatchingServices: make(map[string]*Plan),
		RegistryType:     provider.Eureka,
		Status:           provider.UnHealthy,
		cache:            cache,
		mutex:            &sync.Mutex{},
		stop:             make(chan struct{}),
	}

	w.fullRefreshIntervalLimit = DefaultFullRefreshIntervalLimit

	for _, opt := range opts {
		opt(w)
	}

	cfg := NewDefaultConfig()
	cfg.BaseUrl = net.JoinHostPort(w.Domain, strconv.FormatUint(uint64(w.Port), 10))
	w.eurekaClient = NewEurekaHttpClient(cfg)

	return w, nil
}

func WithVport(vport *apiv1.RegistryConfig_VPort) WatcherOption {
	return func(w *watcher) {
		w.Vport = vport
	}
}

func WithEurekaFullRefreshInterval(refreshInterval int64) WatcherOption {
	return func(w *watcher) {
		if refreshInterval < int64(DefaultFullRefreshIntervalLimit) {
			refreshInterval = int64(DefaultFullRefreshIntervalLimit)
		}
		w.fullRefreshIntervalLimit = time.Duration(refreshInterval)
	}
}

func WithType(t string) WatcherOption {
	return func(w *watcher) {
		w.Type = t
	}
}

func WithName(name string) WatcherOption {
	return func(w *watcher) {
		w.Name = name
	}
}

func WithDomain(domain string) WatcherOption {
	return func(w *watcher) {
		w.Domain = domain
	}
}

func WithPort(port uint32) WatcherOption {
	return func(w *watcher) {
		w.Port = port
	}
}

func WithUpdateCacheWhenEmpty(enable bool) WatcherOption {
	return func(w *watcher) {
		w.updateCacheWhenEmpty = enable
	}
}

func (w *watcher) Run() {
	ticker := time.NewTicker(w.fullRefreshIntervalLimit)
	defer ticker.Stop()

	w.Status = provider.ProbeWatcherStatus(w.Domain, strconv.FormatUint(uint64(w.Port), 10))
	w.doFullRefresh()
	w.Ready(true)

	for {
		select {
		case <-ticker.C:
			w.doFullRefresh()
		case <-w.stop:
			log.Infof("eureka watcher(%v) is stopping ...", w.Name)
			return
		}
	}
}

func (w *watcher) Stop() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	for serviceName := range w.WatchingServices {
		if err := w.unsubscribe(serviceName); err != nil {
			log.Errorf("Failed to unsubscribe service : %v", serviceName)
			continue
		}
		w.cache.DeleteServiceWrapper(makeHost(serviceName))
	}
	w.UpdateService()
	w.isStop = true
	close(w.stop)
	w.Ready(false)
}

func (w *watcher) IsHealthy() bool {
	return w.Status == provider.Healthy
}

func (w *watcher) GetRegistryType() string {
	return w.RegistryType.String()
}

// doFullRefresh todo(lql): it's better to support deltaRefresh
func (w *watcher) doFullRefresh() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	applications, err := w.eurekaClient.GetApplications()
	if err != nil {
		log.Errorf("Failed to full fetch eureka services, error : %v", err)
		return
	}

	fetchedServices := applications.Apps
	for serviceName := range w.WatchingServices {
		if _, ok := fetchedServices[serviceName]; !ok {
			if err = w.unsubscribe(serviceName); err != nil {
				log.Errorf("Failed to unsubscribe service %v, error : %v", serviceName, err)
				continue
			}
		}
	}

	for serviceName := range fetchedServices {
		if _, ok := w.WatchingServices[serviceName]; !ok {
			if err = w.subscribe(fetchedServices[serviceName]); err != nil {
				log.Errorf("Failed to subscribe service %v, error : %v", serviceName, err)
				continue
			}
		}
	}
}

func (w *watcher) subscribe(service *fargo.Application) error {
	if service == nil {
		return fmt.Errorf("service is nil")
	}
	callback := func(service *fargo.Application) error {
		defer w.UpdateService()

		if len(service.Instances) != 0 {
			se, err := w.generateServiceEntry(service)
			if err != nil {
				return err
			}
			w.cache.UpdateServiceWrapper(makeHost(service.Name), &ingress.ServiceWrapper{
				ServiceName:  service.Name,
				ServiceEntry: se,
				Suffix:       suffix,
				RegistryType: w.Type,
				RegistryName: w.Name,
			})
			return nil
		}

		if w.updateCacheWhenEmpty {
			w.cache.DeleteServiceWrapper(makeHost(service.Name))
		}

		return nil
	}

	if err := callback(service); err != nil {
		log.Errorf("failed to subscribe service %v, error: %v", service.Name, err)
		return err
	}
	w.WatchingServices[service.Name] = NewPlan(w.eurekaClient, service.Name, callback)
	return nil
}

func (w *watcher) unsubscribe(serviceName string) error {
	w.WatchingServices[serviceName].Stop()
	delete(w.WatchingServices, serviceName)
	w.UpdateService()

	return nil
}

func makeHost(serviceName string) string {
	return serviceName + common.DotSeparator + suffix
}

func convertMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		if value, ok := v.(string); ok {
			result[k] = value
		}
	}

	return result
}

func (w *watcher) generateServiceEntry(app *fargo.Application) (*v1alpha3.ServiceEntry, error) {
	portList := make([]*v1alpha3.ServicePort, 0)
	endpoints := make([]*v1alpha3.WorkloadEntry, 0)
	sePort := provider.GetServiceVport(makeHost(app.Name), w.Vport)
	for _, instance := range app.Instances {
		protocol := common.HTTP
		if val, _ := instance.Metadata.GetString("protocol"); val != "" {
			if protocol = common.ParseProtocol(val); protocol == common.Unsupported {
				return nil, fmt.Errorf("unsupported protocol %v", val)
			}
		}
		port := &v1alpha3.ServicePort{
			Name:     protocol.String(),
			Number:   uint32(instance.Port),
			Protocol: protocol.String(),
		}
		if len(portList) == 0 {
			if sePort != nil {
				sePort.Name = port.Name
				sePort.Protocol = port.Protocol
				portList = append(portList, sePort)
			} else {
				portList = append(portList, port)
			}
		}
		endpoint := v1alpha3.WorkloadEntry{
			Address: instance.IPAddr,
			Ports:   map[string]uint32{port.Protocol: port.Number},
			Labels:  convertMap(instance.Metadata.GetMap()),
		}
		endpoints = append(endpoints, &endpoint)
	}

	se := &v1alpha3.ServiceEntry{
		Hosts:      []string{makeHost(app.Name)},
		Ports:      portList,
		Location:   v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: v1alpha3.ServiceEntry_STATIC,
		Endpoints:  endpoints,
	}

	return se, nil
}
