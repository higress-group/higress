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

package ingressv1

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/model"
	istiomodel "istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/model/credentials"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	schemakubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	kubeclient "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/informerfactory"
	ktypes "istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/util/sets"
	ingress "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	listerv1 "k8s.io/client-go/listers/core/v1"
	networkinglister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/alibaba/higress/v2/pkg/cert"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/annotations"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/secret"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	_ common.IngressController = &controller{}

	// follow specification of ingress-nginx
	defaultPathType = ingress.PathTypePrefix
)

type controller struct {
	queue                   controllers.Queue
	virtualServiceHandlers  []istiomodel.EventHandler
	gatewayHandlers         []istiomodel.EventHandler
	destinationRuleHandlers []istiomodel.EventHandler
	envoyFilterHandlers     []istiomodel.EventHandler

	options common.Options

	mutex sync.RWMutex
	// key: namespace/name
	ingresses map[string]*ingress.Ingress

	ingressInformer informerfactory.StartableInformer
	ingressLister   networkinglister.IngressLister
	serviceInformer informerfactory.StartableInformer
	serviceLister   listerv1.ServiceLister

	classInformer informerfactory.StartableInformer
	classLister   networkinglister.IngressClassLister

	secretController secret.SecretController

	statusSyncer *statusSyncer
}

// NewController creates a new Kubernetes controller
func NewController(localKubeClient, client kubeclient.Client, options common.Options, secretController secret.SecretController) common.IngressController {
	opts := ktypes.InformerOptions{Namespace: options.WatchNamespace}
	ingressInformer := schemakubeclient.GetInformerFilteredFromGVR(client, opts, gvr.Ingress)
	ingressLister := networkinglister.NewIngressLister(ingressInformer.Informer.GetIndexer())
	serviceInformer := schemakubeclient.GetInformerFilteredFromGVR(client, opts, gvr.Service)
	serviceLister := listerv1.NewServiceLister(serviceInformer.Informer.GetIndexer())

	classInformer := schemakubeclient.GetInformerFilteredFromGVR(client, opts, gvr.IngressClass)
	classLister := networkinglister.NewIngressClassLister(classInformer.Informer.GetIndexer())

	c := &controller{
		options:          options,
		ingresses:        make(map[string]*ingress.Ingress),
		ingressInformer:  ingressInformer,
		ingressLister:    ingressLister,
		classInformer:    classInformer,
		classLister:      classLister,
		serviceInformer:  serviceInformer,
		serviceLister:    serviceLister,
		secretController: secretController,
	}

	c.queue = controllers.NewQueue("ingressv1",
		controllers.WithReconciler(c.onEvent),
		controllers.WithMaxAttempts(5))
	_, _ = c.ingressInformer.Informer.AddEventHandler(controllers.ObjectHandler(c.queue.AddObject))

	if options.EnableStatus {
		c.statusSyncer = newStatusSyncer(localKubeClient, client, c, options.SystemNamespace, ingressLister, serviceLister)
	} else {
		IngressLog.Infof("Disable status update for cluster %s", options.ClusterId)
	}

	return c
}

func (c *controller) ServiceLister() listerv1.ServiceLister {
	return c.serviceLister
}

func (c *controller) SecretLister() listerv1.SecretLister {
	return c.secretController.Lister()
}

func (c *controller) Run(stop <-chan struct{}) {
	if c.statusSyncer != nil {
		go c.statusSyncer.run(stop)
	}
	go c.secretController.Run(stop)

	defer utilruntime.HandleCrash()

	if !cache.WaitForCacheSync(stop, c.informerSynced) {
		IngressLog.Errorf("Failed to sync ingress controller cache for cluster %s", c.options.ClusterId)
		return
	}

	c.queue.Run(stop)
}

func (c *controller) onEvent(namespacedName types.NamespacedName) error {
	event := istiomodel.EventUpdate
	ing, err := c.ingressLister.Ingresses(namespacedName.Namespace).Get(namespacedName.Name)
	if err != nil {
		if kerrors.IsNotFound(err) {
			event = istiomodel.EventDelete
			c.mutex.Lock()
			ing = c.ingresses[namespacedName.String()]
			delete(c.ingresses, namespacedName.String())
			c.mutex.Unlock()
		} else {
			IngressLog.Warnf("ingressLister Get failed, ingress: %s, err: %v", namespacedName, err)
			return err
		}
	}

	// ingress deleted, and it is not processed before
	if ing == nil {
		return nil
	}

	IngressLog.Infof("ingress: %s, event: %s", namespacedName, event)

	// we should check need process only when event is not delete,
	// if it is delete event, and previously processed, we need to process too.
	if event != istiomodel.EventDelete {
		shouldProcess, err := c.shouldProcessIngressUpdate(ing)
		if err != nil {
			return err
		}
		if !shouldProcess {
			IngressLog.Infof("no need process, ingress: %s", namespacedName)
			return nil
		}
	}

	drmetadata := config.Meta{
		Name:             ing.Name + "-" + "destinationrule",
		Namespace:        ing.Namespace,
		GroupVersionKind: gvk.DestinationRule,
		// Set this label so that we do not compare configs and just push.
		Labels: map[string]string{constants.AlwaysPushLabel: "true"},
	}
	vsmetadata := config.Meta{
		Name:             ing.Name + "-" + "virtualservice",
		Namespace:        ing.Namespace,
		GroupVersionKind: gvk.VirtualService,
		// Set this label so that we do not compare configs and just push.
		Labels: map[string]string{constants.AlwaysPushLabel: "true"},
	}
	efmetadata := config.Meta{
		Name:             ing.Name + "-" + "envoyfilter",
		Namespace:        ing.Namespace,
		GroupVersionKind: gvk.EnvoyFilter,
		// Set this label so that we do not compare configs and just push.
		Labels: map[string]string{constants.AlwaysPushLabel: "true"},
	}
	gatewaymetadata := config.Meta{
		Name:             ing.Name + "-" + "gateway",
		Namespace:        ing.Namespace,
		GroupVersionKind: gvk.Gateway,
		// Set this label so that we do not compare configs and just push.
		Labels: map[string]string{constants.AlwaysPushLabel: "true"},
	}

	for _, f := range c.destinationRuleHandlers {
		f(config.Config{Meta: drmetadata}, config.Config{Meta: drmetadata}, event)
	}

	for _, f := range c.virtualServiceHandlers {
		f(config.Config{Meta: vsmetadata}, config.Config{Meta: vsmetadata}, event)
	}

	for _, f := range c.envoyFilterHandlers {
		f(config.Config{Meta: efmetadata}, config.Config{Meta: efmetadata}, event)
	}

	for _, f := range c.gatewayHandlers {
		f(config.Config{Meta: gatewaymetadata}, config.Config{Meta: gatewaymetadata}, event)
	}

	return nil
}

