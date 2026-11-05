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
	"errors"
	"fmt"
	"io"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/kubernetes"
)

const (
	Higress ComponentName = "higress"
)

type HigressComponent struct {
	profile  *helm.Profile
	started  bool
	opts     *ComponentOptions
	renderer helm.Renderer
	writer   io.Writer
	kubeCli  kubernetes.CLIClient
}

func (h *HigressComponent) ComponentName() ComponentName {
	return Higress
}

func (h *HigressComponent) Namespace() string {
	return h.opts.Namespace
}

func (h *HigressComponent) Enabled() bool {
	return true
}

func (h *HigressComponent) Run() error {
	// Parse latest version
	if h.opts.Version == helm.RepoLatestVersion {

		latestVersion, err := helm.ParseLatestVersion(h.opts.RepoURL, h.opts.Version, h.opts.Devel)
		if err != nil {
			return err
		}
		if !h.opts.Quiet {
			fmt.Fprintf(h.writer, "⚡️ Fetching Higress Helm Chart latest version \"%s\" \n", latestVersion)
		}

		// Reset Helm Chart version
		h.opts.Version = latestVersion
		h.renderer.SetVersion(latestVersion)
	}
	if !h.opts.Quiet {
		fmt.Fprintf(h.writer, "🏄 Downloading Higress Helm Chart version: %s, url: %s\n", h.opts.Version, h.opts.RepoURL)
	}
	if err := h.renderer.Init(); err != nil {
		return err
	}
	h.profile.HigressVersion = h.opts.Version
	h.started = true
	return nil
}

func (h *HigressComponent) RenderManifest() (string, error) {
	if !h.started {
		return "", nil
	}
	if !h.opts.Quiet {
		fmt.Fprintf(h.writer, "📦 Rendering Higress Helm Chart\n")
	}
	valsYaml, err := h.profile.ValuesYaml()
	if err != nil {
		return "", err
	}
	manifest, err2 := renderComponentManifest(valsYaml, h.renderer, true, h.ComponentName(), h.opts.Namespace)
	if err2 != nil {
		return "", err2
	}
	return manifest, nil
}

func NewHigressComponent(kubeCli kubernetes.CLIClient, profile *helm.Profile, writer io.Writer, opts ...ComponentOption) (Component, error) {
	newOpts := &ComponentOptions{}
	for _, opt := range opts {
		opt(newOpts)
	}

	if len(newOpts.RepoURL) == 0 {
		return nil, errors.New("Higress helm chart url can't be empty")
	}
	if newOpts.Name == "" {
		newOpts.Name = newOpts.ChartName
	}

	// Higress can only be installed by remote type
	renderer, err := helm.NewRemoteRenderer(
		helm.WithName(newOpts.Name),
		helm.WithChartName(newOpts.ChartName),
		helm.WithNamespace(newOpts.Namespace),
		helm.WithRepoURL(newOpts.RepoURL),
		helm.WithVersion(newOpts.Version),
		helm.WithCapabilities(newOpts.Capabilities),
		helm.WithRestConfig(kubeCli.RESTConfig()),
	)
	if err != nil {
		return nil, err
	}

	higressComponent := &HigressComponent{
		profile:  profile,
		renderer: renderer,
		opts:     newOpts,
		writer:   writer,
		kubeCli:  kubeCli,
	}
	return higressComponent, nil
}
