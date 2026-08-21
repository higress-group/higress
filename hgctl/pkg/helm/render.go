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

package helm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/manifests"
	"github.com/alibaba/higress/hgctl/pkg/util"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

const (
	// DefaultProfileName is the name of the default profile for installation.
	DefaultProfileName = "local-k8s"
	// DefaultProfileFilename is the name of the default profile yaml file for installation.
	DefaultProfileFilename = "local-k8s.yaml"
	// DefaultUninstallProfileName is the name of the default profile yaml file for uninstallation.
	DefaultUninstallProfileName = "local-k8s"

	// ChartsSubdirName       = "charts"
	profilesRoot = "profiles"

	RepoLatestVersion              = "latest"
	RepoChartIndexYamlHigressIndex = "higress"

	YAMLSeparator       = "\n---\n"
	NotesFileNameSuffix = ".txt"
)

func LoadValues(profileName string, chartsDir string) (string, error) {
	path := strings.Join([]string{profilesRoot, builtinProfileToFilename(profileName)}, "/")
	by, err := fs.ReadFile(manifests.BuiltinOrDir(chartsDir), path)
	if err != nil {
		return "", err
	}
	return string(by), nil
}

func readProfiles(chartsDir string) (map[string]bool, error) {
	profiles := map[string]bool{}
	f := manifests.BuiltinOrDir(chartsDir)
	dir, err := fs.ReadDir(f, profilesRoot)
	if err != nil {
		return nil, err
	}
	for _, f := range dir {
		if f.Name() == "_all.yaml" {
			continue
		}
		trimmedString := strings.TrimSuffix(f.Name(), ".yaml")
		if f.Name() != trimmedString {
			profiles[trimmedString] = true
		}
	}
	return profiles, nil
}

func builtinProfileToFilename(name string) string {
	if name == "" {
		return DefaultProfileFilename
	}
	return name + ".yaml"
}

// stripPrefix removes the given prefix from prefix.
func stripPrefix(path, prefix string) string {
	pl := len(strings.Split(prefix, "/"))
	pv := strings.Split(path, "/")
	return strings.Join(pv[pl:], "/")
}

// ListProfiles list all the profiles.
func ListProfiles(charts string) ([]string, error) {
	profiles, err := readProfiles(charts)
	if err != nil {
		return nil, err
	}
	return util.StringBoolMapToSlice(profiles), nil
}

var DefaultFilters = []util.FilterFunc{
	util.LicenseFilter,
	util.FormatterFilter,
	util.SpaceFilter,
}

// Renderer is responsible for rendering helm chart with new values.
type Renderer interface {
	Init() error
	RenderManifest(valsYaml string) (string, error)
	SetVersion(version string)
}

type RendererOptions struct {
	Name      string
	Namespace string

	// fields for LocalChartRenderer and LocalFileRenderer
	FS  fs.FS
	Dir string

	// fields for RemoteRenderer
	Version string
	RepoURL string

	// Capabilities
	Capabilities *chartutil.Capabilities

	// rest config
	restConfig *rest.Config
}

type RendererOption func(*RendererOptions)

func WithName(name string) RendererOption {
	return func(opts *RendererOptions) {
		opts.Name = name
	}
}

func WithNamespace(ns string) RendererOption {
	return func(opts *RendererOptions) {
		opts.Namespace = ns
	}
}

func WithFS(f fs.FS) RendererOption {
	return func(opts *RendererOptions) {
		opts.FS = f
	}
}

func WithDir(dir string) RendererOption {
	return func(opts *RendererOptions) {
		opts.Dir = dir
	}
}

func WithVersion(version string) RendererOption {
	return func(opts *RendererOptions) {
		opts.Version = version
	}
}

func WithRepoURL(repo string) RendererOption {
	return func(opts *RendererOptions) {
		opts.RepoURL = repo
	}
}

func WithCapabilities(capabilities *chartutil.Capabilities) RendererOption {
	return func(opts *RendererOptions) {
		opts.Capabilities = capabilities
	}
}

