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
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dubbogo/go-zookeeper/zk"
	gxzookeeper "github.com/dubbogo/gost/database/kv/zk"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/log"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	"github.com/alibaba/higress/v2/pkg/common"
	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	provider "github.com/alibaba/higress/v2/registry"
	"github.com/alibaba/higress/v2/registry/memory"
)

type watchConfig struct {
	exit   chan struct{}
	listen bool
}

type watcher struct {
	provider.BaseWatcher
	apiv1.RegistryConfig
	WatchingServices   map[string]watchConfig       `json:"watching_services"`
	RegistryType       provider.ServiceRegistryType `json:"registry_type"`
	Status             provider.WatcherStatus       `json:"status"`
	serviceRemaind     *atomic.Int32
	cache              memory.Cache
	mutex              *sync.Mutex
	stop               chan struct{}
	zkClient           *gxzookeeper.ZookeeperClient
	reconnectCh        <-chan struct{}
	Done               chan struct{}
	seMux              *sync.Mutex
	serviceEntry       map[string]InterfaceConfig
	listIndex          chan ListServiceConfig
	listServiceChan    chan struct{}
	isStop             bool
	keepStaleWhenEmpty bool
	zkServicesPath     []string
}

type WatcherOption func(w *watcher)

func NewWatcher(cache memory.Cache, opts ...WatcherOption) (provider.Watcher, error) {
	w := &watcher{
		WatchingServices: make(map[string]watchConfig),
		RegistryType:     provider.Zookeeper,
		Status:           provider.UnHealthy,
		cache:            cache,
		mutex:            &sync.Mutex{},
		stop:             make(chan struct{}),
		Done:             make(chan struct{}),
		seMux:            &sync.Mutex{},
		serviceEntry:     make(map[string]InterfaceConfig),
		listIndex:        make(chan ListServiceConfig, 1),
		listServiceChan:  make(chan struct{}),
		zkServicesPath:   []string{SPRING_CLOUD_SERVICES},
	}

	timeout, _ := time.ParseDuration(DEFAULT_REG_TIMEOUT)

	for _, opt := range opts {
		opt(w)
	}

	var address []string
	address = append(address, w.Domain+":"+strconv.Itoa(int(w.Port)))
	newClient, cltErr := gxzookeeper.NewZookeeperClient("zk", address, false, gxzookeeper.WithZkTimeOut(timeout))
	if cltErr != nil {
		log.Errorf("[NewWatcher] NewWatcher zk, err:%v, zk address:%s", cltErr, address)
		return nil, cltErr
	}
	valid := newClient.ZkConnValid()
	if !valid {
		log.Info("connect zk error")
		return nil, errors.New("connect zk error")
	}
	w.reconnectCh = newClient.Reconnect()
	w.zkClient = newClient
	go func() {
		w.HandleClientRestart()
	}()
	return w, nil
}

func WithKeepStaleWhenEmpty(enable bool) WatcherOption {
	return func(w *watcher) {
		w.keepStaleWhenEmpty = enable
	}
}

func WithZkServicesPath(paths []string) WatcherOption {
	return func(w *watcher) {
		for _, path := range paths {
			path = strings.TrimSuffix(path, common.Slash)
			if path == DUBBO_SERVICES || path == SPRING_CLOUD_SERVICES {
				continue
			}
			w.zkServicesPath = append(w.zkServicesPath, path)
		}
	}
}

func (w *watcher) HandleClientRestart() {
	for {
		select {
		case <-w.reconnectCh:
			w.reconnectCh = w.zkClient.Reconnect()
			log.Info("zkclient reconnected")
			w.RestartCallBack()
			time.Sleep(10 * time.Microsecond)
		case <-w.Done:
			log.Info("[HandleClientRestart] receive registry destroy event, quit client restart handler")
			return
		}
	}
}

func (w *watcher) RestartCallBack() bool {
	err := w.fetchAllServices()
	if err != nil {
		log.Errorf("[RestartCallBack] fetch all service for zk err:%v", err)
		return false
	}
	return true
}

