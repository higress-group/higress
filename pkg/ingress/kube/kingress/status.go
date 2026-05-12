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

package kingress

import (
	"context"
	"reflect"
	"time"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	kingressclient "knative.dev/networking/pkg/client/clientset/versioned"
	kingresslister "knative.dev/networking/pkg/client/listers/networking/v1alpha1"

	common2 "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
	"github.com/alibaba/higress/v2/pkg/kube"
)

// statusSyncer keeps the status IP in each Ingress resource updated
type statusSyncer struct {
	client           kingressclient.Interface
	controller       *controller
	watchedNamespace string
	ingressLister    kingresslister.IngressLister
	serviceLister    listerv1.ServiceLister
}

// newStatusSyncer creates a new instance
func newStatusSyncer(localKubeClient, client kube.Client, controller *controller, namespace string,
	serviceLister listerv1.ServiceLister,
) *statusSyncer {
	return &statusSyncer{
		client:           client.KIngress(),
		controller:       controller,
		watchedNamespace: namespace,
		ingressLister:    client.KIngressInformer().Networking().V1alpha1().Ingresses().Lister(),
		serviceLister:    serviceLister,
	}
}

func (s *statusSyncer) run(stopCh <-chan struct{}) {
	cache.WaitForCacheSync(stopCh, s.controller.HasSynced)

	ticker := time.NewTicker(common2.DefaultStatusUpdateInterval)
	for {
		select {
		case <-stopCh:
			ticker.Stop()
			return
		case <-ticker.C:
			if err := s.runUpdateStatus(); err != nil {
				IngressLog.Errorf("update status task fail, err %v", err)
			}
		}
	}
}

func (s *statusSyncer) runUpdateStatus() error {
	svcList, err := s.serviceLister.Services(s.watchedNamespace).List(common2.SvcLabelSelector)
	if err != nil {
		return err
	}

	lbStatusList := common2.GetLbStatusList(svcList)
	return s.updateStatus(lbStatusList)
}

func transportLoadBalancerIngress(status []coreV1.LoadBalancerIngress) []v1alpha1.LoadBalancerIngressStatus {
	var KnativeLBIngress []v1alpha1.LoadBalancerIngressStatus
	for _, addr := range status {
		KnativeIng := v1alpha1.LoadBalancerIngressStatus{
			IP:     addr.IP,
			Domain: addr.Hostname,
		}
		KnativeLBIngress = append(KnativeLBIngress, KnativeIng)
	}
	return KnativeLBIngress
}

// updateStatus updates ingress status with the list of IP
func (s *statusSyncer) updateStatus(status []coreV1.LoadBalancerIngress) error {
	ingressList, err := s.ingressLister.List(labels.Everything())
	if err != nil {
		return err
	}
	for _, ingress := range ingressList {
		shouldTarget, err := s.controller.shouldProcessIngress(ingress)
		if err != nil {
			IngressLog.Warnf("error determining whether should target ingress %s/%s within cluster %s for status update: %v",
				ingress.Namespace, ingress.Name, s.controller.options.ClusterId, err)
			return err
		}
		if !shouldTarget {
			continue
		}
		ingress.Status.MarkNetworkConfigured()
		KIngressStatus := transportLoadBalancerIngress(status)
		if ingress.Status.PublicLoadBalancer == nil || len(ingress.Status.PublicLoadBalancer.Ingress) != len(KIngressStatus) || !reflect.DeepEqual(ingress.Status.PublicLoadBalancer.Ingress, KIngressStatus) {
			ingress.Status.ObservedGeneration = ingress.Generation
			ingress.Status.MarkLoadBalancerReady(KIngressStatus, KIngressStatus)
			IngressLog.Infof("Update Ingress %v/%v within cluster %s status", ingress.Namespace, ingress.Name, s.controller.options.ClusterId)
		}
		_, err = s.client.NetworkingV1alpha1().Ingresses(ingress.Namespace).UpdateStatus(context.TODO(), ingress, metaV1.UpdateOptions{})
		if err != nil {
			IngressLog.Warnf("error updating ingress %s/%s within cluster %s status: %v",
				ingress.Namespace, ingress.Name, s.controller.options.ClusterId, err)
		}
	}

	return nil
}