func (c *controller) RegisterEventHandler(kind config.GroupVersionKind, f istiomodel.EventHandler) {
	switch kind {
	case gvk.VirtualService:
		c.virtualServiceHandlers = append(c.virtualServiceHandlers, f)
	case gvk.Gateway:
		c.gatewayHandlers = append(c.gatewayHandlers, f)
	case gvk.DestinationRule:
		c.destinationRuleHandlers = append(c.destinationRuleHandlers, f)
	case gvk.EnvoyFilter:
		c.envoyFilterHandlers = append(c.envoyFilterHandlers, f)
	}
}

func (c *controller) SetWatchErrorHandler(handler func(r *cache.Reflector, err error)) error {
	var errs error
	if err := c.serviceInformer.Informer.SetWatchErrorHandler(handler); err != nil {
		errs = multierror.Append(errs, err)
	}
	if err := c.ingressInformer.Informer.SetWatchErrorHandler(handler); err != nil {
		errs = multierror.Append(errs, err)
	}
	if err := c.secretController.Informer().SetWatchErrorHandler(handler); err != nil {
		errs = multierror.Append(errs, err)
	}
	if err := c.classInformer.Informer.SetWatchErrorHandler(handler); err != nil {
		errs = multierror.Append(errs, err)
	}
	return errs
}

func (c *controller) informerSynced() bool {
	return c.ingressInformer.Informer.HasSynced() && c.serviceInformer.Informer.HasSynced() &&
		c.classInformer.Informer.HasSynced()
}

func (c *controller) HasSynced() bool {
	return c.queue.HasSynced() && c.secretController.HasSynced()
}

func (c *controller) List() []config.Config {
	out := make([]config.Config, 0, len(c.ingresses))

	for _, raw := range c.ingressInformer.Informer.GetStore().List() {
		ing, ok := raw.(*ingress.Ingress)
		if !ok {
			IngressLog.Warnf("get ingress from informer failed: %v", raw)
			continue
		}

		should, err := c.shouldProcessIngress(ing)
		if err != nil {
			IngressLog.Warnf("check should process ingress failed: %v", err)
			continue
		}
		if !should {
			IngressLog.Debugf("no need process ingress: %s/%s", ing.Namespace, ing.Name)
			continue
		}

		copiedConfig := ing.DeepCopy()
		setDefaultMSEIngressOptionalField(copiedConfig)

		outConfig := config.Config{
			Meta: config.Meta{
				Name:              copiedConfig.Name,
				Namespace:         copiedConfig.Namespace,
				Annotations:       common.CreateOrUpdateAnnotations(copiedConfig.Annotations, c.options),
				Labels:            copiedConfig.Labels,
				CreationTimestamp: copiedConfig.CreationTimestamp.Time,
			},
			Spec: copiedConfig.Spec,
		}

		out = append(out, outConfig)
	}

	common.RecordIngressNumber(c.options.ClusterId, len(out))
	return out
}

func extractTLSSecretName(host string, tls []ingress.IngressTLS) string {
	if len(tls) == 0 {
		return ""
	}

	for _, t := range tls {
		match := false
		for _, h := range t.Hosts {
			if h == host {
				match = true
			}
		}

		if match {
			return t.SecretName
		}
	}

	return ""
}