type serviceInfo struct {
	serviceType ServiceType
	rootPath    string
	service     string
}

func (w *watcher) fetchedServices(fetchedServices map[string]serviceInfo, path string, serviceType ServiceType) error {
	children, err := w.zkClient.GetChildren(path)
	if err != nil {
		if err == gxzookeeper.ErrNilChildren || err == gxzookeeper.ErrNilNode ||
			strings.Contains(err.Error(), "has none children") {
			return nil
		} else {
			log.Errorf("[fetchAllServices] can not get children, err:%v, path:%s", err, path)
			return err
		}
	}
	info := serviceInfo{
		serviceType: serviceType,
		rootPath:    path,
	}
	for _, child := range children {
		if child == CONFIG || child == MAPPING || child == METADATA {
			continue
		}
		var interfaceName string
		switch serviceType {
		case DubboService:
			interfaceName = child
		case SpringCloudService:
			info.service = child
			if path == "" || path == common.Slash {
				interfaceName = child
				break
			}
			interfaceName = child + "." + strings.ReplaceAll(
				strings.TrimPrefix(path, common.Slash), common.Slash, common.Hyphen)
		}
		fetchedServices[interfaceName] = info
		log.Debugf("fetchedServices, interface:%s, path:%s", interfaceName, info.rootPath)
	}
	return nil
}

func (w *watcher) fetchAllServices(firstFetch ...bool) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.isStop {
		return nil
	}

	fetchedServices := make(map[string]serviceInfo)
	var result error
	err := w.fetchedServices(fetchedServices, DUBBO_SERVICES, DubboService)
	if err != nil {
		result = multierror.Append(result, err)
	}
	for _, path := range w.zkServicesPath {
		err = w.fetchedServices(fetchedServices, path, SpringCloudService)
		if err != nil {
			result = multierror.Append(result, err)
		}
	}
	for interfaceName, value := range w.WatchingServices {
		if _, exist := fetchedServices[interfaceName]; !exist {
			if value.exit != nil {
				close(value.exit)
			}
			delete(w.WatchingServices, interfaceName)
		}
	}
	var serviceConfigs []ListServiceConfig
	for interfaceName, serviceInfo := range fetchedServices {
		if _, exist := w.WatchingServices[interfaceName]; !exist {
			w.WatchingServices[interfaceName] = watchConfig{
				exit:   make(chan struct{}),
				listen: true,
			}
			serviceConfig := ListServiceConfig{
				ServiceType:   serviceInfo.serviceType,
				InterfaceName: interfaceName,
				Exit:          w.WatchingServices[interfaceName].exit,
			}
			switch serviceInfo.serviceType {
			case DubboService:
				serviceConfig.UrlIndex = DUBBO + interfaceName + PROVIDERS
			case SpringCloudService:
				serviceConfig.UrlIndex = path.Join(serviceInfo.rootPath, serviceInfo.service)
			default:
				return errors.New("unknown type")
			}
			serviceConfigs = append(serviceConfigs, serviceConfig)
		}
	}
	if len(firstFetch) > 0 && firstFetch[0] {
		w.serviceRemaind = atomic.NewInt32(int32(len(serviceConfigs)))
	}
	for _, service := range serviceConfigs {
		w.listIndex <- service
	}
	return result
}

