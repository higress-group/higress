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
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/model"
	"github.com/nacos-group/nacos-sdk-go/vo"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/log"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	"github.com/alibaba/higress/v2/pkg/common"
	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	provider "github.com/alibaba/higress/v2/registry"
	"github.com/alibaba/higress/v2/registry/memory"
)

const (
	DefaultNacosTimeout         = 5000
	DefaultNacosLogLevel        = "warn"
	DefaultNacosLogDir          = "/var/log/nacos/log/"
	DefaultNacosCacheDir        = "/var/log/nacos/cache/"
	DefaultNacosNotLoadCache    = true
	DefaultNacosLogRotateTime   = "24h"
	DefaultNacosLogMaxAge       = 3
	DefaultUpdateCacheWhenEmpty = true
	DefaultRefreshInterval      = time.Second * 30
	DefaultRefreshIntervalLimit = time.Second * 10
	DefaultFetchPageSize        = 50
	DefaultJoiner               = "@@"
)

type watcher struct {
	provider.BaseWatcher
	apiv1.RegistryConfig
	WatchingServices     map[string]bool              `json:"watching_services"`
	RegistryType         provider.ServiceRegistryType `json:"registry_type"`
	Status               provider.WatcherStatus       `json:"status"`
	namingClient         naming_client.INamingClient
	cache                memory.Cache
	mutex                *sync.Mutex
	stop                 chan struct{}
	isStop               bool
	updateCacheWhenEmpty bool
	authOption           provider.AuthOption
}

type WatcherOption func(w *watcher)

func NewWatcher(cache memory.Cache, opts ...WatcherOption) (provider.Watcher, error) {
	w := &watcher{
		WatchingServices: make(map[string]bool),
		RegistryType:     provider.Nacos,
		Status:           provider.UnHealthy,
		cache:            cache,
		mutex:            &sync.Mutex{},
		stop:             make(chan struct{}),
	}

	w.NacosRefreshInterval = int64(DefaultRefreshInterval)

	for _, opt := range opts {
		opt(w)
	}

	if w.NacosNamespace == "" {
		w.NacosNamespace = w.NacosNamespaceId
	}

	log.Infof("new nacos watcher with config Name:%s", w.Name)

	cc := constant.NewClientConfig(
		constant.WithTimeoutMs(DefaultNacosTimeout),
		constant.WithLogLevel(DefaultNacosLogLevel),
		constant.WithLogDir(DefaultNacosLogDir),
		constant.WithCacheDir(DefaultNacosCacheDir),
		constant.WithNotLoadCacheAtStart(DefaultNacosNotLoadCache),
		constant.WithRotateTime(DefaultNacosLogRotateTime),
		constant.WithMaxAge(DefaultNacosLogMaxAge),
		constant.WithUpdateCacheWhenEmpty(w.updateCacheWhenEmpty),
		constant.WithNamespaceId(w.NacosNamespaceId),
	)

	sc := []constant.ServerConfig{
		*constant.NewServerConfig(w.Domain, uint64(w.Port)),
	}

	namingClient, err := clients.NewNamingClient(vo.NacosClientParam{
		ClientConfig:  cc,
		ServerConfigs: sc,
	})
	if err != nil {
		log.Errorf("can not create naming client, err:%v", err)
		return nil, err
	}

	w.namingClient = namingClient

	return w, nil
}

func WithVport(vport *apiv1.RegistryConfig_VPort) WatcherOption {
	return func(w *watcher) {
		w.Vport = vport
	}
}

func WithNacosNamespaceId(nacosNamespaceId string) WatcherOption {
	return func(w *watcher) {
		if nacosNamespaceId == "" {
			w.NacosNamespaceId = "public"
		} else {
			w.NacosNamespaceId = nacosNamespaceId
		}
	}
}

func WithNacosNamespace(nacosNamespace string) WatcherOption {
	return func(w *watcher) {
		w.NacosNamespace = nacosNamespace
	}
}

func WithNacosGroups(nacosGroups []string) WatcherOption {
	return func(w *watcher) {
		w.NacosGroups = nacosGroups
	}
}