func (c *controller) ConvertGateway(convertOptions *common.ConvertOptions, wrapper *common.WrapperConfig, httpsCredentialConfig *cert.Config) error {
	if convertOptions == nil {
		return fmt.Errorf("convertOptions is nil")
	}
	if wrapper == nil {
		return fmt.Errorf("wrapperConfig is nil")
	}

	// Ignore canary config.
	if wrapper.AnnotationsConfig.IsCanary() {
		return nil
	}

	cfg := wrapper.Config
	ingressV1, ok := cfg.Spec.(ingress.IngressSpec)
	if !ok {
		common.IncrementInvalidIngress(c.options.ClusterId, common.Unknown)
		return fmt.Errorf("convert type is invalid in cluster %s", c.options.ClusterId)
	}
	if len(ingressV1.Rules) == 0 && ingressV1.DefaultBackend == nil {
		common.IncrementInvalidIngress(c.options.ClusterId, common.EmptyRule)
		return fmt.Errorf("invalid ingress rule %s:%s in cluster %s, either `defaultBackend` or `rules` must be specified", cfg.Namespace, cfg.Name, c.options.ClusterId)
	}

	for _, rule := range ingressV1.Rules {
		// Need create builder for every rule.
		domainBuilder := &common.IngressDomainBuilder{
			ClusterId: c.options.ClusterId,
			Protocol:  common.HTTP,
			Host:      rule.Host,
			Ingress:   cfg,
			Event:     common.Normal,
		}

		// Extract the previous gateway and builder
		wrapperGateway, exist := convertOptions.Gateways[rule.Host]
		preDomainBuilder, _ := convertOptions.IngressDomainCache.Valid[rule.Host]
		if !exist {
			wrapperGateway = &common.WrapperGateway{
				Gateway:       &networking.Gateway{},
				WrapperConfig: wrapper,
				ClusterId:     c.options.ClusterId,
				Host:          rule.Host,
			}
			if c.options.GatewaySelectorKey != "" {
				wrapperGateway.Gateway.Selector = map[string]string{c.options.GatewaySelectorKey: c.options.GatewaySelectorValue}
			}
			wrapperGateway.Gateway.Servers = append(wrapperGateway.Gateway.Servers, &networking.Server{
				Port: &networking.Port{
					Number:   c.options.GatewayHttpPort,
					Protocol: string(protocol.HTTP),
					Name:     common.CreateConvertedName("http-"+strconv.FormatUint(uint64(c.options.GatewayHttpPort), 10)+"-ingress", string(c.options.ClusterId)),
				},
				Hosts: []string{common.WildcardHost(rule.Host)},
			})

			// Add new gateway, builder
			convertOptions.Gateways[rule.Host] = wrapperGateway
			convertOptions.IngressDomainCache.Valid[rule.Host] = domainBuilder
		} else {
			// Fallback to get downstream tls from current ingress.
			if wrapperGateway.WrapperConfig.AnnotationsConfig.DownstreamTLS == nil {
				wrapperGateway.WrapperConfig.AnnotationsConfig.DownstreamTLS = wrapper.AnnotationsConfig.DownstreamTLS
			}
		}

		passthroughOwner := common.PassthroughTLSHostOwner(convertOptions, rule.Host)
		standaloneSSLPassthrough := convertOptions.PassthroughTLSHostOwners == nil && wrapper.AnnotationsConfig.IsSSLPassthrough()
		if common.SameConfig(passthroughOwner, cfg) || standaloneSSLPassthrough {
			if rule.HTTP == nil || len(rule.HTTP.Paths) == 0 {
				continue
			}
			if _, ok := rootHTTPIngressPath(rule.HTTP.Paths); !ok {
				continue
			}

			domainBuilder.Protocol = common.HTTPS
			if wrapperGateway.IsHTTPS() {
				if common.SameConfig(preDomainBuilder.Ingress, cfg) {
					continue
				}
				domainBuilder.Event = common.DuplicatedTls
				domainBuilder.PreIngress = preDomainBuilder.Ingress
				convertOptions.IngressDomainCache.Invalid = append(convertOptions.IngressDomainCache.Invalid,
					domainBuilder.Build())
				continue
			}
			wrapperGateway.Gateway.Servers = append(wrapperGateway.Gateway.Servers,
				common.CreateSSLPassthroughServer(rule.Host, c.options.GatewayHttpsPort, c.options.ClusterId))
			convertOptions.IngressDomainCache.Valid[rule.Host] = domainBuilder
			continue
		}
		if wrapper.AnnotationsConfig.IsSSLPassthrough() {
			if rule.HTTP != nil {
				if _, ok := rootHTTPIngressPath(rule.HTTP.Paths); ok && passthroughOwner != nil {
					domainBuilder.Protocol = common.HTTPS
					domainBuilder.Event = common.DuplicatedTls
					domainBuilder.PreIngress = passthroughOwner
					convertOptions.IngressDomainCache.Invalid = append(convertOptions.IngressDomainCache.Invalid,
						domainBuilder.Build())
				}
			}
			continue
		}

		// There are no tls settings, so just skip.
		if len(ingressV1.TLS) == 0 {
			continue
		}

		// Get tls secret matching the rule host
		secretName := extractTLSSecretName(rule.Host, ingressV1.TLS)
		secretNamespace := cfg.Namespace
		if secretName != "" {
			if httpsCredentialConfig != nil && httpsCredentialConfig.FallbackForInvalidSecret {
				_, err := c.secretController.Lister().Secrets(secretNamespace).Get(secretName)
				if err != nil {
					if k8serrors.IsNotFound(err) {
						// If there is no matching secret, try to get it from configmap.
						matchSecretName := httpsCredentialConfig.MatchSecretNameByDomain(rule.Host)
						if matchSecretName != "" {
							namespace, secret := cert.ParseTLSSecret(matchSecretName)
							if namespace == "" {
								secretNamespace = c.options.SystemNamespace
							} else {
								secretNamespace = namespace
							}
							secretName = secret
						}
					}
				}
			}
		} else {
			// If there is no matching secret, try to get it from configmap.
			if httpsCredentialConfig != nil {
				secretName = httpsCredentialConfig.MatchSecretNameByDomain(rule.Host)
				secretNamespace = c.options.SystemNamespace
				namespace, secret := cert.ParseTLSSecret(secretName)
				if namespace != "" {
					secretNamespace = namespace
					secretName = secret
				}
			}
		}

		if secretName == "" {
			// There no matching secret, so just skip.
			continue
		}

		domainBuilder.Protocol = common.HTTPS
		domainBuilder.SecretName = path.Join(c.options.ClusterId.String(), cfg.Namespace, secretName)

		if passthroughOwner != nil {
			domainBuilder.Event = common.DuplicatedTls
			domainBuilder.PreIngress = passthroughOwner
			convertOptions.IngressDomainCache.Invalid = append(convertOptions.IngressDomainCache.Invalid,
				domainBuilder.Build())
			continue
		}

		// There is a matching secret and the gateway has already a tls secret.
		// We should report the duplicated tls secret event.
		if wrapperGateway.IsHTTPS() {
			domainBuilder.Event = common.DuplicatedTls
			domainBuilder.PreIngress = preDomainBuilder.Ingress
			convertOptions.IngressDomainCache.Invalid = append(convertOptions.IngressDomainCache.Invalid,
				domainBuilder.Build())
			continue
		}

		// Append https server
		wrapperGateway.Gateway.Servers = append(wrapperGateway.Gateway.Servers, &networking.Server{
			Port: &networking.Port{
				Number:   uint32(c.options.GatewayHttpsPort),
				Protocol: string(protocol.HTTPS),
				Name:     common.CreateConvertedName("https-"+strconv.FormatUint(uint64(c.options.GatewayHttpsPort), 10)+"-ingress", string(c.options.ClusterId)),
			},
			Hosts: []string{common.WildcardHost(rule.Host)},
			Tls: &networking.ServerTLSSettings{
				Mode:           networking.ServerTLSSettings_SIMPLE,
				CredentialName: credentials.ToKubernetesIngressResource(c.options.RawClusterId, secretNamespace, secretName),
			},
		})

		// Update domain builder
		convertOptions.IngressDomainCache.Valid[rule.Host] = domainBuilder
	}

	return nil
}