func (w *watcher) ListenService() {
	defer func() {
		w.listServiceChan <- struct{}{}
	}()
	ttl := DefaultTTL
	var failTimes int
	for {
		select {
		case listIndex := <-w.listIndex:
			go func() {
				for {
					log.Info(listIndex.UrlIndex)
					children, childEventCh, err := w.zkClient.GetChildrenW(listIndex.UrlIndex)
					if err != nil {
						failTimes++
						if MaxFailTimes <= failTimes {
							failTimes = MaxFailTimes
						}
						log.Errorf("[Zookeeper][ListenService] Get children of path zkRootPath with watcher failed, err:%v, index:%s", err, listIndex.UrlIndex)

						// May be the provider does not ready yet, sleep failTimes * ConnDelay seconds to wait
						after := time.After(timeSecondDuration(failTimes * ConnDelay))
						select {
						case <-after:
							continue
						case <-listIndex.Exit:
							return
						}
					}
					failTimes = 0
					if len(children) > 0 {
						w.ChildToServiceEntry(children, listIndex.InterfaceName, listIndex.UrlIndex, listIndex.ServiceType)
					}
					if w.serviceRemaind != nil {
						w.serviceRemaind.Sub(1)
					}
					if w.startScheduleWatchTask(listIndex, children, ttl, childEventCh, listIndex.Exit) {
						return
					}
				}
			}()
		case <-w.stop:
			log.Info("[ListenService] is shutdown")
			return
		}
	}
}

func (w *watcher) DataChange(eventType Event) bool {
	// fmt.Println(eventType)
	host, interfaceConfig, err := w.GetInterfaceConfig(eventType)
	if err != nil {
		log.Errorf("GetInterfaceConfig failed, err:%v, event:%v", err, eventType)
		return false
	}
	if eventType.Action == EventTypeAdd || eventType.Action == EventTypeUpdate {
		w.seMux.Lock()
		isHave := false
		value, ok := w.serviceEntry[host]
		if ok {
			for _, endpoint := range value.Endpoints {
				if endpoint.Ip == interfaceConfig.Endpoints[0].Ip && endpoint.Port == interfaceConfig.Endpoints[0].Port {
					isHave = true
				}
			}
			if !isHave {
				value.Endpoints = append(value.Endpoints, interfaceConfig.Endpoints[0])
			}
			w.serviceEntry[host] = value
		} else {
			w.serviceEntry[host] = *interfaceConfig
		}
		se := w.generateServiceEntry(w.serviceEntry[host])

		w.seMux.Unlock()
		w.cache.UpdateServiceWrapper(host, &ingress.ServiceWrapper{
			ServiceName:  host,
			ServiceEntry: se,
			Suffix:       "zookeeper",
			RegistryType: w.Type,
			RegistryName: w.Name,
		})
		w.UpdateService()
	} else if eventType.Action == EventTypeDel {
		w.seMux.Lock()
		value, ok := w.serviceEntry[host]
		if ok {
			var endpoints []Endpoint
			for _, endpoint := range value.Endpoints {
				if endpoint.Ip == interfaceConfig.Endpoints[0].Ip && endpoint.Port == interfaceConfig.Endpoints[0].Port {
					continue
				} else {
					endpoints = append(endpoints, endpoint)
				}
			}
			value.Endpoints = endpoints
			w.serviceEntry[host] = value
		}
		se := w.generateServiceEntry(w.serviceEntry[host])
		w.seMux.Unlock()
		// todo update
		if len(se.Endpoints) == 0 {
			if !w.keepStaleWhenEmpty {
				w.cache.DeleteServiceWrapper(host)
			}
		} else {
			w.cache.UpdateServiceWrapper(host, &ingress.ServiceWrapper{
				ServiceName:  host,
				ServiceEntry: se,
				Suffix:       "zookeeper",
				RegistryType: w.Type,
				RegistryName: w.Name,
			})
		}
		w.UpdateService()
	}
	return true
}

func (w *watcher) GetInterfaceConfig(event Event) (string, *InterfaceConfig, error) {
	switch event.ServiceType {
	case DubboService:
		return w.GetDubboConfig(event.Path)
	case SpringCloudService:
		return w.GetSpringCloudConfig(event.InterfaceName, event.Content)
	default:
		return "", nil, errors.New("unknown service type")
	}
}