func WithRestConfig(config *rest.Config) RendererOption {
	return func(opts *RendererOptions) {
		opts.restConfig = config
	}
}

// LocalFileRenderer load yaml files from local file system
type LocalFileRenderer struct {
	Opts     *RendererOptions
	filesMap map[string]string
	Started  bool
}

func NewLocalFileRenderer(opts ...RendererOption) (Renderer, error) {
	newOpts := &RendererOptions{}
	for _, opt := range opts {
		opt(newOpts)
	}

	return &LocalFileRenderer{
		Opts:     newOpts,
		filesMap: make(map[string]string),
	}, nil
}

func (l *LocalFileRenderer) Init() error {
	fileNames, err := getFileNames(l.Opts.FS, l.Opts.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("chart of component %s doesn't exist", l.Opts.Name)
		}
		return fmt.Errorf("getFileNames err: %s", err)
	}
	for _, fileName := range fileNames {
		data, err := fs.ReadFile(l.Opts.FS, fileName)
		if err != nil {
			return fmt.Errorf("ReadFile %s err: %s", fileName, err)
		}

		l.filesMap[fileName] = string(data)
	}
	l.Started = true
	return nil
}

func (l *LocalFileRenderer) RenderManifest(valsYaml string) (string, error) {
	if !l.Started {
		return "", errors.New("LocalFileRenderer has not been init")
	}
	keys := make([]string, 0, len(l.filesMap))
	for key := range l.filesMap {
		keys = append(keys, key)
	}
	// to ensure that every manifest rendered by same values are the same
	sort.Strings(keys)

	var builder strings.Builder
	for i := 0; i < len(keys); i++ {
		file := l.filesMap[keys[i]]
		file = util.ApplyFilters(file, DefaultFilters...)
		// ignore empty manifest
		if file == "" {
			continue
		}
		if !strings.HasSuffix(file, YAMLSeparator) {
			file += YAMLSeparator
		}
		builder.WriteString(file)
	}
	return builder.String(), nil
}

func (l *LocalFileRenderer) SetVersion(version string) {
	l.Opts.Version = version
}

// LocalChartRenderer load chart from local file system
type LocalChartRenderer struct {
	Opts    *RendererOptions
	Chart   *chart.Chart
	Started bool
}

func (lr *LocalChartRenderer) Init() error {
	fileNames, err := getFileNames(lr.Opts.FS, lr.Opts.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("chart of component %s doesn't exist", lr.Opts.Name)
		}
		return fmt.Errorf("getFileNames err: %s", err)
	}
	var files []*loader.BufferedFile
	for _, fileName := range fileNames {
		data, err := fs.ReadFile(lr.Opts.FS, fileName)
		if err != nil {
			return fmt.Errorf("ReadFile %s err: %s", fileName, err)
		}
		// todo:// explain why we need to do this
		name := util.StripPrefix(fileName, lr.Opts.Dir)
		file := &loader.BufferedFile{
			Name: name,
			Data: data,
		}
		files = append(files, file)
	}
	newChart, err := loader.LoadFiles(files)
	if err != nil {
		return fmt.Errorf("load chart of component %s err: %s", lr.Opts.Name, err)
	}
	lr.Chart = newChart
	lr.Started = true
	return nil
}

func (lr *LocalChartRenderer) RenderManifest(valsYaml string) (string, error) {
	if !lr.Started {
		return "", errors.New("LocalChartRenderer has not been init")
	}
	return renderManifest(valsYaml, lr.Chart, true, lr.Opts, DefaultFilters...)
}

func (lr *LocalChartRenderer) SetVersion(version string) {
	lr.Opts.Version = version
}

func NewLocalChartRenderer(opts ...RendererOption) (Renderer, error) {
	newOpts := &RendererOptions{}
	for _, opt := range opts {
		opt(newOpts)
	}

	if err := verifyRendererOptions(newOpts); err != nil {
		return nil, fmt.Errorf("verify err: %s", err)
	}
	return &LocalChartRenderer{
		Opts: newOpts,
	}, nil
}