func (c *controller) ConvertHTTPRoute(convertOptions *common.ConvertOptions, wrapper *common.WrapperConfig) error {
	if convertOptions == nil {
		return fmt.Errorf("convertOptions is nil")
	}
	if wrapper == nil {
		return fmt.Errorf("wrapperConfig is nil")
	}

	// Canary ingress will be processed in the end.
	if wrapper.AnnotationsConfig.IsCanary() {
		convertOptions.CanaryIngresses = append(convertOptions.CanaryIngresses, wrapper)
		return nil
	}

	if convertOptions.Route2Ingress == nil {
		convertOptions.Route2Ingress = map[string]*common.WrapperConfigWithRuleKey{}
	}
	if convertOptions.IngressRouteCache == nil {
		convertOptions.IngressRouteCache = common.NewIngressRouteCache()
	}
	if convertOptions.VirtualServices == nil {
		convertOptions.VirtualServices = map[string]*common.WrapperVirtualService{}
	}
	if convertOptions.HTTPRoutes == nil {
		convertOptions.HTTPRoutes = map[string][]*common.WrapperHTTPRoute{}
	}

	cfg := wrapper.Config
	ingressV1, ok := cfg.Spec.(ingress.IngressSpec)
	if !ok {
		common.IncrementInvalidIngress(c.options.ClusterId, common.Unknown)
		return fmt.Errorf("convert type is invalid in cluster %s", c.options.ClusterId)
	}
	if len(ingressV1.Rules) == 0 && ingressV1.DefaultBackend == nil {
		common.IncrementInvalidIngress(c.options.ClusterId, common.EmptyRule)
		return fmt.Errorf("invalid ingress rule %s:%s in cluster %s, either `defaultBackend` or `rules` must be specified", cfg.Namespace, cfg.Name, c.options.ClusterId)
	}

	if ingressV1.DefaultBackend != nil &&
		((ingressV1.DefaultBackend.Service != nil &&
			ingressV1.DefaultBackend.Service.Name != "") ||
			ingressV1.DefaultBackend.Resource != nil) {
		convertOptions.HasDefaultBackend = true
	}

	// In one ingress, we will limit the rule conflict.
	// When the host, pathType, path of two rule are same, we think there is a conflict event.
	definedRules := sets.New[string]()

	// But in across ingresses case, we will restrict this limit.
	// When the {host, path, headers, method, params} of two rule in different ingress are same, we think there is a conflict event.
	var tempRuleKey []string

	for _, rule := range ingressV1.Rules {
		if rule.HTTP == nil || len(rule.HTTP.Paths) == 0 {
			IngressLog.Warnf("invalid ingress rule %s:%s for host %q in cluster %s, no paths defined", cfg.Namespace, cfg.Name, rule.Host, c.options.ClusterId)
			continue
		}

		wrapperVS, exist := convertOptions.VirtualServices[rule.Host]
		if !exist {
			wrapperVS = common.NewWrapperVirtualService(rule.Host, wrapper)
			convertOptions.VirtualServices[rule.Host] = wrapperVS
		}

		// Record the latest app root for per host.
		redirect := wrapper.AnnotationsConfig.Redirect
		if redirect != nil && redirect.AppRoot != "" {
			wrapperVS.AppRoot = redirect.AppRoot
		}

		wrapperHttpRoutes := make([]*common.WrapperHTTPRoute, 0, len(rule.HTTP.Paths))

		for _, httpPath := range rule.HTTP.Paths {
			wrapperHttpRoute := &common.WrapperHTTPRoute{
				HTTPRoute:     &networking.HTTPRoute{},
				WrapperConfig: wrapper,
				Host:          rule.Host,
				ClusterId:     c.options.ClusterId,
			}

			var pathType common.PathType
			originPath := httpPath.Path
			if annotationsConfig := wrapper.AnnotationsConfig; annotationsConfig.NeedRegexMatch(originPath) {
				if annotationsConfig.IsFullPathRegexMatch() {
					pathType = common.FullPathRegex
				} else {
					pathType = common.PrefixRegex
				}
			} else {
				ingressPathType := defaultPathType
				if httpPath.PathType != nil {
					ingressPathType = *httpPath.PathType
				}
				switch ingressPathType {
				case ingress.PathTypeExact:
					pathType = common.Exact
				case ingress.PathTypePrefix:
					pathType = common.Prefix
					if httpPath.Path != "/" {
						originPath = strings.TrimSuffix(httpPath.Path, "/")
					}
				}
			}
			wrapperHttpRoute.OriginPath = originPath
			wrapperHttpRoute.OriginPathType = pathType
			wrapperHttpRoute.HTTPRoute.Match = c.generateHttpMatches(pathType, httpPath.Path, wrapperVS)
			wrapperHttpRoute.HTTPRoute.Name = common.GenerateUniqueRouteName(c.options.SystemNamespace, wrapperHttpRoute)

			ingressRouteBuilder := convertOptions.IngressRouteCache.New(wrapperHttpRoute)

			hostAndPath := wrapperHttpRoute.PathFormat()
			key := createRuleKey(cfg.Annotations, hostAndPath)
			wrapperHttpRoute.RuleKey = key
			if WrapPreIngress, exist := convertOptions.Route2Ingress[key]; exist {
				ingressRouteBuilder.PreIngress = WrapPreIngress.Config
				ingressRouteBuilder.Event = common.DuplicatedRoute
			}
			tempRuleKey = append(tempRuleKey, key)

			// Two duplicated rules in the same ingress.
			if ingressRouteBuilder.Event == common.Normal {
				pathFormat := wrapperHttpRoute.PathFormat()
				if definedRules.Contains(pathFormat) {
					ingressRouteBuilder.PreIngress = cfg
					ingressRouteBuilder.Event = common.DuplicatedRoute
				}
				definedRules.Insert(pathFormat)
			}

			// backend service check
			var event common.Event
			destinationConfig := wrapper.AnnotationsConfig.Destination
			wrapperHttpRoute.HTTPRoute.Route, event = c.backendToRouteDestination(&httpPath.Backend, cfg.Namespace, ingressRouteBuilder, destinationConfig)

			if destinationConfig != nil {
				wrapperHttpRoute.WeightTotal = int32(destinationConfig.WeightSum)
			}

			if ingressRouteBuilder.Event != common.Normal {
				event = ingressRouteBuilder.Event
			}

			if event != common.Normal {
				common.IncrementInvalidIngress(c.options.ClusterId, event)
				ingressRouteBuilder.Event = event
			} else {
				wrapperHttpRoutes = append(wrapperHttpRoutes, wrapperHttpRoute)
			}

			convertOptions.IngressRouteCache.Add(ingressRouteBuilder)
		}

		for idx, item := range tempRuleKey {
			if val, exist := convertOptions.Route2Ingress[item]; !exist || strings.Compare(val.RuleKey, tempRuleKey[idx]) != 0 {
				convertOptions.Route2Ingress[item] = &common.WrapperConfigWithRuleKey{
					Config:  cfg,
					RuleKey: tempRuleKey[idx],
				}
			}
		}

		old, f := convertOptions.HTTPRoutes[rule.Host]
		if f {
			old = append(old, wrapperHttpRoutes...)
			convertOptions.HTTPRoutes[rule.Host] = old
		} else {
			convertOptions.HTTPRoutes[rule.Host] = wrapperHttpRoutes
		}
	}

	if common.HasPassthroughTLSHostOwner(convertOptions, cfg) ||
		(convertOptions.PassthroughTLSHostOwners == nil && wrapper.AnnotationsConfig.IsSSLPassthrough()) {
		return c.ConvertTLSRoute(convertOptions, wrapper)
	}

	return nil
}