func (w *watcher) GetSpringCloudConfig(interfaceName string, content []byte) (string, *InterfaceConfig, error) {
	var instance SpringCloudInstance
	err := json.Unmarshal(content, &instance)
	if err != nil {
		log.Errorf("unmarshal failed, err:%v, content:%s", err, content)
		return "", nil, err
	}
	var config InterfaceConfig
	host := interfaceName
	config.Host = host
	config.Protocol = common.HTTP.String()
	if len(instance.Payload.Metadata) > 0 && instance.Payload.Metadata["protocol"] != "" {
		config.Protocol = common.ParseProtocol(instance.Payload.Metadata["protocol"]).String()
	}
	port := strconv.Itoa(instance.Port)
	if port == "" {
		return "", nil, errors.New("empty port")
	}
	endpoint := Endpoint{
		Ip:       instance.Address,
		Port:     port,
		Metadata: instance.Payload.Metadata,
	}
	config.Endpoints = []Endpoint{endpoint}
	config.ServiceType = SpringCloudService
	return host, &config, nil
}

func (w *watcher) GetDubboConfig(dubboUrl string) (string, *InterfaceConfig, error) {
	dubboUrl = strings.Replace(dubboUrl, "%3F", "?", 1)
	dubboUrl = strings.ReplaceAll(dubboUrl, "%3D", "=")
	dubboUrl = strings.ReplaceAll(dubboUrl, "%26", "&")
	tempPath := strings.Replace(dubboUrl, DUBBO, "", -1)
	urls := strings.Split(tempPath, PROVIDERS+"/dubbo")
	key := urls[0]
	serviceUrl, urlParseErr := url.Parse(dubboUrl)
	if urlParseErr != nil {
		return "", nil, urlParseErr
	}
	var (
		dubboInterfaceConfig InterfaceConfig
		host                 string
	)

	serviceUrl.Path = strings.Replace(serviceUrl.Path, DUBBO+key+PROVIDERS+"/dubbo://", "", -1)

	values, err := url.ParseQuery(serviceUrl.RawQuery)
	if err != nil {
		return "", nil, err
	}

	paths := strings.Split(serviceUrl.Path, "/")

	if len(paths) > 0 {
		var group string
		_, ok := values["group"]
		if ok {
			group = values["group"][0]
		}
		version := "0.0.0"
		_, ok = values[VERSION]
		if ok && len(values[VERSION]) > 0 {
			version = values[VERSION][0]
		}
		dubboInterfaceConfig.Host = "providers:" + key + ":" + version + ":" + group
		host = dubboInterfaceConfig.Host
		dubboInterfaceConfig.Protocol = DUBBO_PROTOCOL
		address := strings.Split(paths[0], ":")
		if len(address) != 2 {
			log.Infof("[GetDubboConfig] can not get dubbo ip and port, path:%s ", serviceUrl.Path)
			return "", nil, errors.New("can not get dubbo ip and port")
		}
		metadata := make(map[string]string)
		for key, value := range values {
			if len(value) == 1 {
				metadata[key] = value[0]
			}
		}
		metadata[PROTOCOL] = DUBBO_PROTOCOL
		dubboEndpoint := Endpoint{
			Ip:       address[0],
			Port:     address[1],
			Metadata: metadata,
		}
		dubboInterfaceConfig.Endpoints = append(dubboInterfaceConfig.Endpoints, dubboEndpoint)

	}
	dubboInterfaceConfig.ServiceType = DubboService
	return host, &dubboInterfaceConfig, nil
}

func (w *watcher) startScheduleWatchTask(serviceConfig ListServiceConfig, oldChildren []string, ttl time.Duration, childEventCh <-chan zk.Event, exit chan struct{}) bool {
	zkRootPath := serviceConfig.UrlIndex
	interfaceName := serviceConfig.InterfaceName
	serviceType := serviceConfig.ServiceType
	tickerTTL := ttl
	if tickerTTL > 20e9 {
		tickerTTL = 20e9
	}
	ticker := time.NewTicker(tickerTTL)
	for {
		select {
		case <-ticker.C:
			w.handleZkNodeEvent(zkRootPath, oldChildren, interfaceName, serviceType)
			if tickerTTL < ttl {
				tickerTTL *= 2
				if tickerTTL > ttl {
					tickerTTL = ttl
				}
				ticker.Stop()
				ticker = time.NewTicker(tickerTTL)
			}
		case zkEvent := <-childEventCh:
			if zkEvent.Type == zk.EventNodeChildrenChanged {
				w.handleZkNodeEvent(zkEvent.Path, oldChildren, interfaceName, serviceType)
			}
			return false
		case <-exit:
			ticker.Stop()
			return true
		}
	}
}