type RemoteRenderer struct {
	Opts    *RendererOptions
	Chart   *chart.Chart
	Started bool
}

func (rr *RemoteRenderer) initChartPathOptions() *action.ChartPathOptions {
	return &action.ChartPathOptions{
		RepoURL: rr.Opts.RepoURL,
		Version: rr.Opts.Version,
	}
}

func (rr *RemoteRenderer) Init() error {
	cpOpts := rr.initChartPathOptions()
	settings := cli.New()
	// using release name as chart name by default
	cp, err := locateChart(cpOpts, rr.Opts.Name, settings)
	if err != nil {
		return err
	}

	// Check chart dependencies to make sure all are present in /charts
	chartRequested, err := loader.Load(cp)
	if err != nil {
		return err
	}

	if err := verifyInstallable(chartRequested); err != nil {
		return err
	}

	rr.Chart = chartRequested
	rr.Started = true

	return nil
}

func (rr *RemoteRenderer) SetVersion(version string) {
	rr.Opts.Version = version
}

func (rr *RemoteRenderer) RenderManifest(valsYaml string) (string, error) {
	if !rr.Started {
		return "", errors.New("RemoteRenderer has not been init")
	}
	return renderManifest(valsYaml, rr.Chart, false, rr.Opts, DefaultFilters...)
}

func NewRemoteRenderer(opts ...RendererOption) (Renderer, error) {
	newOpts := &RendererOptions{}
	for _, opt := range opts {
		opt(newOpts)
	}

	return &RemoteRenderer{
		Opts: newOpts,
	}, nil
}

func verifyRendererOptions(opts *RendererOptions) error {
	if opts.Name == "" {
		return errors.New("missing component name for Renderer")
	}
	if opts.Namespace == "" {
		return errors.New("missing component namespace for Renderer")
	}
	if opts.FS == nil {
		return errors.New("missing chart FS for Renderer")
	}
	if opts.Dir == "" {
		return errors.New("missing chart dir for Renderer")
	}
	return nil
}

// read all files recursively under root path from a certain local file system
func getFileNames(f fs.FS, root string) ([]string, error) {
	var fileNames []string
	if err := fs.WalkDir(f, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fileNames = append(fileNames, path)
		return nil
	}); err != nil {
		return nil, err
	}
	return fileNames, nil
}

func verifyInstallable(cht *chart.Chart) error {
	typ := cht.Metadata.Type
	if typ == "" || typ == "application" {
		return nil
	}
	return fmt.Errorf("%s chart %s is not installable", typ, cht.Name())
}

func renderManifest(valsYaml string, cht *chart.Chart, builtIn bool, opts *RendererOptions, filters ...util.FilterFunc) (string, error) {
	valsMap := make(map[string]any)
	if err := yaml.Unmarshal([]byte(valsYaml), &valsMap); err != nil {
		return "", fmt.Errorf("unmarshal failed err: %s", err)
	}
	RelOpts := chartutil.ReleaseOptions{
		Name:      opts.Name,
		Namespace: opts.Namespace,
	}
	var caps *chartutil.Capabilities
	caps = opts.Capabilities
	if caps == nil {
		caps = chartutil.DefaultCapabilities
	}
	// maybe we need a configuration to change this caps
	resVals, err := chartutil.ToRenderValues(cht, valsMap, RelOpts, caps)
	if err != nil {
		return "", fmt.Errorf("ToRenderValues failed err: %s", err)
	}
	if builtIn {
		resVals["Values"].(chartutil.Values)["enabled"] = true
	}
	filesMap, err := engine.RenderWithClient(cht, resVals, opts.restConfig)
	if err != nil {
		return "", fmt.Errorf("Render chart failed err: %s", err)
	}
	keys := make([]string, 0, len(filesMap))
	for key := range filesMap {
		// remove notation files such as Notes.txt
		if strings.HasSuffix(key, NotesFileNameSuffix) {
			continue
		}
		keys = append(keys, key)
	}
	// to ensure that every manifest rendered by same values are the same
	sort.Strings(keys)

	var builder strings.Builder
	for i := 0; i < len(keys); i++ {
		file := filesMap[keys[i]]
		file = util.ApplyFilters(file, filters...)
		// ignore empty manifest
		if file == "" {
			continue
		}
		if !strings.HasSuffix(file, YAMLSeparator) {
			file += YAMLSeparator
		}
		builder.WriteString(file)
	}

	// render CRD
	crdFiles := cht.CRDObjects()
	// Sort crd files by name to ensure stable manifest output
	sort.Slice(crdFiles, func(i, j int) bool { return crdFiles[i].Name < crdFiles[j].Name })
	for _, crdFile := range crdFiles {
		f := string(crdFile.File.Data)
		// add yaml separator if the rendered file doesn't have one at the end
		f = strings.TrimSpace(f) + "\n"
		if !strings.HasSuffix(f, YAMLSeparator) {
			f += YAMLSeparator
		}
		builder.WriteString(f)
	}

	return builder.String(), nil
}

