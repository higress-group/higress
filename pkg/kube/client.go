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

package kube

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.uber.org/atomic"
	"istio.io/istio/pkg/cluster"
	istiokube "istio.io/istio/pkg/kube"
	apiExtensionsV1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	kingressclient "knative.dev/networking/pkg/client/clientset/versioned"
	kingressfake "knative.dev/networking/pkg/client/clientset/versioned/fake"
	kingressinformer "knative.dev/networking/pkg/client/informers/externalversions"

	higressclient "github.com/alibaba/higress/v2/client/pkg/clientset/versioned"
	higressfake "github.com/alibaba/higress/v2/client/pkg/clientset/versioned/fake"
	higressinformer "github.com/alibaba/higress/v2/client/pkg/informers/externalversions"
	"github.com/alibaba/higress/v2/pkg/config/constants"
)

type Client interface {
	istiokube.Client

	// Higress returns the Higress kube client.
	Higress() higressclient.Interface

	// HigressInformer returns an informer for the higress client
	HigressInformer() higressinformer.SharedInformerFactory

	// KIngress return the Knative kube client
	KIngress() kingressclient.Interface

	KIngressInformer() kingressinformer.SharedInformerFactory
}

type client struct {
	istiokube.Client

	higress         higressclient.Interface
	higressInformer higressinformer.SharedInformerFactory

	kingress         kingressclient.Interface
	kingressInformer kingressinformer.SharedInformerFactory
	// If enable, will wait for cache syncs with extremely short delay. This should be used only for tests
	fastSync                bool
	informerWatchesPending  *atomic.Int32
	kinformerWatchesPending *atomic.Int32
}

const resyncInterval = 0

func NewFakeClient(objects ...runtime.Object) Client {
	c := &client{
		Client: istiokube.NewFakeClient(objects...),
	}
	c.higress = higressfake.NewSimpleClientset()
	c.higressInformer = higressinformer.NewSharedInformerFactoryWithOptions(c.higress, resyncInterval)
	c.informerWatchesPending = atomic.NewInt32(0)
	c.kingress = kingressfake.NewSimpleClientset()
	c.kingressInformer = kingressinformer.NewSharedInformerFactoryWithOptions(c.kingress, resyncInterval)
	c.kinformerWatchesPending = atomic.NewInt32(0)
	// https://github.com/kubernetes/kubernetes/issues/95372
	// There is a race condition in the client fakes, where events that happen between the List and Watch
	// of an informer are dropped. To avoid this, we explicitly manage the list and watch, ensuring all lists
	// have an associated watch before continuing.
	// This would likely break any direct calls to List(), but for now our tests don't do that anyways. If we need
	// to in the future we will need to identify the Lists that have a corresponding Watch, possibly by looking
	// at created Informers
	// an atomic.Int is used instead of sync.WaitGroup because wg.Add and wg.Wait cannot be called concurrently
	listReactor := func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		c.informerWatchesPending.Inc()
		return false, nil, nil
	}
	watchReactor := func(tracker clienttesting.ObjectTracker) func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
		return func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
			gvr := action.GetResource()
			ns := action.GetNamespace()
			watch, err := tracker.Watch(gvr, ns)
			if err != nil {
				return false, nil, err
			}
			c.informerWatchesPending.Dec()
			return true, watch, nil
		}
	}
	fc := c.higress.(*higressfake.Clientset)
	fc.PrependReactor("list", "&", listReactor)
	fc.PrependWatchReactor("*", watchReactor(fc.Tracker()))

	klistReactor := func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		c.kinformerWatchesPending.Inc()
		return false, nil, nil
	}
	kwatchReactor := func(tracker clienttesting.ObjectTracker) func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
		return func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
			gvr := action.GetResource()
			ns := action.GetNamespace()
			watch, err := tracker.Watch(gvr, ns)
			if err != nil {
				return false, nil, err
			}
			c.kinformerWatchesPending.Dec()
			return true, watch, nil
		}
	}
	fcknative := c.kingress.(*kingressfake.Clientset)
	fcknative.PrependReactor("list", "&", klistReactor)
	fcknative.PrependWatchReactor("*", kwatchReactor(fcknative.Tracker()))

	c.fastSync = true
	return c
}