func (w *watcher) handleZkNodeEvent(zkPath string, oldChildren []string, interfaceName string, serviceType ServiceType) {
	newChildren, err := w.zkClient.GetChildren(zkPath)
	if err != nil {
		if err == gxzookeeper.ErrNilChildren || err == gxzookeeper.ErrNilNode ||
			strings.Contains(err.Error(), "has none children") {
			content, _, connErr := w.zkClient.Conn.Get(zkPath)
			if connErr != nil {
				log.Errorf("[handleZkNodeEvent] Get new node path's content error:%v, path:%s", connErr, zkPath)
			} else {
				for _, c := range oldChildren {
					path := path.Join(zkPath, c)
					content, _, connErr = w.zkClient.Conn.Get(path)
					if connErr != nil {
						log.Errorf("[handleZkNodeEvent] Get node path's content error:%v, path:%s", connErr, path)
						continue
					}
					w.DataChange(Event{
						Path:          path,
						Action:        EventTypeDel,
						Content:       content,
						InterfaceName: interfaceName,
						ServiceType:   serviceType,
					})
				}
			}
		} else {
			log.Errorf("zkClient get children failed, err:%v", err)
		}
		return
	}
	w.ChildToServiceEntry(newChildren, interfaceName, zkPath, serviceType)
}

func (w *watcher) ChildToServiceEntry(children []string, interfaceName, zkPath string, serviceType ServiceType) {
	serviceEntry := make(map[string]InterfaceConfig)
	switch serviceType {
	case DubboService:
		w.DubboChildToServiceEntry(serviceEntry, children, interfaceName, zkPath)
	case SpringCloudService:
		w.SpringCloudChildToServiceEntry(serviceEntry, children, interfaceName, zkPath)
	default:
		log.Error("unknown type")
	}
	if len(serviceEntry) != 0 {
		w.seMux.Lock()
		for host, config := range serviceEntry {
			se := w.generateServiceEntry(config)
			value, ok := w.serviceEntry[host]
			if ok {
				if !reflect.DeepEqual(value, config) {
					w.serviceEntry[host] = config
					// todo update or create serviceentry
					w.cache.UpdateServiceWrapper(host, &ingress.ServiceWrapper{
						ServiceName:  host,
						ServiceEntry: se,
						Suffix:       "zookeeper",
						RegistryType: w.Type,
						RegistryName: w.Name,
					})
				}
			} else {
				w.serviceEntry[host] = config
				w.cache.UpdateServiceWrapper(host, &ingress.ServiceWrapper{
					ServiceName:  host,
					ServiceEntry: se,
					Suffix:       "zookeeper",
					RegistryType: w.Type,
					RegistryName: w.Name,
				})
			}
		}
		w.seMux.Unlock()
		w.UpdateService()
	}
}

func (w *watcher) SpringCloudChildToServiceEntry(serviceEntry map[string]InterfaceConfig, children []string, interfaceName, zkPath string) {
	for _, c := range children {
		path := path.Join(zkPath, c)
		content, _, err := w.zkClient.Conn.Get(path)
		if err != nil {
			log.Errorf("[handleZkNodeEvent] Get node path's content error:%v, path:%s", err, path)
			continue
		}
		host, config, err := w.GetSpringCloudConfig(interfaceName, content)
		if err != nil {
			log.Errorf("GetSpringCloudConfig failed:%v", err)
			continue
		}
		if existConfig, exist := serviceEntry[host]; !exist {
			serviceEntry[host] = *config
		} else {
			existConfig.Endpoints = append(existConfig.Endpoints, config.Endpoints...)
			serviceEntry[host] = existConfig
		}
	}
}