func (c *controller) ConvertTLSRoute(convertOptions *common.ConvertOptions, wrapper *common.WrapperConfig) error {
	if convertOptions == nil {
		return fmt.Errorf("convertOptions is nil")
	}
	if wrapper == nil {
		return fmt.Errorf("wrapperConfig is nil")
	}

	if convertOptions.VirtualServices == nil {
		convertOptions.VirtualServices = map[string]*common.WrapperVirtualService{}
	}

	cfg := wrapper.Config
	ingressV1, ok := cfg.Spec.(ingress.IngressSpec)
	if !ok {
		common.IncrementInvalidIngress(c.options.ClusterId, common.Unknown)
		return fmt.Errorf("convert type is invalid in cluster %s", c.options.ClusterId)
	}
	if len(ingressV1.Rules) == 0 {
		common.IncrementInvalidIngress(c.options.ClusterId, common.EmptyRule)
		return fmt.Errorf("invalid ingress rule %s:%s in cluster %s, `rules` must be specified", cfg.Namespace, cfg.Name, c.options.ClusterId)
	}

	for _, rule := range ingressV1.Rules {
		if !common.IsPassthroughTLSHostOwner(convertOptions, cfg, rule.Host) {
			IngressLog.Warnf("ignore duplicated ssl passthrough ingress rule %s:%s for host %q in cluster %s", cfg.Namespace, cfg.Name, rule.Host, c.options.ClusterId)
			continue
		}

		if rule.HTTP == nil || len(rule.HTTP.Paths) == 0 {
			IngressLog.Warnf("invalid ssl passthrough ingress rule %s:%s for host %q in cluster %s, no paths defined", cfg.Namespace, cfg.Name, rule.Host, c.options.ClusterId)
			continue
		}

		httpPath, ok := rootHTTPIngressPath(rule.HTTP.Paths)
		if !ok {
			IngressLog.Warnf("ignore ssl passthrough ingress rule %s:%s for host %q in cluster %s, root path is not defined", cfg.Namespace, cfg.Name, rule.Host, c.options.ClusterId)
			continue
		}

		wrapperVS, exist := convertOptions.VirtualServices[rule.Host]
		if !exist {
			wrapperVS = common.NewWrapperVirtualService(rule.Host, wrapper)
			convertOptions.VirtualServices[rule.Host] = wrapperVS
		} else if wrapperVS.HasTLSRouteForHost(rule.Host) {
			continue
		}

		routeDestination, event := c.backendToTLSRouteDestination(&httpPath.Backend, cfg.Namespace, wrapper.AnnotationsConfig.Destination)
		if event != common.Normal {
			common.IncrementInvalidIngress(c.options.ClusterId, event)
			continue
		}

		wrapperVS.VirtualService.Tls = append(wrapperVS.VirtualService.Tls,
			common.CreateTLSRoute(rule.Host, routeDestination))
	}

	return nil
}

func rootHTTPIngressPath(paths []ingress.HTTPIngressPath) (*ingress.HTTPIngressPath, bool) {
	for idx := range paths {
		if paths[idx].Path == "" || paths[idx].Path == "/" {
			return &paths[idx], true
		}
	}
	return nil, false
}

func (c *controller) generateHttpMatches(pathType common.PathType, path string, wrapperVS *common.WrapperVirtualService) []*networking.HTTPMatchRequest {
	var httpMatches []*networking.HTTPMatchRequest

	httpMatch := &networking.HTTPMatchRequest{}
	switch pathType {
	case common.PrefixRegex:
		httpMatch.Uri = &networking.StringMatch{
			MatchType: &networking.StringMatch_Regex{Regex: path + ".*"},
		}
	case common.FullPathRegex:
		httpMatch.Uri = &networking.StringMatch{
			MatchType: &networking.StringMatch_Regex{Regex: path + "$"},
		}
	case common.Exact:
		httpMatch.Uri = &networking.StringMatch{
			MatchType: &networking.StringMatch_Exact{Exact: path},
		}
	case common.Prefix:
		if path == "/" {
			if wrapperVS != nil {
				wrapperVS.ConfiguredDefaultBackend = true
			}
			// Optimize common case of / to not needed regex
			httpMatch.Uri = &networking.StringMatch{
				MatchType: &networking.StringMatch_Prefix{Prefix: path},
			}
		} else {
			newPath := strings.TrimSuffix(path, "/")
			httpMatches = append(httpMatches, c.generateHttpMatches(common.Exact, newPath, wrapperVS)...)
			httpMatch.Uri = &networking.StringMatch{
				MatchType: &networking.StringMatch_Prefix{Prefix: newPath + "/"},
			}
		}
	}

	httpMatches = append(httpMatches, httpMatch)

	return httpMatches
}

func (c *controller) ApplyDefaultBackend(convertOptions *common.ConvertOptions, wrapper *common.WrapperConfig) error {
	if wrapper.AnnotationsConfig.IsCanary() {
		return nil
	}

	cfg := wrapper.Config
	ingressV1, ok := cfg.Spec.(ingress.IngressSpec)
	if !ok {
		common.IncrementInvalidIngress(c.options.ClusterId, common.Unknown)
		return fmt.Errorf("convert type is invalid in cluster %s", c.options.ClusterId)
	}

	if ingressV1.DefaultBackend == nil {
		return nil
	}

	apply := func(host string, op func(vs *common.WrapperVirtualService, defaultRoute *common.WrapperHTTPRoute)) {
		wirecardVS, exist := convertOptions.VirtualServices[host]
		if !exist || !wirecardVS.ConfiguredDefaultBackend {
			if !exist {
				wirecardVS = common.NewWrapperVirtualService(host, wrapper)
				convertOptions.VirtualServices[host] = wirecardVS
			}

			specDefaultBackend := c.createDefaultRoute(wrapper, ingressV1.DefaultBackend, host)
			if specDefaultBackend != nil {
				convertOptions.VirtualServices[host] = wirecardVS
				op(wirecardVS, specDefaultBackend)
			}
		}
	}

	// First process *
	apply("*", func(_ *common.WrapperVirtualService, defaultRoute *common.WrapperHTTPRoute) {
		var hasFound bool
		for _, httpRoute := range convertOptions.HTTPRoutes["*"] {
			if httpRoute.OriginPathType == common.Prefix && httpRoute.OriginPath == "/" {
				hasFound = true
				convertOptions.IngressRouteCache.Delete(httpRoute)

				httpRoute.HTTPRoute = defaultRoute.HTTPRoute
				httpRoute.WrapperConfig = defaultRoute.WrapperConfig
				convertOptions.IngressRouteCache.NewAndAdd(httpRoute)
			}
		}
		if !hasFound {
			convertOptions.HTTPRoutes["*"] = append(convertOptions.HTTPRoutes["*"], defaultRoute)
		}
	})

	for _, rule := range ingressV1.Rules {
		if rule.Host == "*" {
			continue
		}

		apply(rule.Host, func(vs *common.WrapperVirtualService, defaultRoute *common.WrapperHTTPRoute) {
			convertOptions.HTTPRoutes[rule.Host] = append(convertOptions.HTTPRoutes[rule.Host], defaultRoute)
			vs.ConfiguredDefaultBackend = true

			convertOptions.IngressRouteCache.NewAndAdd(defaultRoute)
		})
	}

	return nil
}