func WithNacosRefreshInterval(refreshInterval int64) WatcherOption {
	return func(w *watcher) {
		if refreshInterval < int64(DefaultRefreshIntervalLimit) {
			refreshInterval = int64(DefaultRefreshIntervalLimit)
		}
		w.NacosRefreshInterval = refreshInterval
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

func WithAuthOption(authOption provider.AuthOption) WatcherOption {
	return func(w *watcher) {
		w.authOption = authOption
	}
}

func (w *watcher) Run() {
	ticker := time.NewTicker(time.Duration(w.NacosRefreshInterval))
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

func (w *watcher) fetchAllServices() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.isStop {
		return nil
	}
	fetchedServices := make(map[string]bool)
	for _, groupName := range w.NacosGroups {
		for page := 1; ; page++ {
			ss, err := w.namingClient.GetAllServicesInfo(vo.GetAllServiceInfoParam{
				GroupName: groupName,
				PageNo:    uint32(page),
				PageSize:  DefaultFetchPageSize,
				NameSpace: w.NacosNamespace,
			})
			if err != nil {
				log.Errorf("fetch all services error:%v", err)
				break
			}
			for _, serviceName := range ss.Doms {
				fetchedServices[groupName+DefaultJoiner+serviceName] = true
			}
			if len(ss.Doms) < DefaultFetchPageSize {
				break
			}
		}
	}

	for key := range w.WatchingServices {
		if _, exist := fetchedServices[key]; !exist {
			s := strings.Split(key, DefaultJoiner)
			err := w.unsubscribe(s[0], s[1])
			if err == nil {
				delete(w.WatchingServices, key)
			}
		}
	}

	for key := range fetchedServices {
		if _, exist := w.WatchingServices[key]; !exist {
			s := strings.Split(key, DefaultJoiner)
			if !shouldSubscribe(s[1]) {
				continue
			}
			err := w.subscribe(s[0], s[1])
			if err == nil {
				w.WatchingServices[key] = true
			}
		}
	}
	return nil
}

func (w *watcher) subscribe(groupName string, serviceName string) error {
	log.Debugf("subscribe service, groupName:%s, serviceName:%s", groupName, serviceName)

	err := w.namingClient.Subscribe(&vo.SubscribeParam{
		ServiceName:       serviceName,
		GroupName:         groupName,
		SubscribeCallback: w.getSubscribeCallback(groupName, serviceName),
	})
	if err != nil {
		log.Errorf("subscribe service error:%v, groupName:%s, serviceName:%s", err, groupName, serviceName)
		return err
	}

	return nil
}

func (w *watcher) unsubscribe(groupName string, serviceName string) error {
	log.Debugf("unsubscribe service, groupName:%s, serviceName:%s", groupName, serviceName)

	err := w.namingClient.Unsubscribe(&vo.SubscribeParam{
		ServiceName:       serviceName,
		GroupName:         groupName,
		SubscribeCallback: w.getSubscribeCallback(groupName, serviceName),
	})
	if err != nil {
		log.Errorf("unsubscribe service error:%v, groupName:%s, serviceName:%s", err, groupName, serviceName)
		return err
	}

	return nil
}

func (w *watcher) getSubscribeCallback(groupName string, serviceName string) func(services []model.SubscribeService, err error) {
	suffix := strings.Join([]string{groupName, w.NacosNamespace, w.Type}, common.DotSeparator)
	suffix = strings.ReplaceAll(suffix, common.Underscore, common.Hyphen)
	host := strings.Join([]string{serviceName, suffix}, common.DotSeparator)

	return func(services []model.SubscribeService, err error) {
		defer w.UpdateService()

		// log.Info("callback", "serviceName", serviceName, "suffix", suffix, "details", services)

		if err != nil {
			if strings.Contains(err.Error(), "hosts is empty") {
				if w.updateCacheWhenEmpty {
					w.cache.DeleteServiceWrapper(host)
				}
			} else {
				log.Errorf("callback error:%v", err)
			}
			return
		}
		if len(services) > 0 && services[0].Metadata != nil && services[0].Metadata["register-resource"] == "mcp-bridge" {
			return
		}
		serviceEntry := w.generateServiceEntry(host, services)
		w.cache.UpdateServiceWrapper(host, &ingress.ServiceWrapper{
			ServiceName:  serviceName,
			ServiceEntry: serviceEntry,
			Suffix:       suffix,
			RegistryType: w.Type,
			RegistryName: w.Name,
		})
	}
}

func (w *watcher) generateServiceEntry(host string, services []model.SubscribeService) *v1alpha3.ServiceEntry {
	portList := make([]*v1alpha3.ServicePort, 0)
	endpoints := make([]*v1alpha3.WorkloadEntry, 0)
	sePort := provider.GetServiceVport(host, w.Vport)
	for _, service := range services {
		protocol := common.HTTP
		if service.Metadata != nil && service.Metadata["protocol"] != "" {
			protocol = common.ParseProtocol(service.Metadata["protocol"])
		} else {
			service.Metadata = make(map[string]string)
		}
		port := &v1alpha3.ServicePort{
			Name:     protocol.String(),
			Number:   uint32(service.Port),
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
		// Calculate weight from Nacos instance
		// Nacos weight is float64, need to convert to uint32 for Istio
		// Use math.Round to preserve fractional weights (e.g., 0.5, 1.5)
		// If weight is 0 or negative, use default weight 1
		weight := uint32(1)
		if service.Weight > 0 {
			weight = uint32(math.Round(service.Weight))
		}
		endpoint := v1alpha3.WorkloadEntry{
			Address: service.Ip,
			Ports:   map[string]uint32{port.Protocol: port.Number},
			Labels:  service.Metadata,
			Weight:  weight,
		}
		endpoints = append(endpoints, &endpoint)
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

func (w *watcher) Stop() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	for key := range w.WatchingServices {
		s := strings.Split(key, DefaultJoiner)
		err := w.unsubscribe(s[0], s[1])
		if err == nil {
			delete(w.WatchingServices, key)
		}

		// clean the cache
		suffix := strings.Join([]string{s[0], w.NacosNamespace, w.Type}, common.DotSeparator)
		suffix = strings.ReplaceAll(suffix, common.Underscore, common.Hyphen)
		host := strings.Join([]string{s[1], suffix}, common.DotSeparator)
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

func shouldSubscribe(serviceName string) bool {
	prefixFilters := []string{"consumers:"}
	fullFilters := []string{""}

	for _, f := range prefixFilters {
		if strings.HasPrefix(serviceName, f) {
			return false
		}
	}

	for _, f := range fullFilters {
		if serviceName == f {
			return false
		}
	}

	return true
}