func NewClient(clientConfig clientcmd.ClientConfig, cluster cluster.ID) (Client, error) {
	var c client
	istioClient, err := istiokube.NewClient(clientConfig, cluster)
	if err != nil {
		return nil, err
	}
	c.Client = istioClient

	c.higress, err = higressclient.NewForConfig(istioClient.RESTConfig())
	if err != nil {
		return nil, err
	}
	c.higressInformer = higressinformer.NewSharedInformerFactory(c.higress, resyncInterval)

	c.kingress, err = kingressclient.NewForConfig(istioClient.RESTConfig())
	if err != nil {
		return nil, err
	}
	if CheckKIngressCRDExist(istioClient.RESTConfig()) {
		c.kingressInformer = kingressinformer.NewSharedInformerFactory(c.kingress, resyncInterval)
	} else {
		c.kingressInformer = nil
	}

	return &c, nil
}

func (c *client) KIngress() kingressclient.Interface {
	return c.kingress
}

func (c *client) KIngressInformer() kingressinformer.SharedInformerFactory {
	return c.kingressInformer
}

func (c *client) Higress() higressclient.Interface {
	return c.higress
}

func (c *client) HigressInformer() higressinformer.SharedInformerFactory {
	return c.higressInformer
}

func (c *client) RunAndWait(stop <-chan struct{}) bool {
	c.Client.RunAndWait(stop)
	c.higressInformer.Start(stop)

	if c.fastSync {
		fastWaitForCacheSync(stop, c.higressInformer)
		err := wait.PollImmediate(time.Microsecond*100, wait.ForeverTestTimeout, func() (bool, error) {
			select {
			case <-stop:
				return false, fmt.Errorf("channel closed")
			default:
			}
			if c.informerWatchesPending.Load() == 0 {
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return false
		}
		return true
	} else {
		c.higressInformer.WaitForCacheSync(stop)
	}

	if c.kingressInformer != nil {
		c.kingressInformer.Start(stop)
		if c.fastSync {
			fastWaitForCacheSync(stop, c.kingressInformer)
			err := wait.PollImmediate(time.Microsecond*100, wait.ForeverTestTimeout, func() (bool, error) {
				select {
				case <-stop:
					return false, fmt.Errorf("channel closed")
				default:
				}
				if c.informerWatchesPending.Load() == 0 {
					return true, nil
				}
				return false, nil
			})
			if err != nil {
				return false
			}
			return true
		} else {
			c.kingressInformer.WaitForCacheSync(stop)
		}
	}
	return true
}

type reflectInformerSync interface {
	WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool
}

// Wait for cache sync immediately, rather than with 100ms delay which slows tests
// See https://github.com/kubernetes/kubernetes/issues/95262#issuecomment-703141573
func fastWaitForCacheSync(stop <-chan struct{}, informerFactory reflectInformerSync) {
	returnImmediately := make(chan struct{})
	close(returnImmediately)
	_ = wait.PollImmediate(time.Microsecond*100, wait.ForeverTestTimeout, func() (bool, error) {
		select {
		case <-stop:
			return false, fmt.Errorf("channel closed")
		default:
		}
		for _, synced := range informerFactory.WaitForCacheSync(returnImmediately) {
			if !synced {
				return false, nil
			}
		}
		return true, nil
	})
}

// Check Knative Ingress CRD
func CheckKIngressCRDExist(config *rest.Config) bool {
	apiExtClientset, err := apiExtensionsV1.NewForConfig(config)
	if err != nil {
		return false
	}
	crdList, err := apiExtClientset.CustomResourceDefinitions().List(context.TODO(), metaV1.ListOptions{})
	if err != nil {
		return false
	}
	for _, crd := range crdList.Items {
		if crd.Name == constants.KnativeIngressCRDName {
			return true
		}
	}
	return false
}

// EnableCrdWatcher enables the CRD watcher on the client.
func EnableCrdWatcher(c Client) Client {
	istiokube.EnableCrdWatcher(c.(*client).Client)
	return c
}
