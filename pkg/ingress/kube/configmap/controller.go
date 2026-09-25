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

package configmap

import (
	"reflect"
	"sync/atomic"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/config/schema/kind"
	schemakubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	kubeclient "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	ktypes "istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/apimachinery/pkg/types"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/controller"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/mcpserver"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
)

type HigressConfigController controller.Controller[listersv1.ConfigMapNamespaceLister]

func NewController(client kubeclient.Client, clusterId cluster.ID, namespace string) HigressConfigController {
	opts := ktypes.InformerOptions{
		Namespace: namespace,
		Cluster:   clusterId,
	}
	informer := schemakubeclient.GetInformerFilteredFromGVR(client, opts, gvr.ConfigMap)
	lister := listersv1.NewConfigMapLister(informer.Informer.GetIndexer()).ConfigMaps(namespace)
	return controller.NewCommonController("higressConfig", lister, informer.Informer, GetConfigmap, clusterId)
}

func GetConfigmap(lister listersv1.ConfigMapNamespaceLister, namespacedName types.NamespacedName) (controllers.Object, error) {
	return lister.Get(namespacedName.Name)
}

type ItemController interface {
	GetName() string
	AddOrUpdateHigressConfig(name util.ClusterNamespacedName, old *HigressConfig, new *HigressConfig) error
	ValidHigressConfig(higressConfig *HigressConfig) error
	ConstructEnvoyFilters() ([]*config.Config, error)
	RegisterItemEventHandler(eventHandler ItemEventHandler)
}

type ConfigmapMgr struct {
	Namespace               string
	HigressConfigController HigressConfigController
	HigressConfigLister     listersv1.ConfigMapNamespaceLister
	higressConfig           atomic.Value
	ItemControllers         []ItemController
	XDSUpdater              model.XDSUpdater
}

func NewConfigmapMgr(XDSUpdater model.XDSUpdater, namespace string, higressConfigController HigressConfigController, higressConfigLister listersv1.ConfigMapNamespaceLister) *ConfigmapMgr {
	configmapMgr := &ConfigmapMgr{
		XDSUpdater:              XDSUpdater,
		Namespace:               namespace,
		HigressConfigController: higressConfigController,
		HigressConfigLister:     higressConfigLister,
		higressConfig:           atomic.Value{},
	}
	configmapMgr.HigressConfigController.AddEventHandler(configmapMgr.AddOrUpdateHigressConfig)
	configmapMgr.SetHigressConfig(NewDefaultHigressConfig())

	tracingController := NewTracingController(namespace)
	configmapMgr.AddItemControllers(tracingController)

	gzipController := NewGzipController(namespace)
	configmapMgr.AddItemControllers(gzipController)

	globalOptionController := NewGlobalOptionController(namespace)
	configmapMgr.AddItemControllers(globalOptionController)

	mcpServerController := NewMcpServerController(namespace)
	configmapMgr.AddItemControllers(mcpServerController)

	configmapMgr.initEventHandlers()

	return configmapMgr
}

func (c *ConfigmapMgr) SetHigressConfig(higressConfig *HigressConfig) {
	c.higressConfig.Store(higressConfig)
}

func (c *ConfigmapMgr) GetHigressConfig() *HigressConfig {
	value := c.higressConfig.Load()
	if value != nil {
		if higressConfig, ok := value.(*HigressConfig); ok {
			return higressConfig
		}
	}
	return nil
}

func (c *ConfigmapMgr) RegisterMcpServerProvider(provider mcpserver.McpServerProvider) {
	for _, itemController := range c.ItemControllers {
		if mcpRouteProviderAware, ok := itemController.(mcpserver.McpRouteProviderAware); ok {
			mcpRouteProviderAware.RegisterMcpServerProvider(provider)
		}
	}
}

// GetUpstream returns the current global upstream option, or nil if unavailable.
func (c *ConfigmapMgr) GetUpstream() *Upstream {
	for _, itemController := range c.ItemControllers {
		if g, ok := itemController.(*GlobalOptionController); ok {
			if global := g.GetGlobal(); global != nil {
				return global.Upstream
			}
		}
	}
	return nil
}

func (c *ConfigmapMgr) AddItemControllers(controllers ...ItemController) {
	c.ItemControllers = append(c.ItemControllers, controllers...)
}

