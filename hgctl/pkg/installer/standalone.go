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
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/util"
)

const (
	defaultHttpRequestTimeout = 15 * time.Second
	defaultHttpMaxTry         = 3
	defaultHttpBufferSize     = 1024 * 1024 * 2
)

type StandaloneComponent struct {
	profile     *helm.Profile
	started     bool
	opts        *ComponentOptions
	writer      io.Writer
	httpFetcher *util.HTTPFetcher
	agent       *Agent
}

func (s *StandaloneComponent) Install() error {
	if !s.opts.Quiet {
		fmt.Fprintf(s.writer, "\n🏄 Downloading installer from  %s\n", s.opts.RepoURL)
	}
	// download get-higress.sh
	data, err := s.httpFetcher.Fetch(context.Background(), s.opts.RepoURL)
	if err != nil {
		return err
	}
	// write installer binary shell
	if err := util.WriteFileString(s.agent.installBinaryName, string(data), os.ModePerm); err != nil {
		return err
	}
	// start to install higress
	if err := s.agent.Install(); err != nil {
		return err
	}
	// Set Higress version
	if version, err := s.agent.Version(); err == nil {
		s.profile.HigressVersion = version
	}
	return nil
}

func (s *StandaloneComponent) UnInstall() error {
	if err := s.agent.Uninstall(); err != nil {
		return err
	}
	return nil
}

func (s *StandaloneComponent) Upgrade() error {
	if !s.opts.Quiet {
		fmt.Fprintf(s.writer, "\n🏄 Downloading installer from  %s\n", s.opts.RepoURL)
	}
	// download get-higress.sh
	data, err := s.httpFetcher.Fetch(context.Background(), s.opts.RepoURL)
	if err != nil {
		return err
	}
	// write installer binary shell
	if err := util.WriteFileString(s.agent.installBinaryName, string(data), os.ModePerm); err != nil {
		return err
	}
	// start to upgrade higress
	if err := s.agent.Upgrade(); err != nil {
		return err
	}
	return nil
}

func NewStandaloneComponent(profile *helm.Profile, writer io.Writer, opts ...ComponentOption) (*StandaloneComponent, error) {
	newOpts := &ComponentOptions{}
	for _, opt := range opts {
		opt(newOpts)
	}

	httpFetcher := util.NewHTTPFetcher(defaultHttpRequestTimeout, defaultHttpMaxTry, defaultHttpBufferSize)
	if err := prepareProfile(profile); err != nil {
		return nil, err
	}
	agent := NewAgent(profile, writer, newOpts.Quiet)
	standaloneComponent := &StandaloneComponent{
		profile:     profile,
		opts:        newOpts,
		writer:      writer,
		httpFetcher: httpFetcher,
		agent:       agent,
	}
	return standaloneComponent, nil
}

func prepareProfile(profile *helm.Profile) error {
	if len(profile.InstallPackagePath) == 0 {
		dir, err := GetDefaultInstallPackagePath()
		if err != nil {
			return err
		}
		profile.InstallPackagePath = dir
	}

	if _, err := os.Stat(profile.InstallPackagePath); os.IsNotExist(err) {
		if err = os.MkdirAll(profile.InstallPackagePath, os.ModePerm); err != nil {
			return err
		}
	}

	// parse INSTALLPACKAGEPATH in storage.url
	if strings.HasPrefix(profile.Storage.Url, "file://") {
		profile.Storage.Url = strings.ReplaceAll(profile.Storage.Url, "${INSTALLPACKAGEPATH}", profile.InstallPackagePath)
	}

	return nil
}
