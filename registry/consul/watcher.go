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

package consul

import (
	"strconv"
	"strings"
	"sync"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/log"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	"github.com/alibaba/higress/v2/pkg/common"
	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	provider "github.com/alibaba/higress/v2/registry"
	"github.com/alibaba/higress/v2/registry/memory"
)

const (
	ConsulHealthPassing         = "passing"
	DefaultRefreshInterval      = time.Second * 30
	DefaultRefreshIntervalLimit = time.Second * 10
)

type watcher struct {
	provider.BaseWatcher
	apiv1.RegistryConfig
	serverAddress        string
	consulClient         *consulapi.Client
	consulCatalog        *consulapi.Catalog
	WatchingServices     map[string]bool
	watchers             map[string]*watch.Plan
	RegistryType         provider.ServiceRegistryType
	Status               provider.WatcherStatus
	cache                memory.Cache
	mutex                *sync.Mutex
	stop                 chan struct{}
	isStop               bool
	updateCacheWhenEmpty bool
	authOption           provider.AuthOption
}

type WatcherOption func(w *watcher)

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

func WithDatacenter(dataCenter string) WatcherOption {
	return func(w *watcher) {
		w.ConsulDatacenter = dataCenter
	}
}

func WithAuthOption(authOption provider.AuthOption) WatcherOption {
	return func(w *watcher) {
		w.authOption = authOption
	}
}

func WithServiceTag(serviceTag string) WatcherOption {
	return func(w *watcher) {
		w.ConsulServiceTag = strings.ToLower(strings.TrimSpace(serviceTag))
	}
}

func WithRefreshInterval(refreshInterval int64) WatcherOption {
	return func(w *watcher) {
		if refreshInterval < int64(DefaultRefreshIntervalLimit) {
			refreshInterval = int64(DefaultRefreshIntervalLimit)
		}
		w.ConsulRefreshInterval = refreshInterval
	}
}

func NewWatcher(cache memory.Cache, opts ...WatcherOption) (provider.Watcher, error) {
	w := &watcher{
		WatchingServices: make(map[string]bool),
		watchers:         make(map[string]*watch.Plan),
		RegistryType:     provider.Consul,
		Status:           provider.UnHealthy,
		cache:            cache,
		mutex:            &sync.Mutex{},
		stop:             make(chan struct{}),
	}

	// Set default
	w.ConsulRefreshInterval = int64(DefaultRefreshInterval)

	// Set option
	for _, opt := range opts {
		opt(w)
	}

	// Init consul client
	w.serverAddress = w.Domain + ":" + strconv.Itoa(int(w.Port))
	config := consulapi.DefaultConfig()
	config.Address = w.serverAddress
	config.Token = w.authOption.ConsulToken
	client, err := consulapi.NewClient(config)
	if err != nil {
		log.Errorf("[NewWatcher] NewWatcher consul, err:%v, consul address:%s", err, w.serverAddress)
		return nil, err
	}
	w.consulClient = client
	w.consulCatalog = client.Catalog()
	return w, nil
}

func (w *watcher) fetchAllServices() error {
	log.Infof("consul fetchAllServices")
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.isStop {
		return nil
	}

	fetchedServices := make(map[string]bool)
	q := &consulapi.QueryOptions{}
	q.Datacenter = w.ConsulDatacenter
	q.Token = w.authOption.ConsulToken
	services, _, err := w.consulCatalog.Services(q)
	if err != nil {
		log.Errorf("consul fetch all services error:%v", err)
		return err
	}

	for serviceName, tags := range services {
		if w.filterTags(w.ConsulServiceTag, tags) {
			fetchedServices[serviceName] = true
		}
	}
	log.Infof("consul fetch services num:%d", len(fetchedServices))

	for serviceName := range w.WatchingServices {
		if _, exist := fetchedServices[serviceName]; !exist {
			err := w.unsubscribe(serviceName)
			if err == nil {
				delete(w.WatchingServices, serviceName)
			}
		}
	}

	for serviceName := range fetchedServices {
		if _, exist := w.WatchingServices[serviceName]; !exist {
			if !w.shouldSubscribe(serviceName) {
				continue
			}
			err := w.subscribe(serviceName)
			if err == nil {
				w.WatchingServices[serviceName] = true
			}
		}
	}

	return nil
}

func (w *watcher) filterTags(consulTag string, tags []string) bool {
	if len(consulTag) == 0 {
		return true
	}

	if len(tags) == 0 {
		return false
	}

	for _, tag := range tags {
		if strings.ToLower(tag) == consulTag {
			return true
		}
	}

	return false
}

