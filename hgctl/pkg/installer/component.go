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

package installer

import (
	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/util"
	"helm.sh/helm/v3/pkg/chartutil"
	"sigs.k8s.io/yaml"
)

type ComponentName string

var ComponentMap = map[ComponentName]struct{}{
	Higress: {},
	Istio:   {},
}

type Component interface {
	// ComponentName returns the name of the component.
	ComponentName() ComponentName
	// Namespace returns the namespace for the component.
	Namespace() string
	// Enabled reports whether the component is enabled.
	Enabled() bool
	// Run starts the component. Must be called before the component is used.
	Run() error
	RenderManifest() (string, error)
}

type ComponentOptions struct {
	Name      string
	Namespace string
	// local
	ChartPath string
	// remote
	RepoURL   string
	ChartName string
	Version   string
	Quiet     bool
	// Capabilities
	Capabilities *chartutil.Capabilities
	// devel
	Devel bool
}

type ComponentOption func(*ComponentOptions)

func WithComponentName(name string) ComponentOption {
	return func(opts *ComponentOptions) {
		opts.Name = name
	}
}

func WithComponentNamespace(namespace string) ComponentOption {
	return func(opts *ComponentOptions) {
		opts.Namespace = namespace
	}
}

func WithComponentChartPath(path string) ComponentOption {
	return func(opts *ComponentOptions) {
		opts.ChartPath = path
	}
}

func WithComponentChartName(chartName string) ComponentOption {
	return func(opts *ComponentOptions) {
		opts.ChartName = chartName
	}
}

func WithComponentRepoURL(url string) ComponentOption {
	return func(opts *ComponentOptions) {
		opts.RepoURL = url
	}
}

func WithComponentVersion(version string) ComponentOption {
	return func(opts *ComponentOptions) {
		opts.Version = version
	}
}

func WithComponentCapabilities(capabilities *chartutil.Capabilities) ComponentOption {
	return func(opts *ComponentOptions) {
		opts.Capabilities = capabilities
	}
}

func WithQuiet() ComponentOption {
	return func(opts *ComponentOptions) {
		opts.Quiet = true
	}
}

func WithDevel(devel bool) ComponentOption {
	return func(opts *ComponentOptions) {
		opts.Devel = devel
	}
}

func renderComponentManifest(spec any, renderer helm.Renderer, addOn bool, name ComponentName, namespace string) (string, error) {
	var valsBytes []byte
	var valsYaml string
	var err error
	if yamlString, ok := spec.(string); ok {
		valsYaml = yamlString
	} else {
		if !util.IsValueNil(spec) {
			valsBytes, err = yaml.Marshal(spec)
			if err != nil {
				return "", err
			}
			valsYaml = string(valsBytes)
		}
	}
	final, err := renderer.RenderManifest(valsYaml)
	if err != nil {
		return "", err
	}
	return final, nil
}