func (c *controller) ApplyCanaryIngress(convertOptions *common.ConvertOptions, wrapper *common.WrapperConfig) error {
	byHeader, _ := wrapper.AnnotationsConfig.CanaryKind()

	cfg := wrapper.Config
	ingressV1, ok := cfg.Spec.(ingress.IngressSpec)
	if !ok {
		common.IncrementInvalidIngress(c.options.ClusterId, common.Unknown)
		return fmt.Errorf("convert type is invalid in cluster %s", c.options.ClusterId)
	}
	if len(ingressV1.Rules) == 0 && ingressV1.DefaultBackend == nil {
		common.IncrementInvalidIngress(c.options.ClusterId, common.EmptyRule)
		return fmt.Errorf("invalid ingress rule %s:%s in cluster %s, either `defaultBackend` or `rules` must be specified", cfg.Namespace, cfg.Name, c.options.ClusterId)
	}

	for _, rule := range ingressV1.Rules {
		if rule.HTTP == nil || len(rule.HTTP.Paths) == 0 {
			IngressLog.Warnf("invalid ingress rule %s:%s for host %q in cluster %s, no paths defined", cfg.Namespace, cfg.Name, rule.Host, c.options.ClusterId)
			continue
		}

		routes, exist := convertOptions.HTTPRoutes[rule.Host]
		if !exist {
			continue
		}

		for _, httpPath := range rule.HTTP.Paths {
			canary := &common.WrapperHTTPRoute{
				HTTPRoute:     &networking.HTTPRoute{},
				WrapperConfig: wrapper,
				Host:          rule.Host,
				ClusterId:     c.options.ClusterId,
			}

			var pathType common.PathType
			originPath := httpPath.Path
			if annotationsConfig := wrapper.AnnotationsConfig; annotationsConfig.NeedRegexMatch(originPath) {
				if annotationsConfig.IsFullPathRegexMatch() {
					pathType = common.FullPathRegex
				} else {
					pathType = common.PrefixRegex
				}
			} else {
				ingressPathType := defaultPathType
				if httpPath.PathType != nil {
					ingressPathType = *httpPath.PathType
				}
				switch ingressPathType {
				case ingress.PathTypeExact:
					pathType = common.Exact
				case ingress.PathTypePrefix:
					pathType = common.Prefix
					if httpPath.Path != "/" {
						originPath = strings.TrimSuffix(httpPath.Path, "/")
					}
				}
			}
			canary.OriginPath = originPath
			canary.OriginPathType = pathType

			ingressRouteBuilder := convertOptions.IngressRouteCache.New(canary)
			// backend service check
			var event common.Event
			destinationConfig := wrapper.AnnotationsConfig.Destination
			canary.HTTPRoute.Route, event = c.backendToRouteDestination(&httpPath.Backend, cfg.Namespace, ingressRouteBuilder, destinationConfig)
			if event != common.Normal {
				common.IncrementInvalidIngress(c.options.ClusterId, event)
				ingressRouteBuilder.Event = event
				convertOptions.IngressRouteCache.Add(ingressRouteBuilder)
				continue
			}
			canary.RuleKey = createRuleKey(canary.WrapperConfig.Config.Annotations, canary.PathFormat())

			// find the base ingress
			pos := 0
			var targetRoute *common.WrapperHTTPRoute
			for _, route := range routes {
				if isCanaryRoute(canary, route) {
					targetRoute = route
					break
				}
				pos += 1
			}

			if targetRoute == nil {
				continue
			}

			canaryConfig := wrapper.AnnotationsConfig.Canary

			// Header, Cookie
			if byHeader {
				IngressLog.Debug("Insert canary route by header")
				annotations.ApplyByHeader(canary.HTTPRoute, targetRoute.HTTPRoute, canary.WrapperConfig.AnnotationsConfig)
				canary.HTTPRoute.Name = common.GenerateUniqueRouteName(c.options.SystemNamespace, canary)
			} else {
				IngressLog.Debug("Merge canary route by weight")
				if targetRoute.WeightTotal == 0 {
					targetRoute.WeightTotal = int32(canaryConfig.WeightTotal)
				}
				annotations.ApplyByWeight(canary.HTTPRoute, targetRoute.HTTPRoute, canary.WrapperConfig.AnnotationsConfig)
			}
			IngressLog.Debugf("Canary route is %v", canary)

			if byHeader {
				// Inherit policy from normal route
				canary.WrapperConfig.AnnotationsConfig.Auth = targetRoute.WrapperConfig.AnnotationsConfig.Auth

				routes = append(routes[:pos+1], routes[pos:]...)
				routes[pos] = canary
				convertOptions.HTTPRoutes[rule.Host] = routes

				// Recreate route name.
				ingressRouteBuilder.RouteName = common.GenerateUniqueRouteName(c.options.SystemNamespace, canary)
				convertOptions.IngressRouteCache.Add(ingressRouteBuilder)
			} else {
				convertOptions.IngressRouteCache.Update(targetRoute)
			}

		}
	}
	return nil
}

func (c *controller) ConvertTrafficPolicy(convertOptions *common.ConvertOptions, wrapper *common.WrapperConfig) error {
	if !wrapper.AnnotationsConfig.NeedTrafficPolicy() {
		return nil
	}

	cfg := wrapper.Config
	ingressV1, ok := cfg.Spec.(ingress.IngressSpec)
	if !ok {
		common.IncrementInvalidIngress(c.options.ClusterId, common.Unknown)
		return fmt.Errorf("convert type is invalid in cluster %s", c.options.ClusterId)
	}
	if len(ingressV1.Rules) == 0 && ingressV1.DefaultBackend == nil {
		common.IncrementInvalidIngress(c.options.ClusterId, common.EmptyRule)
		return fmt.Errorf("invalid ingress rule %s:%s in cluster %s, either `defaultBackend` or `rules` must be specified", cfg.Namespace, cfg.Name, c.options.ClusterId)
	}

	if ingressV1.DefaultBackend != nil {
		err := c.storeBackendTrafficPolicy(wrapper, ingressV1.DefaultBackend, convertOptions.Service2TrafficPolicy)
		if err != nil {
			IngressLog.Errorf("ignore default service within ingress %s/%s, since error:%v", cfg.Namespace, cfg.Name, err)
		}
	}

	for _, rule := range ingressV1.Rules {
		if rule.HTTP == nil || len(rule.HTTP.Paths) == 0 {
			continue
		}

		for _, httpPath := range rule.HTTP.Paths {
			err := c.storeBackendTrafficPolicy(wrapper, &httpPath.Backend, convertOptions.Service2TrafficPolicy)
			if err != nil {
				IngressLog.Errorf("ignore service within ingress %s/%s, since error:%v", cfg.Namespace, cfg.Name, err)
			}
		}
	}

	return nil
}