// locateChart locate the target chart path by sequential orders:
// 1. find local helm repository using "name-version.tgz" format
// 2. using downloader to pull remote chart
func locateChart(cpOpts *action.ChartPathOptions, name string, settings *cli.EnvSettings) (string, error) {
	name = strings.TrimSpace(name)
	version := strings.TrimSpace(cpOpts.Version)

	// check if it's in Helm's chart cache
	// cacheName is hardcoded as format of helm. eg: grafana-6.31.1.tgz
	cacheName := name + "-" + cpOpts.Version + ".tgz"
	cachePath := path.Join(settings.RepositoryCache, cacheName)
	if _, err := os.Stat(cachePath); err == nil {
		abs, err := filepath.Abs(cachePath)
		if err != nil {
			return abs, err
		}
		if cpOpts.Verify {
			if _, err := downloader.VerifyChart(abs, cpOpts.Keyring); err != nil {
				return "", err
			}
		}
		return abs, nil
	}

	dl := downloader.ChartDownloader{
		Out:     os.Stdout,
		Keyring: cpOpts.Keyring,
		Getters: getter.All(settings),
		Options: []getter.Option{
			getter.WithPassCredentialsAll(cpOpts.PassCredentialsAll),
			getter.WithTLSClientConfig(cpOpts.CertFile, cpOpts.KeyFile, cpOpts.CaFile),
			getter.WithInsecureSkipVerifyTLS(cpOpts.InsecureSkipTLSverify),
		},
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
	}

	if cpOpts.Verify {
		dl.Verify = downloader.VerifyAlways
	}
	if cpOpts.RepoURL != "" {
		chartURL, err := repo.FindChartInAuthAndTLSAndPassRepoURL(cpOpts.RepoURL, cpOpts.Username, cpOpts.Password, name, version,
			cpOpts.CertFile, cpOpts.KeyFile, cpOpts.CaFile, cpOpts.InsecureSkipTLSverify, cpOpts.PassCredentialsAll, getter.All(settings))
		if err != nil {
			return "", err
		}
		name = chartURL

		// Only pass the user/pass on when the user has said to or when the
		// location of the chart repo and the chart are the same domain.
		u1, err := url.Parse(cpOpts.RepoURL)
		if err != nil {
			return "", err
		}
		u2, err := url.Parse(chartURL)
		if err != nil {
			return "", err
		}

		// Host on URL (returned from url.Parse) contains the port if present.
		// This check ensures credentials are not passed between different
		// services on different ports.
		if cpOpts.PassCredentialsAll || (u1.Scheme == u2.Scheme && u1.Host == u2.Host) {
			dl.Options = append(dl.Options, getter.WithBasicAuth(cpOpts.Username, cpOpts.Password))
		} else {
			dl.Options = append(dl.Options, getter.WithBasicAuth("", ""))
		}
	} else {
		dl.Options = append(dl.Options, getter.WithBasicAuth(cpOpts.Username, cpOpts.Password))
	}

	// if RepositoryCache doesn't exist, create it
	if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
		return "", err
	}

	filename, _, err := dl.DownloadTo(name, version, settings.RepositoryCache)
	if err != nil {
		return "", err
	}

	fileAbsPath, err := filepath.Abs(filename)
	if err != nil {
		return filename, err
	}
	return fileAbsPath, nil
}

