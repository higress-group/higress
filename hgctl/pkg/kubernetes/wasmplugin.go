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

package kubernetes

import (
	"context"

	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	DefaultHigressNamespace = "higress-system"
	HigressExtGroup         = "extensions.higress.io"
	HigressExtVersion       = "v1alpha1"
	HigressExtAPIVersion    = HigressExtGroup + "/" + HigressExtVersion

	WasmPluginKind     = "WasmPlugin"
	WasmPluginResource = "wasmplugins"
)

var (
	HigressNamespace = DefaultHigressNamespace
	WasmPluginGVK    = schema.GroupVersionKind{Group: HigressExtGroup, Version: HigressExtVersion, Kind: WasmPluginKind}
	WasmPluginGVR    = schema.GroupVersionResource{Group: HigressExtGroup, Version: HigressExtVersion, Resource: WasmPluginResource}
)

func AddHigressNamespaceFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&HigressNamespace, "namespace", "n",
		DefaultHigressNamespace, "Namespace where Higress was installed")
}

type WasmPluginClient struct {
	dyn *DynamicClient
}

func NewWasmPluginClient(dynClient *DynamicClient) *WasmPluginClient {
	return &WasmPluginClient{dynClient}
}

func (c WasmPluginClient) Get(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return c.dyn.Get(ctx, WasmPluginGVR, HigressNamespace, name)
}

func (c WasmPluginClient) List(ctx context.Context) (*unstructured.UnstructuredList, error) {
	return c.dyn.List(ctx, WasmPluginGVR, HigressNamespace)
}

func (c WasmPluginClient) Create(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.CreateInNamespace(ctx, HigressNamespace, obj)
}

// CreateInNamespace creates a plugin in the namespace selected by the caller.
func (c WasmPluginClient) CreateInNamespace(ctx context.Context, namespace string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.dyn.Create(ctx, WasmPluginGVR, namespace, obj)
}

func (c WasmPluginClient) Delete(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return c.dyn.Delete(ctx, WasmPluginGVR, HigressNamespace, name)
}

func (c WasmPluginClient) Update(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.dyn.Update(ctx, WasmPluginGVR, HigressNamespace, obj)
}

// TODO(WeixinX): Will be changed to WasmPlugin specific Client instead of Unstructured
type DynamicClient struct {
	config *rest.Config
	client dynamic.Interface
}

func NewDynamicClient(clientConfig clientcmd.ClientConfig) (*DynamicClient, error) {
	var (
		c   DynamicClient
		err error
	)

	c.config, err = clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	c.client, err = dynamic.NewForConfig(c.config)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (c DynamicClient) Get(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	return c.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c DynamicClient) List(ctx context.Context, gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
	return c.client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
}

func (c DynamicClient) Create(ctx context.Context, gvr schema.GroupVersionResource, namespace string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.client.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
}

func (c DynamicClient) Delete(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	result, err := c.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	err = c.client.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c DynamicClient) Update(ctx context.Context, gvr schema.GroupVersionResource, namespace string,
	obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.client.Resource(gvr).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
}