func (w *watcher) Run() {
	ticker := time.NewTicker(time.Duration(w.ConsulRefreshInterval))
	defer ticker.Stop()
	w.Status = provider.ProbeWatcherStatus(w.Domain, strconv.FormatUint(uint64(w.Port), 10))
	w.fetchAllServices()
	w.Ready(true)
	for {
		select {
		case <-ticker.C:
			w.fetchAllServices()
		case <-w.stop:
			return
		}
	}
}

func (w *watcher) Stop() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	for serviceName := range w.WatchingServices {
		err := w.unsubscribe(serviceName)
		if err == nil {
			delete(w.WatchingServices, serviceName)
		}
		// clean the cache
		suffix := strings.Join([]string{serviceName, w.ConsulDatacenter, w.Type}, common.DotSeparator)
		host := strings.ReplaceAll(suffix, common.Underscore, common.Hyphen)
		w.cache.DeleteServiceWrapper(host)
	}
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

func (w *watcher) unsubscribe(serviceName string) error {
	log.Infof("consul unsubscribe service, serviceName:%s", serviceName)
	if plan, ok := w.watchers[serviceName]; ok {
		plan.Stop()
		delete(w.watchers, serviceName)
	}
	return nil
}

func (w *watcher) shouldSubscribe(serviceName string) bool {
	return true
}

func (w *watcher) subscribe(serviceName string) error {
	log.Infof("consul  subscribe service, serviceName:%s", serviceName)
	plan, err := watch.Parse(map[string]interface{}{
		"type":    "service",
		"service": serviceName,
	})
	if err != nil {
		return err
	}
	plan.Handler = w.getSubscribeCallback(serviceName)
	plan.Token = w.authOption.ConsulToken
	plan.Datacenter = w.ConsulDatacenter
	go plan.Run(w.serverAddress)
	w.watchers[serviceName] = plan
	return nil
}

func (w *watcher) getSubscribeCallback(serviceName string) func(idx uint64, data interface{}) {
	suffix := strings.Join([]string{serviceName, w.ConsulDatacenter, w.Type}, common.DotSeparator)
	host := strings.ReplaceAll(suffix, common.Underscore, common.Hyphen)

	return func(idx uint64, data interface{}) {
		log.Infof("consul subscribe callback service, host:%s, serviceName:%s", host, serviceName)
		switch services := data.(type) {
		case []*consulapi.ServiceEntry:
			defer w.UpdateService()
			serviceEntry := w.generateServiceEntry(host, services)
			if serviceEntry != nil {
				log.Infof("consul update serviceEntry %s cache", host)
				w.cache.UpdateServiceWrapper(host, &ingress.ServiceWrapper{
					ServiceEntry: serviceEntry,
					ServiceName:  serviceName,
					Suffix:       suffix,
					RegistryType: w.Type,
					RegistryName: w.Name,
				})
			} else {
				log.Infof("consul serviceEntry %s is nil", host)
				// w.cache.DeleteServiceWrapper(host)
			}
		}
	}
}

func (w *watcher) generateServiceEntry(host string, services []*consulapi.ServiceEntry) *v1alpha3.ServiceEntry {
	portList := make([]*v1alpha3.ServicePort, 0)
	endpoints := make([]*v1alpha3.WorkloadEntry, 0)

	for _, service := range services {
		// service status: maintenance > critical > warning > passing
		if service.Checks.AggregatedStatus() != ConsulHealthPassing {
			continue
		}

		metaData := make(map[string]string, 0)
		if service.Service.Meta != nil {
			metaData = service.Service.Meta
		}

		protocol := common.HTTP
		if metaData["protocol"] != "" {
			protocol = common.ParseProtocol(metaData["protocol"])
		}

		port := &v1alpha3.ServicePort{
			Name:     protocol.String(),
			Number:   uint32(service.Service.Port),
			Protocol: protocol.String(),
		}

		if len(portList) == 0 {
			portList = append(portList, port)
		}

		endpoint := v1alpha3.WorkloadEntry{
			Address: service.Service.Address,
			Ports:   map[string]uint32{port.Protocol: port.Number},
			Labels:  metaData,
		}
		endpoints = append(endpoints, &endpoint)
	}

	if len(endpoints) == 0 {
		return nil
	}

	se := &v1alpha3.ServiceEntry{
		Hosts:      []string{host},
		Ports:      portList,
		Location:   v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: v1alpha3.ServiceEntry_STATIC,
		Endpoints:  endpoints,
	}

	return se
}