func ParseLatestVersion(repoUrl string, version string, devel bool) (string, error) {
	cpOpts := &action.ChartPathOptions{
		RepoURL: repoUrl,
		Version: version,
	}
	settings := cli.New()

	indexURL, err := repo.ResolveReferenceURL(repoUrl, "index.yaml")
	if err != nil {
		return "", err
	}

	u, err := url.Parse(repoUrl)
	if err != nil {
		return "", fmt.Errorf("invalid chart URL format: %s", repoUrl)
	}

	client, err := getter.All(settings).ByScheme(u.Scheme)
	if err != nil {
		return "", fmt.Errorf("could not find protocol handler for: %s", u.Scheme)
	}

	resp, err := client.Get(indexURL,
		getter.WithURL(cpOpts.RepoURL),
		getter.WithInsecureSkipVerifyTLS(cpOpts.InsecureSkipTLSverify),
		getter.WithTLSClientConfig(cpOpts.CertFile, cpOpts.KeyFile, cpOpts.CaFile),
		getter.WithBasicAuth(cpOpts.Username, cpOpts.Password),
		getter.WithPassCredentialsAll(cpOpts.PassCredentialsAll),
	)
	if err != nil {
		return "", err
	}

	index, err := io.ReadAll(resp)
	if err != nil {
		return "", err
	}

	indexFile, err := loadIndex(index)
	if err != nil {
		return "", err
	}

	// get higress helm chart latest version
	if entries, ok := indexFile.Entries[RepoChartIndexYamlHigressIndex]; ok {
		if len(entries) == 0 {
			return "", errors.New("no versions found for higress chart in repository index")
		}
		if devel {
			return entries[0].AppVersion, nil
		}

		if chatVersion, err := indexFile.Get(RepoChartIndexYamlHigressIndex, ""); err != nil {
			return "", errors.New("can't find higress latest version")
		} else {
			return chatVersion.Version, nil
		}

	}

	return "", errors.New("can't find higress latest version")
}

// loadIndex loads an index file and does minimal validity checking.
//
// The source parameter is only used for logging.
// This will fail if API Version is not set (ErrNoAPIVersion) or if the unmarshal fails.
func loadIndex(data []byte) (*repo.IndexFile, error) {
	i := &repo.IndexFile{}
	if len(data) == 0 {
		return i, errors.New("empty index.yaml file")
	}
	if err := jsonOrYamlUnmarshal(data, i); err != nil {
		return i, err
	}
	for _, cvs := range i.Entries {
		for idx := len(cvs) - 1; idx >= 0; idx-- {
			if cvs[idx] == nil {
				continue
			}
			if cvs[idx].APIVersion == "" {
				cvs[idx].APIVersion = chart.APIVersionV1
			}
			if err := cvs[idx].Validate(); err != nil {
				cvs = append(cvs[:idx], cvs[idx+1:]...)
			}
		}
	}
	i.SortEntries()
	if i.APIVersion == "" {
		return i, errors.New("no API version specified")
	}
	return i, nil
}

// jsonOrYamlUnmarshal unmarshals the given byte slice containing JSON or YAML
// into the provided interface.
//
// It automatically detects whether the data is in JSON or YAML format by
// checking its validity as JSON. If the data is valid JSON, it will use the
// `encoding/json` package to unmarshal it. Otherwise, it will use the
// `sigs.k8s.io/yaml` package to unmarshal the YAML data.
func jsonOrYamlUnmarshal(b []byte, i interface{}) error {
	if json.Valid(b) {
		return json.Unmarshal(b, i)
	}
	return yaml.UnmarshalStrict(b, i)
}
