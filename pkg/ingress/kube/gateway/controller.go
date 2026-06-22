// Copyright (c) 2023 Alibaba Group Holding Ltd.
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

package gateway

import (
	"istio.io/istio/pilot/pkg/features"
	"sync/atomic"

	"istio.io/istio/pilot/pkg/config/kube/crdclient"
	"istio.io/istio/pilot/pkg/model"
	kubecontroller "istio.io/istio/pilot/pkg/serviceregistry/kube/controller"
	"istio.io/istio/pilot/pkg/status"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/schema/collection"
	"istio.io/istio/pkg/config/schema/collections"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/resource"
	"istio.io/istio/pkg/kube"
	"k8s.io/client-go/tools/cache"

	higressconfig "github.com/alibaba/higress/v2/pkg/config"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	istiogateway "github.com/alibaba/higress/v2/pkg/ingress/kube/gateway/istio"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
)

type gatewayController struct {
	virtualServiceHandlers  []model.EventHandler
	gatewayHandlers         []model.EventHandler
	destinationRuleHandlers []model.EventHandler
	envoyFilterHandlers     []model.EventHandler

	store           model.ConfigStoreController
	istioController *istiogateway.Controller
	statusManager   *status.Manager

	resourceUpToDate atomic.Bool
}

func NewController(client kube.Client, options common.Options, xdsUpdater model.XDSUpdater) common.GatewayController {
	domainSuffix := util.GetDomainSuffix()
	opts := crdclient.Option{
		Revision:     higressconfig.Revision,
		DomainSuffix: domainSuffix,
		Identifier:   "gateway-controller",
	}
	schemasBuilder := collection.NewSchemasBuilder()
	collections.PilotGatewayAPI().ForEach(func(schema resource.Schema) bool {
		if schema.Group() == collections.GatewayClass.Group() {
			schemasBuilder.MustAdd(schema)
		}
		return false
	})
	// Add gateway api inference schema if enabled
	if features.EnableGatewayAPIInferenceExtension {
		schemasBuilder.MustAdd(collections.InferencePool)
	}
	store := crdclient.NewForSchemas(client, opts, schemasBuilder.Build())

	clusterId := options.ClusterId
	opt := kubecontroller.Options{
		DomainSuffix: domainSuffix,
		ClusterID:    clusterId,
		Revision:     higressconfig.Revision,
	}
	istiogateway.SetGatewayClassName(options.GatewayClass)
	istioController := istiogateway.NewController(client, client.CrdWatcher().WaitForCRD, opt, xdsUpdater)
	if options.GatewaySelectorKey != "" {
		istioController.DefaultGatewaySelector = map[string]string{options.GatewaySelectorKey: options.GatewaySelectorValue}
	}

	var statusManager *status.Manager = nil
	if options.EnableStatus {
		statusManager = status.NewManager(store)
		istioController.SetStatusWrite(true, statusManager)
	} else {
		IngressLog.Infof("Disable status update for cluster %s", clusterId)
	}

	return &gatewayController{
		store:           store,
		istioController: istioController,
		statusManager:   statusManager,
	}
}

func (g *gatewayController) Schemas() collection.Schemas {
	return g.istioController.Schemas()
}

func (g *gatewayController) Get(typ config.GroupVersionKind, name, namespace string) *config.Config {
	return g.istioController.Get(typ, name, namespace)
}

func (g *gatewayController) List(typ config.GroupVersionKind, namespace string) []config.Config {
	if g.resourceUpToDate.CompareAndSwap(false, true) {
		g.istioController.Reconcile(model.NewPushContext())
	}
	return g.istioController.List(typ, namespace)
}

func (g *gatewayController) Create(config config.Config) (revision string, err error) {
	return g.istioController.Create(config)
}

func (g *gatewayController) Update(config config.Config) (newRevision string, err error) {
	return g.istioController.Update(config)
}

func (g *gatewayController) UpdateStatus(config config.Config) (newRevision string, err error) {
	return g.istioController.UpdateStatus(config)
}

func (g *gatewayController) Patch(orig config.Config, patchFn config.PatchFunc) (string, error) {
	return g.istioController.Patch(orig, patchFn)
}

func (g *gatewayController) Delete(typ config.GroupVersionKind, name, namespace string, resourceVersion *string) error {
	return g.istioController.Delete(typ, name, namespace, resourceVersion)
}

func (g *gatewayController) RegisterEventHandler(kind config.GroupVersionKind, f model.EventHandler) {
	switch kind {
	case gvk.VirtualService:
		g.virtualServiceHandlers = append(g.virtualServiceHandlers, f)
	case gvk.Gateway:
		g.gatewayHandlers = append(g.gatewayHandlers, f)
	case gvk.DestinationRule:
		g.destinationRuleHandlers = append(g.destinationRuleHandlers, f)
	case gvk.EnvoyFilter:
		g.envoyFilterHandlers = append(g.envoyFilterHandlers, f)
	}
}

func (g *gatewayController) Run(stop <-chan struct{}) {
	g.store.Schemas().ForEach(func(schema resource.Schema) bool {
		g.store.RegisterEventHandler(schema.GroupVersionKind(), g.onEvent)
		return false
	})
	go g.store.Run(stop)
	go g.istioController.Run(stop)
	if g.statusManager != nil {
		g.statusManager.Start(stop)
	}
}

func (g *gatewayController) SetWatchErrorHandler(f func(r *cache.Reflector, err error)) error {
	// TODO: implement
	return nil
}

func (g *gatewayController) HasSynced() bool {
	ret := g.istioController.HasSynced()
	if ret {
		g.istioController.Reconcile(model.NewPushContext())
	}
	return ret
}

func (g *gatewayController) onEvent(prev config.Config, curr config.Config, event model.Event) {
	g.resourceUpToDate.Store(false)

	name := "gateway-api"
	namespace := curr.Namespace

	vsMetadata := config.Meta{
		Name:             name + "-" + "virtualservice",
		Namespace:        namespace,
		GroupVersionKind: gvk.VirtualService,
		// Set this label so that we do not compare configs and just push.
		Labels: map[string]string{constants.AlwaysPushLabel: "true"},
	}
	gatewayMetadata := config.Meta{
		Name:             name + "-" + "gateway",
		Namespace:        namespace,
		GroupVersionKind: gvk.Gateway,
		// Set this label so that we do not compare configs and just push.
		Labels: map[string]string{constants.AlwaysPushLabel: "true"},
	}

	for _, f := range g.virtualServiceHandlers {
		f(config.Config{Meta: vsMetadata}, config.Config{Meta: vsMetadata}, event)
	}

	for _, f := range g.gatewayHandlers {
		f(config.Config{Meta: gatewayMetadata}, config.Config{Meta: gatewayMetadata}, event)
	}
}
