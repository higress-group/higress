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

package reconcile

import (
	"context"
	"errors"
	"fmt"
	"path"
	"reflect"
	"sync"
	"time"

	"istio.io/istio/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	v1 "github.com/alibaba/higress/v2/client/pkg/apis/networking/v1"
	higressmcpserver "github.com/alibaba/higress/v2/pkg/ingress/kube/mcpserver"
	"github.com/alibaba/higress/v2/pkg/kube"
	. "github.com/alibaba/higress/v2/registry"
	"github.com/alibaba/higress/v2/registry/consul"
	"github.com/alibaba/higress/v2/registry/direct"
	"github.com/alibaba/higress/v2/registry/eureka"
	"github.com/alibaba/higress/v2/registry/memory"
	"github.com/alibaba/higress/v2/registry/nacos"
	nacosv2 "github.com/alibaba/higress/v2/registry/nacos/v2"
	"github.com/alibaba/higress/v2/registry/proxy"
	"github.com/alibaba/higress/v2/registry/zookeeper"
)

const (
	DefaultReadyTimeout = time.Second * 60
)

type Reconciler struct {
	memory.Cache
	registries    map[string]*apiv1.RegistryConfig
	proxies       map[string]*apiv1.ProxyConfig
	watchers      map[string]Watcher
	serviceUpdate func()
	client        kube.Client
	namespace     string
	clusterId     string
}

func NewReconciler(serviceUpdate func(), client kube.Client, namespace, clusterId string) *Reconciler {
	return &Reconciler{
		Cache:         memory.NewCache(),
		registries:    make(map[string]*apiv1.RegistryConfig),
		proxies:       make(map[string]*apiv1.ProxyConfig),
		watchers:      make(map[string]Watcher),
		serviceUpdate: serviceUpdate,
		client:        client,
		namespace:     namespace,
		clusterId:     clusterId,
	}
}

func (r *Reconciler) Reconcile(mcpbridge *v1.McpBridge) error {
	var registries []*apiv1.RegistryConfig
	var proxies []*apiv1.ProxyConfig

	if mcpbridge != nil {
		if proxy.NeedToFillProxyListenerPorts(mcpbridge.Spec.Proxies) {
			// Make a deep copy of the McpBridge resource to avoid modifying the original one
			mcpBridgeForUpdate := mcpbridge.DeepCopy()
			if proxy.FillProxyListenerPorts(mcpBridgeForUpdate.Spec.Proxies) {
				// Some listener ports are filled, we need to update the resource and reconcile again
				mcpBridgeClient := r.client.Higress().NetworkingV1().McpBridges(mcpBridgeForUpdate.Namespace)
				if _, err := mcpBridgeClient.Update(context.Background(), mcpBridgeForUpdate, metav1.UpdateOptions{}); err != nil {
					return fmt.Errorf("failed to save filled proxy listener ports: %v", err)
				}
				return nil
			}
		}

		registries = mcpbridge.Spec.Registries
		proxies = mcpbridge.Spec.Proxies
	}

	if err := r.reconcileRegistries(registries); err != nil {
		return err
	}
	if err := r.reconcileProxies(proxies); err != nil {
		return err
	}

	if r.Cache.PurgeStaleItems() {
		// Something stale are purged. We need to notify the service update handler
		r.serviceUpdate()
	}
	return nil
}

func (r *Reconciler) reconcileRegistries(registries []*apiv1.RegistryConfig) error {
	newRegistries := make(map[string]*apiv1.RegistryConfig)
	for _, registry := range registries {
		newRegistries[path.Join(registry.Type, registry.Name)] = registry
	}
	var wg sync.WaitGroup
	toBeCreated := make(map[string]*apiv1.RegistryConfig)
	toBeUpdated := make(map[string]*apiv1.RegistryConfig)
	toBeDeleted := make(map[string]*apiv1.RegistryConfig)

	for key, newRegistry := range newRegistries {
		if oldRegistry, ok := r.registries[key]; !ok {
			toBeCreated[key] = newRegistry
		} else if reflect.DeepEqual(newRegistry, oldRegistry) {
			continue
		} else {
			toBeUpdated[key] = newRegistry
		}
	}

	for key, oldRegistry := range r.registries {
		if _, ok := newRegistries[key]; !ok {
			toBeDeleted[key] = oldRegistry
		}
	}
	errHappened := false
	log.Infof("ReconcileRegistries, toBeCreated: %d, toBeUpdated: %d, toBeDeleted: %d",
		len(toBeCreated), len(toBeUpdated), len(toBeDeleted))
	for k := range toBeDeleted {
		r.watchers[k].Stop()
		delete(r.registries, k)
		delete(r.watchers, k)
	}
	for k, v := range toBeUpdated {
		r.watchers[k].Stop()
		delete(r.registries, k)
		delete(r.watchers, k)
		watcher, err := r.generateWatcherFromRegistryConfig(v, &wg)
		if err != nil {
			errHappened = true
			log.Errorf("ReconcileRegistries failed, err:%v", err)
			continue
		}

		go watcher.Run()
		r.watchers[k] = watcher
		r.registries[k] = v
	}
	for k, v := range toBeCreated {
		watcher, err := r.generateWatcherFromRegistryConfig(v, &wg)
		if err != nil {
			errHappened = true
			log.Errorf("ReconcileRegistries failed, err:%v", err)
			continue
		}

		go watcher.Run()
		r.watchers[k] = watcher
		r.registries[k] = v
	}
	if errHappened {
		return errors.New("ReconcileRegistries failed, Init Watchers failed")
	}
	ready := make(chan struct{})
	readyTimer := time.NewTimer(DefaultReadyTimeout)
	go func() {
		wg.Wait()
		ready <- struct{}{}
	}()
	select {
	case <-ready:
	case <-readyTimer.C:
		return errors.New("ReoncileRegistries failed, waiting for ready timeout")
	}
	log.Infof("Registries is reconciled")
	return nil
}