func (c *ConfigmapMgr) AddOrUpdateHigressConfig(name util.ClusterNamespacedName) {
	if name.Namespace != c.Namespace || name.Name != HigressConfigMapName {
		return
	}

	IngressLog.Infof("configmapMgr AddOrUpdateHigressConfig")
	higressConfigmap, err := c.HigressConfigLister.Get(HigressConfigMapName)
	if err != nil {
		IngressLog.Errorf("higress-config configmap is not found, namespace:%s, name:%s",
			name.Namespace, name.Name)
		return
	}

	if _, ok := higressConfigmap.Data[HigressConfigMapKey]; !ok {
		return
	}

	newHigressConfig := NewDefaultHigressConfig()
	if err = yaml.Unmarshal([]byte(higressConfigmap.Data[HigressConfigMapKey]), newHigressConfig); err != nil {
		IngressLog.Errorf("data:%s,  convert to higress config error, error: %+v", higressConfigmap.Data[HigressConfigMapKey], err)
		return
	}

	for _, itemController := range c.ItemControllers {
		if itemErr := itemController.ValidHigressConfig(newHigressConfig); itemErr != nil {
			IngressLog.Errorf("configmap %s controller valid higress config error, error: %+v", itemController.GetName(), itemErr)
			return
		}
	}

	oldHigressConfig := c.GetHigressConfig()
	IngressLog.Infof("configmapMgr oldHigressConfig: %s", GetHigressConfigString(oldHigressConfig))
	IngressLog.Infof("configmapMgr newHigressConfig: %s", GetHigressConfigString(newHigressConfig))
	result, _ := c.CompareHigressConfig(oldHigressConfig, newHigressConfig)
	IngressLog.Infof("configmapMgr CompareHigressConfig result is %d", result)

	if result == ResultNothing {
		return
	}

	if result == ResultDelete {
		newHigressConfig = NewDefaultHigressConfig()
	}

	if result == ResultReplace || result == ResultDelete {
		// Pass AddOrUpdateHigressConfig to itemControllers
		for _, itemController := range c.ItemControllers {
			IngressLog.Infof("configmap %s controller AddOrUpdateHigressConfig", itemController.GetName())
			if itemErr := itemController.AddOrUpdateHigressConfig(name, oldHigressConfig, newHigressConfig); itemErr != nil {
				IngressLog.Errorf("configmap %s controller AddOrUpdateHigressConfig error, error: %+v", itemController.GetName(), itemErr)
			}
		}
		c.SetHigressConfig(newHigressConfig)
		IngressLog.Infof("configmapMgr higress config AddOrUpdate success, result is %d", result)
		// Call updateConfig
	}
}

func (c *ConfigmapMgr) ConstructEnvoyFilters() ([]*config.Config, error) {
	configs := make([]*config.Config, 0)
	for _, itemController := range c.ItemControllers {
		IngressLog.Infof("controller %s ConstructEnvoyFilters", itemController.GetName())
		if itemConfigs, err := itemController.ConstructEnvoyFilters(); err != nil {
			IngressLog.Errorf("controller %s ConstructEnvoyFilters error, error: %+v", itemController.GetName(), err)
		} else {
			configs = append(configs, itemConfigs...)
		}
	}
	return configs, nil
}

func (c *ConfigmapMgr) CompareHigressConfig(old *HigressConfig, new *HigressConfig) (Result, error) {
	if old == nil || new == nil {
		return ResultNothing, nil
	}

	if !reflect.DeepEqual(old, new) {
		return ResultReplace, nil
	}

	return ResultNothing, nil
}

func (c *ConfigmapMgr) initEventHandlers() error {
	itemEventHandler := func(name string) {
		c.XDSUpdater.ConfigUpdate(&model.PushRequest{
			Full: true,
			ConfigsUpdated: map[model.ConfigKey]struct{}{{
				Kind:      kind.EnvoyFilter,
				Name:      name,
				Namespace: c.Namespace,
			}: {}},
			Reason: model.NewReasonStats(ModelUpdatedReason),
		})
	}

	for _, itemController := range c.ItemControllers {
		itemController.RegisterItemEventHandler(itemEventHandler)
	}

	return nil
}