func (w *watcher) DubboChildToServiceEntry(serviceEntry map[string]InterfaceConfig, children []string, interfaceName, zkPath string) {
	for _, c := range children {
		path := path.Join(zkPath, c)
		host, config, err := w.GetDubboConfig(path)
		if err != nil {
			log.Errorf("GetDubboConfig failed:%v", err)
			continue
		}
		if existConfig, exist := serviceEntry[host]; !exist {
			serviceEntry[host] = *config
		} else {
			existConfig.Endpoints = append(existConfig.Endpoints, config.Endpoints...)
			serviceEntry[host] = existConfig
		}
	}
}

func (w *watcher) generateServiceEntry(config InterfaceConfig) *v1alpha3.ServiceEntry {
	portList := make([]*v1alpha3.ServicePort, 0)
	endpoints := make([]*v1alpha3.WorkloadEntry, 0)

	for _, service := range config.Endpoints {
		protocol := common.HTTP
		if service.Metadata != nil && service.Metadata[PROTOCOL] != "" {
			protocol = common.ParseProtocol(service.Metadata[PROTOCOL])
		}
		portNumber, _ := strconv.Atoi(service.Port)
		port := &v1alpha3.ServicePort{
			Name:     protocol.String(),
			Number:   uint32(portNumber),
			Protocol: protocol.String(),
		}
		if len(portList) == 0 {
			portList = append(portList, port)
		}
		endpoints = append(endpoints, &v1alpha3.WorkloadEntry{
			Address: service.Ip,
			Ports:   map[string]uint32{port.Protocol: port.Number},
			Labels:  service.Metadata,
			Weight:  1,
		})
	}

	se := &v1alpha3.ServiceEntry{
		Hosts:      []string{config.Host + ".zookeeper"},
		Ports:      portList,
		Location:   v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: v1alpha3.ServiceEntry_STATIC,
		Endpoints:  endpoints,
	}
	return se
}

func (w *watcher) Run() {
	defer func() {
		log.Info("[zookeeper] Run is down")
		if r := recover(); r != nil {
			log.Infof("Recovered in Run: %v", r)
		}
	}()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	w.Status = provider.ProbeWatcherStatus(w.Domain, strconv.FormatUint(uint64(w.Port), 10))
	go func() {
		w.ListenService()
	}()
	firstFetchErr := w.fetchAllServices(true)
	if firstFetchErr != nil {
		log.Errorf("first fetch services failed:%v", firstFetchErr)
	}
	for {
		select {
		case <-ticker.C:
			var needNewFetch bool
			if w.watcherReady() {
				w.Ready(true)
				needNewFetch = true
			}
			if firstFetchErr != nil || needNewFetch {
				firstFetchErr = w.fetchAllServices()
			}
		case <-w.stop:
			return
		case <-w.listServiceChan:
			go func() {
				w.ListenService()
			}()
		}
	}
}

func (w *watcher) Stop() {
	w.mutex.Lock()
	for key, value := range w.WatchingServices {
		if value.exit != nil {
			close(value.exit)
		}
		delete(w.WatchingServices, key)
	}
	w.isStop = true
	w.mutex.Unlock()

	w.seMux.Lock()
	for key := range w.serviceEntry {
		w.cache.DeleteServiceWrapper(key)
	}
	w.UpdateService()
	w.seMux.Unlock()

	close(w.stop)
	close(w.Done)
	w.zkClient.Close()
	w.Ready(false)
}

func (w *watcher) IsHealthy() bool {
	return w.Status == provider.Healthy
}

func (w *watcher) GetRegistryType() string {
	return w.RegistryType.String()
}

func (w *watcher) watcherReady() bool {
	if w.serviceRemaind == nil {
		return true
	}
	remaind := w.serviceRemaind.Load()
	if remaind <= 0 {
		return true
	}
	return false
}

func timeSecondDuration(sec int) time.Duration {
	return time.Duration(sec) * time.Second
}