func (r *Reconciler) generateWatcherFromRegistryConfig(registry *apiv1.RegistryConfig, wg *sync.WaitGroup) (Watcher, error) {
	var watcher Watcher
	var err error

	authOption, err := r.getAuthOption(registry)
	if err != nil {
		return nil, err
	}

	switch registry.Type {
	case string(Nacos):
		watcher, err = nacos.NewWatcher(
			r.Cache,
			nacos.WithType(registry.Type),
			nacos.WithName(registry.Name),
			nacos.WithDomain(registry.Domain),
			nacos.WithPort(registry.Port),
			nacos.WithNacosNamespaceId(registry.NacosNamespaceId),
			nacos.WithNacosNamespace(registry.NacosNamespace),
			nacos.WithNacosGroups(registry.NacosGroups),
			nacos.WithNacosRefreshInterval(registry.NacosRefreshInterval),
			nacos.WithAuthOption(authOption),
			nacos.WithVport(registry.Vport),
		)
	case string(Nacos2), string(Nacos3):
		watcher, err = nacosv2.NewWatcher(
			r.Cache,
			nacosv2.WithType(registry.Type),
			nacosv2.WithName(registry.Name),
			nacosv2.WithNacosAddressServer(registry.NacosAddressServer),
			nacosv2.WithDomain(registry.Domain),
			nacosv2.WithPort(registry.Port),
			nacosv2.WithNacosAccessKey(registry.NacosAccessKey),
			nacosv2.WithNacosSecretKey(registry.NacosSecretKey),
			nacosv2.WithNacosNamespaceId(registry.NacosNamespaceId),
			nacosv2.WithNacosNamespace(registry.NacosNamespace),
			nacosv2.WithNacosGroups(registry.NacosGroups),
			nacosv2.WithNacosRefreshInterval(registry.NacosRefreshInterval),
			nacosv2.WithMcpExportDomains(registry.McpServerExportDomains),
			nacosv2.WithMcpBaseUrl(registry.McpServerBaseUrl),
			nacosv2.WithEnableMcpServer(registry.EnableMCPServer),
			nacosv2.WithClusterId(r.clusterId),
			nacosv2.WithNamespace(r.namespace),
			nacosv2.WithAuthOption(authOption),
			nacosv2.WithVport(registry.Vport),
		)
	case string(Zookeeper):
		watcher, err = zookeeper.NewWatcher(
			r.Cache,
			zookeeper.WithType(registry.Type),
			zookeeper.WithName(registry.Name),
			zookeeper.WithDomain(registry.Domain),
			zookeeper.WithPort(registry.Port),
			zookeeper.WithZkServicesPath(registry.ZkServicesPath),
		)
	case string(Consul):
		watcher, err = consul.NewWatcher(
			r.Cache,
			consul.WithType(registry.Type),
			consul.WithName(registry.Name),
			consul.WithDomain(registry.Domain),
			consul.WithPort(registry.Port),
			consul.WithDatacenter(registry.ConsulDatacenter),
			consul.WithServiceTag(registry.ConsulServiceTag),
			consul.WithRefreshInterval(registry.ConsulRefreshInterval),
			consul.WithAuthOption(authOption),
		)
	case string(Static), string(DNS):
		watcher, err = direct.NewWatcher(
			r.Cache,
			direct.WithType(registry.Type),
			direct.WithName(registry.Name),
			direct.WithDomain(registry.Domain),
			direct.WithPort(registry.Port),
			direct.WithProtocol(registry.Protocol),
			direct.WithSNI(registry.Sni),
			direct.WithProxyName(registry.ProxyName),
		)
	case string(Eureka):
		watcher, err = eureka.NewWatcher(
			r.Cache,
			eureka.WithName(registry.Name),
			eureka.WithDomain(registry.Domain),
			eureka.WithType(registry.Type),
			eureka.WithPort(registry.Port),
			eureka.WithVport(registry.Vport),
		)
	default:
		return nil, errors.New("unsupported registry type:" + registry.Type)
	}

	if err != nil {
		return nil, err
	}

	wg.Add(1)
	var once sync.Once
	watcher.ReadyHandler(func(ready bool) {
		once.Do(func() {
			wg.Done()
			if ready {
				log.Infof("Registry Watcher is ready, type:%s, name:%s", registry.Type, registry.Name)
			}
		})
	})
	watcher.AppendServiceUpdateHandler(r.serviceUpdate)

	return watcher, nil
}