func (c *controller) storeBackendTrafficPolicy(wrapper *common.WrapperConfig, backend *ingress.IngressBackend, store map[common.ServiceKey]*common.WrapperTrafficPolicy) error {
	if backend == nil {
		return errors.New("invalid empty backend")
	}
	if common.ValidateBackendResource(backend.Resource) && wrapper.AnnotationsConfig.Destination != nil {
		for _, dest := range wrapper.AnnotationsConfig.Destination.McpDestination {
			portNumber := dest.Destination.GetPort().GetNumber()
			serviceKey := common.CreateMcpServiceKey(dest.Destination.Host, int32(portNumber))
			if _, exist := store[serviceKey]; !exist {
				if serviceKey.Port != 0 {
					store[serviceKey] = &common.WrapperTrafficPolicy{
						PortTrafficPolicy: &networking.TrafficPolicy_PortTrafficPolicy{
							Port: &networking.PortSelector{
								Number: uint32(serviceKey.Port),
							},
						},
						WrapperConfig: wrapper,
					}
				} else {
					store[serviceKey] = &common.WrapperTrafficPolicy{
						TrafficPolicy: &networking.TrafficPolicy{},
						WrapperConfig: wrapper,
					}
				}
			}
		}
	} else {
		if backend.Service == nil {
			return nil
		}
		serviceKey, err := c.createServiceKey(backend.Service, wrapper.Config.Namespace)
		if err != nil {
			return fmt.Errorf("ignore service %s within ingress %s/%s", serviceKey.Name, wrapper.Config.Namespace, wrapper.Config.Name)
		}

		if _, exist := store[serviceKey]; !exist {
			store[serviceKey] = &common.WrapperTrafficPolicy{
				PortTrafficPolicy: &networking.TrafficPolicy_PortTrafficPolicy{
					Port: &networking.PortSelector{
						Number: uint32(serviceKey.Port),
					},
				},
				WrapperConfig: wrapper,
			}
		}
	}
	return nil
}

func (c *controller) createDefaultRoute(wrapper *common.WrapperConfig, backend *ingress.IngressBackend, host string) *common.WrapperHTTPRoute {
	if backend == nil {
		return nil
	}

	var routeDestination []*networking.HTTPRouteDestination

	if common.ValidateBackendResource(backend.Resource) {
		routeDestination = wrapper.AnnotationsConfig.Destination.McpDestination
	} else {
		service := backend.Service
		namespace := wrapper.Config.Namespace

		port := &networking.PortSelector{}
		if service.Port.Number > 0 {
			port.Number = uint32(service.Port.Number)
		} else {
			resolvedPort, err := resolveNamedPort(service, namespace, c.serviceLister)
			if err != nil {
				return nil
			}
			port.Number = uint32(resolvedPort)
		}

		routeDestination = []*networking.HTTPRouteDestination{
			{
				Destination: &networking.Destination{
					Host: util.CreateServiceFQDN(namespace, service.Name),
					Port: port,
				},
				Weight: 100,
			},
		}
	}

	route := &common.WrapperHTTPRoute{
		HTTPRoute: &networking.HTTPRoute{
			Route: routeDestination,
		},
		WrapperConfig:    wrapper,
		ClusterId:        c.options.ClusterId,
		Host:             host,
		IsDefaultBackend: true,
		OriginPathType:   common.Prefix,
		OriginPath:       "/",
	}
	route.HTTPRoute.Name = common.GenerateUniqueRouteNameWithSuffix(c.options.SystemNamespace, route, "default")

	return route
}

func (c *controller) createServiceKey(service *ingress.IngressServiceBackend, namespace string) (common.ServiceKey, error) {
	serviceKey := common.ServiceKey{}
	if service == nil || service.Name == "" {
		return serviceKey, errors.New("service name is empty")
	}

	var port int32
	var err error
	if service.Port.Number > 0 {
		port = service.Port.Number
	} else {
		port, err = resolveNamedPort(service, namespace, c.serviceLister)
		if err != nil {
			return serviceKey, err
		}
	}

	return common.ServiceKey{
		Namespace: namespace,
		Name:      service.Name,
		Port:      port,
	}, nil
}

func isCanaryRoute(canary, route *common.WrapperHTTPRoute) bool {
	return !route.WrapperConfig.AnnotationsConfig.IsCanary() && canary.RuleKey == route.RuleKey
}

func (c *controller) backendToRouteDestination(backend *ingress.IngressBackend, namespace string,
	builder *common.IngressRouteBuilder, config *annotations.DestinationConfig,
) ([]*networking.HTTPRouteDestination, common.Event) {
	if backend == nil || (backend.Service == nil && backend.Resource == nil) {
		return nil, common.InvalidBackendService
	}

	if backend.Service == nil {
		if config != nil {
			return config.McpDestination, common.Normal
		}
		return nil, common.InvalidBackendService
	}

	service := backend.Service
	builder.PortName = service.Port.Name

	port := &networking.PortSelector{}
	if service.Port.Number > 0 {
		port.Number = uint32(service.Port.Number)
	} else {
		resolvedPort, err := resolveNamedPort(service, namespace, c.serviceLister)
		if err != nil {
			return nil, common.PortNameResolveError
		}
		port.Number = uint32(resolvedPort)
	}

	builder.ServiceList = []model.BackendService{
		{
			Namespace: namespace,
			Name:      service.Name,
			Port:      port.Number,
			Weight:    100,
		},
	}

	return []*networking.HTTPRouteDestination{
		{
			Destination: &networking.Destination{
				Host: util.CreateServiceFQDN(namespace, service.Name),
				Port: port,
			},
			Weight: 100,
		},
	}, common.Normal
}

func (c *controller) backendToTLSRouteDestination(backend *ingress.IngressBackend, namespace string,
	config *annotations.DestinationConfig,
) ([]*networking.RouteDestination, common.Event) {
	if backend == nil {
		return nil, common.InvalidBackendService
	}

	if backend.Service == nil {
		if config != nil && len(config.McpDestination) > 0 {
			return httpRouteDestinationToRouteDestination(config.McpDestination), common.Normal
		}
		return nil, common.InvalidBackendService
	}

	service := backend.Service
	port := &networking.PortSelector{}
	if service.Port.Number > 0 {
		port.Number = uint32(service.Port.Number)
	} else {
		resolvedPort, err := resolveNamedPort(service, namespace, c.serviceLister)
		if err != nil {
			return nil, common.PortNameResolveError
		}
		port.Number = uint32(resolvedPort)
	}

	return []*networking.RouteDestination{
		{
			Destination: &networking.Destination{
				Host: util.CreateServiceFQDN(namespace, service.Name),
				Port: port,
			},
			Weight: 100,
		},
	}, common.Normal
}

func httpRouteDestinationToRouteDestination(destinations []*networking.HTTPRouteDestination) []*networking.RouteDestination {
	out := make([]*networking.RouteDestination, 0, len(destinations))
	for _, destination := range destinations {
		out = append(out, &networking.RouteDestination{
			Destination: destination.Destination,
			Weight:      destination.Weight,
		})
	}
	return out
}

func resolveNamedPort(service *ingress.IngressServiceBackend, namespace string, serviceLister listerv1.ServiceLister) (int32, error) {
	svc, err := serviceLister.Services(namespace).Get(service.Name)
	if err != nil {
		return 0, err
	}
	for _, port := range svc.Spec.Ports {
		if port.Name == service.Port.Name {
			return port.Port, nil
		}
	}
	return 0, common.ErrNotFound
}