func (r *Reconciler) getAuthOption(registry *apiv1.RegistryConfig) (AuthOption, error) {
	authOption := AuthOption{}
	authSecretName := registry.AuthSecretName

	if len(authSecretName) == 0 {
		return authOption, nil
	}

	authSecret, err := r.client.Kube().CoreV1().Secrets(r.namespace).Get(context.Background(), authSecretName, metav1.GetOptions{})
	if err != nil {
		return authOption, errors.New(fmt.Sprintf("get auth secret %s in namespace %s error:%v", authSecretName, r.namespace, err))
	}

	if nacosUsername, ok := authSecret.Data[AuthNacosUsernameKey]; ok {
		authOption.NacosUsername = string(nacosUsername)
	}

	if nacosPassword, ok := authSecret.Data[AuthNacosPasswordKey]; ok {
		authOption.NacosPassword = string(nacosPassword)
	}

	if consulToken, ok := authSecret.Data[AuthConsulTokenKey]; ok {
		authOption.ConsulToken = string(consulToken)
	}

	if etcdUsername, ok := authSecret.Data[AuthEtcdUsernameKey]; ok {
		authOption.EtcdUsername = string(etcdUsername)
	}

	if etcdPassword, ok := authSecret.Data[AuthEtcdPasswordKey]; ok {
		authOption.EtcdPassword = string(etcdPassword)
	}

	return authOption, nil
}

func (r *Reconciler) reconcileProxies(proxies []*apiv1.ProxyConfig) error {
	newProxies := make(map[string]*apiv1.ProxyConfig)
	for _, p := range proxies {
		newProxies[p.Name] = p
	}

	toBeUpdated := make(map[string]*apiv1.ProxyConfig)
	toBeDeleted := make(map[string]*apiv1.ProxyConfig)

	for key, newProxy := range newProxies {
		if oldProxy, ok := r.proxies[key]; !ok || !reflect.DeepEqual(newProxy, oldProxy) {
			toBeUpdated[key] = newProxy
		}
	}

	for key, oldProxy := range r.proxies {
		if _, ok := newProxies[key]; !ok {
			toBeDeleted[key] = oldProxy
		}
	}

	log.Infof("ReconcileProxies, toBeUpdated: %d, toBeDeleted: %d",
		len(toBeUpdated), len(toBeDeleted))

	needNotify := false

	for k := range toBeDeleted {
		r.Cache.DeleteProxyWrapper(k)
		delete(r.proxies, k)
		needNotify = true
	}
	for k, v := range toBeUpdated {
		proxyWrapper := proxy.BuildProxyWrapper(v)
		if proxyWrapper == nil {
			continue
		}
		r.Cache.UpdateProxyWrapper(k, proxyWrapper)
		r.proxies[k] = v
		needNotify = true
	}

	if needNotify {
		r.serviceUpdate()
	}

	log.Infof("Proxies are reconciled")
	return nil
}

func (r *Reconciler) GetMcpServers() []*higressmcpserver.McpServer {
	mcpServersFromMcp := r.GetAllConfigs(higressmcpserver.GvkMcpServer)
	servers := make([]*higressmcpserver.McpServer, 0, len(mcpServersFromMcp))
	for _, c := range mcpServersFromMcp {
		if server, ok := c.Spec.(*higressmcpserver.McpServer); ok {
			servers = append(servers, server)
		}
	}
	return servers
}

type RegistryWatcherStatus struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Healthy bool   `json:"healthy"`
	Ready   bool   `json:"ready"`
}

func (r *Reconciler) GetRegistryWatcherStatusList() []RegistryWatcherStatus {
	var registryStatusList []RegistryWatcherStatus
	for key, watcher := range r.watchers {
		_, name := path.Split(key)
		registryStatus := RegistryWatcherStatus{
			Name:    name,
			Type:    watcher.GetRegistryType(),
			Healthy: watcher.IsHealthy(),
			Ready:   watcher.IsReady(),
		}
		registryStatusList = append(registryStatusList, registryStatus)
	}
	return registryStatusList
}