func (c *controller) shouldProcessIngressWithClass(ingress *ingress.Ingress, ingressClass *ingress.IngressClass) bool {
	if class, exists := ingress.Annotations[util.IngressClassAnnotation]; exists {
		switch c.options.IngressClass {
		case "":
			return true
		case common.DefaultIngressClass:
			return class == "" || class == common.DefaultIngressClass
		default:
			return c.options.IngressClass == class
		}
	} else if ingressClass != nil {
		switch c.options.IngressClass {
		case "":
			return true
		default:
			return c.options.IngressClass == ingressClass.Name
		}
	} else {
		ingressClassName := ingress.Spec.IngressClassName
		switch c.options.IngressClass {
		case "":
			return true
		case common.DefaultIngressClass:
			return ingressClassName == nil || *ingressClassName == "" ||
				*ingressClassName == common.DefaultIngressClass
		default:
			return ingressClassName != nil && *ingressClassName == c.options.IngressClass
		}
	}
}

func (c *controller) shouldProcessIngress(i *ingress.Ingress) (bool, error) {
	var class *ingress.IngressClass
	if c.classLister != nil && i.Spec.IngressClassName != nil {
		classCache, err := c.classLister.Get(*i.Spec.IngressClassName)
		if err != nil && !kerrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to get ingress class %v from cluster %s: %v", i.Spec.IngressClassName, c.options.ClusterId, err)
		}
		class = classCache
	}

	// first check ingress class
	if c.shouldProcessIngressWithClass(i, class) {
		// then check namespace
		switch c.options.WatchNamespace {
		case "":
			return true, nil
		default:
			return c.options.WatchNamespace == i.Namespace, nil
		}
	}

	return false, nil
}

// shouldProcessIngressUpdate checks whether we should renotify registered handlers about an update event
func (c *controller) shouldProcessIngressUpdate(ing *ingress.Ingress) (bool, error) {
	shouldProcess, err := c.shouldProcessIngress(ing)
	if err != nil {
		return false, err
	}

	namespacedName := ing.Namespace + "/" + ing.Name
	if shouldProcess {
		// record processed ingress
		c.mutex.Lock()
		preConfig, exist := c.ingresses[namespacedName]
		c.ingresses[namespacedName] = ing
		c.mutex.Unlock()

		// We only care about annotations, labels and spec.
		if exist {
			if !reflect.DeepEqual(preConfig.Annotations, ing.Annotations) {
				IngressLog.Debugf("Annotations of ingress %s changed, should process.", namespacedName)
				return true, nil
			}
			if !reflect.DeepEqual(preConfig.Labels, ing.Labels) {
				IngressLog.Debugf("Labels of ingress %s changed, should process.", namespacedName)
				return true, nil
			}
			if !reflect.DeepEqual(preConfig.Spec, ing.Spec) {
				IngressLog.Debugf("Spec of ingress %s changed, should process.", namespacedName)
				return true, nil
			}

			return false, nil
		}

		IngressLog.Debugf("First receive relative ingress %s, should process.", namespacedName)
		return true, nil
	}

	c.mutex.Lock()
	_, preProcessed := c.ingresses[namespacedName]
	// previous processed but should not currently, delete it
	if preProcessed && !shouldProcess {
		delete(c.ingresses, namespacedName)
	}
	c.mutex.Unlock()

	return preProcessed, nil
}

// setDefaultMSEIngressOptionalField sets a default value for optional fields when is not defined.
func setDefaultMSEIngressOptionalField(ing *ingress.Ingress) {
	for idx, tls := range ing.Spec.TLS {
		if len(tls.Hosts) == 0 {
			ing.Spec.TLS[idx].Hosts = []string{common.DefaultHost}
		}
	}

	for idx, rule := range ing.Spec.Rules {
		if rule.IngressRuleValue.HTTP == nil {
			continue
		}

		if rule.Host == "" {
			ing.Spec.Rules[idx].Host = common.DefaultHost
		}

		for innerIdx := range rule.IngressRuleValue.HTTP.Paths {
			p := &rule.IngressRuleValue.HTTP.Paths[innerIdx]

			if p.Path == "" {
				p.Path = common.DefaultPath
			}

			if p.PathType == nil {
				p.PathType = &defaultPathType
				// for old k8s version
				if !annotations.NeedRegexMatch(ing.Annotations) {
					if strings.HasSuffix(p.Path, ".*") {
						p.Path = strings.TrimSuffix(p.Path, ".*")
					}

					if strings.HasSuffix(p.Path, "/*") {
						p.Path = strings.TrimSuffix(p.Path, "/*")
					}
				}
			}

			if *p.PathType == ingress.PathTypeImplementationSpecific {
				p.PathType = &defaultPathType
			}
		}
	}
}

// createRuleKey according to the pathType, path, methods, headers, params of rules
func createRuleKey(annots map[string]string, hostAndPath string) string {
	var (
		headers [][2]string
		params  [][2]string
		sb      strings.Builder
	)

	sep := "\n\n"

	// path
	sb.WriteString(hostAndPath)
	sb.WriteString(sep)

	// methods
	if str, ok := annots[annotations.HigressAnnotationsPrefix+"/"+annotations.MatchMethod]; ok {
		sb.WriteString(str)
	}
	sb.WriteString(sep)

	start := len(annotations.HigressAnnotationsPrefix) + 1 // example: higress.io/exact-match-header-key: value
	// headers && params
	for k, val := range annots {
		if idx := strings.Index(k, annotations.MatchHeader); idx != -1 {
			key := k[start:idx] + k[idx+len(annotations.MatchHeader)+1:]
			headers = append(headers, [2]string{key, val})
		} else if idx := strings.Index(k, annotations.MatchPseudoHeader); idx != -1 {
			key := k[start:idx] + ":" + k[idx+len(annotations.MatchPseudoHeader)+1:]
			headers = append(headers, [2]string{key, val})
		} else if idx := strings.Index(k, annotations.MatchQuery); idx != -1 {
			key := k[start:idx] + k[idx+len(annotations.MatchQuery)+1:]
			params = append(params, [2]string{key, val})
		}
	}
	sort.SliceStable(headers, func(i, j int) bool {
		return headers[i][0] < headers[j][0]
	})
	sort.SliceStable(params, func(i, j int) bool {
		return params[i][0] < params[j][0]
	})
	for idx := range headers {
		if idx != 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(headers[idx][0])
		sb.WriteByte('\t')
		sb.WriteString(headers[idx][1])
	}
	sb.WriteString(sep)
	for idx := range params {
		if idx != 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(params[idx][0])
		sb.WriteByte('\t')
		sb.WriteString(params[idx][1])
	}
	sb.WriteString(sep)

	return sb.String()
}
