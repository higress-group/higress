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

package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/plugin/option"
	ptypes "github.com/alibaba/higress/hgctl/pkg/plugin/types"
	"github.com/alibaba/higress/hgctl/pkg/plugin/utils"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	DefaultBuilderRepository = "higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/wasm-go-builder"
	DefaultBuilderGo         = "1.19"
	DefaultBuilderTinyGo     = "0.28.1"
	DefaultBuilderOras       = "1.0.0"

	MediaTypeSpec      = "application/vnd.module.wasm.spec.v1+yaml"
	MediaTypeREADME    = "application/vnd.module.wasm.doc.v1+markdown"
	MediaTypeREADME_ZH = "application/vnd.module.wasm.doc.v1.zh+markdown"
	MediaTypeREADME_EN = "application/vnd.module.wasm.doc.v1.en+markdown"
	MediaTypeIcon      = "application/vnd.module.wasm.icon.v1+png"
	MediaTypePlugin    = "application/vnd.oci.image.layer.v1.tar+gzip"

	HostTempDirPattern     = "higress-wasm-go-build-*"
	HostDockerEntryPattern = "higress-wasm-go-build-docker-entrypoint-*.sh"

	ContainerWorkDir       = "/workspace"
	ContainerTempDir       = "/higress_temp" // the directory to temporarily store the build products
	ContainerOutDir        = "/output"
	ContainerDockerAuth    = "/root/.docker/config.json"
	ContainerEntryFile     = "docker-entrypoint.sh"
	ContainerEntryFilePath = "/" + ContainerEntryFile
)

type Builder struct {
	OptionFile string
	option.BuildOptions
	Username, Password string

	repository       string
	tempDir          string
	dockerEntrypoint string
	uid, gid         string
	manualClean      bool

	containerID   string
	containerConf types.ContainerCreateConfig
	dockerCli     *client.Client
	w             io.Writer
	sig           chan os.Signal // watch interrupt
	stop          chan struct{}  // stop the build process when an interruption occurs
	done          chan struct{}  // signal that the build process is finished

	utils.Debugger
	*utils.YesOrNoPrinter
}

func NewBuilder(f ConfigFunc) (*Builder, error) {
	b := new(Builder)
	if err := b.config(f); err != nil {
		return nil, err
	}

	return b, nil
}

func NewCommand() *cobra.Command {
	var bld Builder
	v := viper.New()

	buildCmd := &cobra.Command{
		Use:     "build",
		Aliases: []string{"bld", "b"},
		Short:   "Build Golang WASM plugin",
		Example: `  # If the option.yaml file exists in the current path, do the following:
  hgctl plugin build

  # Using "--model(-s)" to specify the WASM plugin configuration structure name, e.g. "HelloWorldConfig"
  hgctl plugin build --model HelloWorldConfig

  # Using "--output-type(-t)" and "--output-dest(-d)" to push the build products as an OCI image to the specified repository
  docker login
  hgctl plugin build -s BasicAuthConfig -t image -d docker.io/<your_username>/<your_image>
  `,
		PreRun: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(bld.config(func(b *Builder) error {
				return b.parseOptions(v, cmd)
			}))
		},

		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(bld.Build())
		},
	}

	bld.bindFlags(v, buildCmd.PersistentFlags())

	return buildCmd
}

func (b *Builder) bindFlags(v *viper.Viper, flags *pflag.FlagSet) {
	option.AddOptionFileFlag(&b.OptionFile, flags)
	flags.StringVarP(&b.Username, "username", "u", "", "Username for pushing image to the docker repository")
	flags.StringVarP(&b.Password, "password", "p", "", "Password for pushing image to the docker repository")
	v.BindPFlags(flags)

	// this binding ensures that flags explicitly set on the command line have the
	// highest priority, and if they are not set, they are read from the configuration file.
	flags.StringP("builder-go", "g", DefaultBuilderGo, "Golang version in the official builder image")
	v.BindPFlag("build.builder.go", flags.Lookup("builder-go"))
	v.SetDefault("build.builder.go", DefaultBuilderGo)

	flags.StringP("builder-tinygo", "n", DefaultBuilderTinyGo, "TinyGo version in the official builder image")
	v.BindPFlag("build.builder.tinygo", flags.Lookup("builder-tinygo"))
	v.SetDefault("build.builder.tinygo", DefaultBuilderTinyGo)

	flags.StringP("builder-oras", "r", DefaultBuilderOras, "ORAS version in official the builder image")
	v.BindPFlag("build.builder.oras", flags.Lookup("builder-oras"))
	v.SetDefault("build.builder.oras", DefaultBuilderOras)

	flags.StringP("input", "i", "./", "Directory of the WASM plugin project to be built")
	v.BindPFlag("build.input", flags.Lookup("input"))
	v.SetDefault("build.input", "./")

	flags.StringP("output-type", "t", "files", "Output type of the build products. [files, image]")
	v.BindPFlag("build.output.type", flags.Lookup("output-type"))
	v.SetDefault("build.output.type", "files")

	flags.StringP("output-dest", "d", "./out", "Output destination of the build products")
	v.BindPFlag("build.output.dest", flags.Lookup("output-dest"))
	v.SetDefault("build.output.dest", "./out")

	flags.StringP("docker-auth", "a", "~/.docker/config.json", "Authentication configuration for pushing image to the docker repository")
	v.BindPFlag("build.docker-auth", flags.Lookup("docker-auth"))
	v.SetDefault("build.docker-auth", "~/.docker/config.json")

	flags.StringP("model-dir", "m", "./", "Directory of the WASM plugin configuration structure")
	v.BindPFlag("build.model-dir", flags.Lookup("model-dir"))
	v.SetDefault("build.model-dir", "./")

	flags.StringP("model", "s", "", "Structure name of the WASM plugin configuration")
	v.BindPFlag("build.model", flags.Lookup("model"))
	v.SetDefault("build.model", "PluginConfig")

	flags.BoolP("debug", "", false, "Enable debug mode")
	v.BindPFlag("build.debug", flags.Lookup("debug"))
	v.SetDefault("build.debug", false)
}

func (b *Builder) Build() (err error) {
	b.Debugf("build options: \n%s\n", b.String())

	go func() {
		err = b.doBuild()
	}()

	// wait for an interruption to occur or finishing the build
	select {
	case <-b.sig:
		b.interrupt()
		b.Nof("\nInterrupt ...\n")
		// wait for the doBuild process to exit, otherwise there will be unexpected bugs
		b.waitForFinished()
		// if the build process is interrupted, then we ignore the flag `manualClean` and clean up
		// TODO(WeixinX): How do we clean up uploaded image when an interruption occurs?
		b.Debugln("clean up for interrupting ...")
		b.CleanupForError()
		os.Exit(0)

	case <-b.done:
		if err != nil {
			if !b.manualClean {
				b.Debugln("clean up for error ...")
				b.CleanupForError()
			}
			return
		}
		if !b.manualClean {
			b.Debugln("clean up for normal ...")
			b.Cleanup()
		}
	}

	return
}

var (
	waitIcon       = "[-]"
	successfulIcon = "[√]"
)

func (b *Builder) doBuild() (err error) {
	// finish here does not mean that the build was successful,
	// but that the doBuild process is complete
	defer b.finish()

	if err = b.generateMetadata(); err != nil {
		return errors.Wrap(err, "failed to generate wasm plugin metadata files")
	}

	b.Printf("%s pull the builder image ...\n", waitIcon)
	ctx := context.TODO()
	if err = b.imagePull(ctx); err != nil {
		return errors.Wrapf(err, "failed to pull the builder image %s", b.builderImageRef())
	}
	b.Yesf("%s pull the builder image: %s\n", successfulIcon, b.builderImageRef())

	if err = b.addContainerConfByOutType(); err != nil {
		return errors.Wrapf(err, "failed to add the additional container configuration for output type %q", b.Output.Type)
	}

	b.Printf("%s create the builder container ...\n", waitIcon)
	if err = b.containerCreate(ctx); err != nil {
		return errors.Wrap(err, "failed to create the builder container")
	}
	b.Yesf("%s create the builder container: %s\n", successfulIcon, b.containerID)

	b.Printf("%s start the builder container ...\n", waitIcon)
	if err = b.containerStart(ctx); err != nil {
		return errors.Wrap(err, "failed to start the builder container")
	}

	if b.Output.Type == "files" {
		b.Yesf("%s finish building!\n", successfulIcon)
	} else if b.Output.Type == "image" {
		b.Yesf("%s finish building and pushing!\n", successfulIcon)
	}

	return nil
}

var errBuildAbort = errors.New("build aborted")

func (b *Builder) generateMetadata() error {
	// spec.yaml
	if b.isInterrupted() {
		return errBuildAbort
	}
	spec, err := os.Create(b.SpecYAMLPath())
	if err != nil {
		return err
	}
	defer spec.Close()
	meta, err := ptypes.ParseGoSrc(b.ModelDir, b.Model)
	if err != nil {
		return err
	}
	if err = utils.MarshalYamlWithIndentTo(spec, meta, 2); err != nil {
		return err
	}

	// TODO(WeixinX): More languages need to be supported
	// README.md is required, README_{lang}.md is optional
	if b.isInterrupted() {
		return errBuildAbort
	}
	usages, err := meta.GetUsages()
	if err != nil {
		return errors.Wrap(err, "failed to get wasm usage")
	}
	for i, u := range usages {
		// since `usages` are ordered by `I18nType` and currently only `en-US` and
		// `zh-CN` are available, en-US is the default README.md language when en-US is
		// present (because after sorting it is in the first place)
		suffix := true
		if i == 0 {
			suffix = false
		}
		if err = genMarkdownUsage(&u, b.tempDir, suffix); err != nil {
			return err
		}
	}

	return nil
}

func (b *Builder) imagePull(ctx context.Context) error {
	if b.isInterrupted() {
		return errBuildAbort
	}
	r, err := b.dockerCli.ImagePull(ctx, b.builderImageRef(), types.ImagePullOptions{})
	if err != nil {
		return err
	}

	if b.isInterrupted() {
		return errBuildAbort
	}
	io.Copy(b.w, r)

	return nil
}

func (b *Builder) addContainerConfByOutType() error {
	if b.isInterrupted() {
		return errBuildAbort
	}

	var err error
	switch b.Output.Type {
	case "files":
		err = b.filesHandler()
	case "image":
		err = b.imageHandler()
	default:
		return errors.New("invalid output option, output type is unknown")
	}
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) containerCreate(ctx context.Context) error {
	if b.isInterrupted() {
		return errBuildAbort
	}

	resp, err := b.dockerCli.ContainerCreate(ctx, b.containerConf.Config, b.containerConf.HostConfig,
		b.containerConf.NetworkingConfig, b.containerConf.Platform, b.containerConf.Name)
	if err != nil {
		return err
	}
	b.containerID = resp.ID

	return nil
}

func (b *Builder) containerStart(ctx context.Context) error {
	if b.isInterrupted() {
		return errBuildAbort
	}
	if err := b.dockerCli.ContainerStart(ctx, b.containerID, types.ContainerStartOptions{}); err != nil {
		return err
	}

	if b.isInterrupted() {
		return errBuildAbort
	}
	statusCh, errCh := b.dockerCli.ContainerWait(ctx, b.containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-statusCh:
	}

	if b.isInterrupted() {
		return errBuildAbort
	}
	logs, err := b.dockerCli.ContainerLogs(ctx, b.containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return err
	}

	if b.isInterrupted() {
		return errBuildAbort
	}
	_, err = stdcopy.StdCopy(b.w, b.w, logs)
	if err != nil {
		return err
	}

	return nil
}

var errWriteDockerEntrypoint = errors.New("failed to write docker entrypoint")

func (b *Builder) filesHandler() error {
	b.containerConf.HostConfig.Mounts = append(b.containerConf.HostConfig.Mounts, mount.Mount{
		// output dir for the build products
		Type:   mount.TypeBind,
		Source: b.Output.Dest,
		Target: ContainerOutDir,
	})

	ft := &FilesTmplFields{
		BuildSrcDir:  ContainerWorkDir,
		BuildDestDir: ContainerTempDir,
		Output:       ContainerOutDir,
		UID:          b.uid,
		GID:          b.uid,
		Debug:        b.Debug,
	}

	if err := genFilesDockerEntrypoint(ft, b.dockerEntrypoint); err != nil {
		return errors.Wrap(err, errWriteDockerEntrypoint.Error())
	}

	return nil
}

var optionalProducts = [][2]string{
	{"README_ZH.md", MediaTypeREADME_ZH},
	{"README_EN.md", MediaTypeREADME_EN},
	{"icon.png", MediaTypeIcon},
}

// TODO(WeixinX): If the image exists, no push is performed
func (b *Builder) imageHandler() error {
	products := ""
	for i, p := range optionalProducts {
		fileName := p[0]
		mediaType := p[1]
		if i == 0 {
			products = fmt.Sprintf("%s %s", fileName, mediaType)
		} else {
			products = fmt.Sprintf("%s %s %s", products, fileName, mediaType)
		}
	}

	// spec.yaml, README.md and plugin.tar.gz are required
	basicCmd := fmt.Sprintf("oras push %s -u %s -p %s ./spec.yaml:%s ./README.md:%s",
		b.Output.Dest, b.Username, b.Password, MediaTypeSpec, MediaTypeREADME)

	if b.Username == "" || b.Password == "" {
		basicCmd = fmt.Sprintf("oras push %s ./spec.yaml:%s ./README.md:%s",
			b.Output.Dest, MediaTypeSpec, MediaTypeREADME)

		b.containerConf.HostConfig.Mounts = append(b.containerConf.HostConfig.Mounts, mount.Mount{
			// docker auth
			Type:   mount.TypeBind,
			Source: b.DockerAuth,
			Target: ContainerDockerAuth,
		})
	}

	it := &ImageTmplFields{
		BuildSrcDir:     ContainerWorkDir,
		BuildDestDir:    ContainerTempDir,
		Output:          ContainerOutDir,
		Username:        b.Username,
		Password:        b.Password,
		BasicCmd:        basicCmd,
		Products:        products,
		MediaTypePlugin: MediaTypePlugin,
		Debug:           b.Debug,
	}

	if err := genImageDockerEntrypoint(it, b.dockerEntrypoint); err != nil {
		return errors.Wrap(err, errWriteDockerEntrypoint.Error())
	}

	return nil
}

// ConfigFunc is customized to set the fields of Builder
type ConfigFunc func(b *Builder) error

func (b *Builder) config(f ConfigFunc) (err error) {
	if err = f(b); err != nil {
		return err
	}

	// builder-go
	b.Builder.Go = strings.TrimSpace(b.Builder.Go)
	if b.Builder.Go == "" {
		b.Builder.Go = DefaultBuilderGo
	}

	// builder-tinygo
	b.Builder.TinyGo = strings.TrimSpace(b.Builder.TinyGo)
	if b.Builder.TinyGo == "" {
		b.Builder.TinyGo = DefaultBuilderTinyGo
	}

	// builder-oras
	b.Builder.Oras = strings.TrimSpace(b.Builder.Oras)
	if b.Builder.Oras == "" {
		b.Builder.Oras = DefaultBuilderOras
	}

	// input
	b.Input = strings.TrimSpace(b.Input)
	if b.Input == "" {
		b.Input = "./"
	}
	inp, err := utils.GetAbsolutePath(b.Input)
	if err != nil {
		return errors.Wrapf(err, "failed to parse input option %q", b.Input)
	}
	b.Input = inp

	// output-type
	b.Output.Type = strings.ToLower(strings.TrimSpace(b.Output.Type))
	if b.Output.Type == "" {
		b.Output.Type = "files"
	}
	if b.Output.Type != "files" && b.Output.Type != "image" {
		return errors.Errorf("invalid output type: %q, must be `files` or `image`", b.Output.Type)
	}

	// output-dest
	b.Output.Dest = strings.TrimSpace(b.Output.Dest)
	if b.Output.Dest == "" {
		b.Output.Dest = "./out"
	}
	out := b.Output.Dest
	if b.Output.Type == "files" {
		out, err = utils.GetAbsolutePath(b.Output.Dest)
		if err != nil {
			return errors.Wrapf(err, "failed to parse output destination %q", b.Output.Dest)
		}
		err = os.MkdirAll(b.Output.Dest, 0o755)
		if err != nil && !os.IsExist(err) {
			return errors.Wrapf(err, "failed to create output destination %q", b.Output.Dest)
		}
	}
	b.Output.Dest = out

	// docker-auth
	b.DockerAuth = strings.TrimSpace(b.DockerAuth)
	if b.DockerAuth == "" {
		b.DockerAuth = "~/.docker/config.json"
	}
	auth, err := utils.GetAbsolutePath(b.DockerAuth)
	if err != nil {
		return errors.Wrapf(err, "failed to parse docker authentication %q", b.DockerAuth)
	}
	b.DockerAuth = auth

	// model-dir
	b.ModelDir = strings.TrimSpace(b.ModelDir)
	if b.ModelDir == "" {
		b.ModelDir = "./"
	}

	// option-file/username/password/model/debug: nothing to deal with

	// the unexported fields that users do not need to care about are as follows:
	b.repository = DefaultBuilderRepository

	b.tempDir, err = os.MkdirTemp("", HostTempDirPattern)
	if err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "failed to create the host temporary dir")
	}

	dockerEp, err := os.CreateTemp("", HostDockerEntryPattern)
	if err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "failed to create the docker entrypoint file")
	}
	err = dockerEp.Chmod(0o777)
	if err != nil {
		return err
	}
	b.dockerEntrypoint = dockerEp.Name()
	dockerEp.Close()

	u, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "failed to get the current user information")
	}
	b.uid, b.gid = u.Uid, u.Gid

	b.containerConf = types.ContainerCreateConfig{
		Name: "higress-wasm-go-builder",
		Config: &container.Config{
			Image: b.builderImageRef(),
			Env: []string{
				"GO111MODULE=on",
				"GOPROXY=https://goproxy.cn,direct",
			},
			WorkingDir: ContainerWorkDir,
			Entrypoint: []string{ContainerEntryFilePath},
		},
		HostConfig: &container.HostConfig{
			NetworkMode: "host",
			Mounts: []mount.Mount{
				{ // input dir that includes the wasm plugin source: main.go ...
					Type:   mount.TypeBind,
					Source: b.Input,
					Target: ContainerWorkDir,
				},
				{ // temp dir that includes the wasm plugin metadata: spec.yaml and README.md ...
					Type:   mount.TypeBind,
					Source: b.tempDir,
					Target: ContainerTempDir,
				},
				{ // entrypoint
					Type:   mount.TypeBind,
					Source: b.dockerEntrypoint,
					Target: ContainerEntryFilePath,
				},
			},
		},
	}

	if b.dockerCli == nil {
		b.dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return errors.Wrap(err, "failed to initialize the docker client")
		}
	}

	if b.w == nil {
		b.w = os.Stdout
	}

	signalNotify(b)

	if b.Debugger == nil {
		b.Debugger = utils.NewDefaultDebugger(b.Debug, b.w)
	}

	if b.YesOrNoPrinter == nil {
		b.YesOrNoPrinter = utils.NewPrinter(b.w, utils.DefaultIdent, utils.DefaultYes, utils.DefaultNo)
	}

	return nil
}

func (b *Builder) parseOptions(v *viper.Viper, cmd *cobra.Command) error {
	allOpt, err := option.ParseOptions(b.OptionFile, v, cmd.PersistentFlags())
	if err != nil {
		return err
	}
	b.BuildOptions = allOpt.Build

	b.w = cmd.OutOrStdout()

	return nil
}

func (b *Builder) finish() {
	select {
	case <-b.done:
	default:
		close(b.done)
	}
}

func (b *Builder) waitForFinished() {
	<-b.done
}

func (b *Builder) interrupt() {
	select {
	case <-b.stop:
	default:
		close(b.stop)
	}
}

func (b *Builder) isInterrupted() bool {
	if b.stop == nil {
		return true
	}
	select {
	case <-b.stop:
		return true
	default:
		return false
	}
}

// WithManualClean if set this option, then the temporary files and the container
// will not be cleaned up automatically, and you need to clean up manually
func (b *Builder) WithManualClean() {
	b.manualClean = true
}

func (b *Builder) WithWriter(w io.Writer) {
	b.w = w
}

// CleanupForError cleans up invocation-owned temporary files and the container.
// The output destination may contain caller-owned files, so it must be preserved.
func (b *Builder) CleanupForError() {
	b.Cleanup()
}

// Cleanup cleans up the temporary files and the container
func (b *Builder) Cleanup() {
	b.removeTempDir()
	b.removeDockerEntrypoint()
	b.removeBuilderContainer()
	b.closeDockerCli()
}

func (b *Builder) removeTempDir() {
	if b.tempDir != "" {
		b.Debugf("remove temporary directory %q\n", b.tempDir)
		os.RemoveAll(b.tempDir)
	}
}

func (b *Builder) removeDockerEntrypoint() {
	if b.dockerEntrypoint != "" {
		b.Debugf("delete docker entrypoint %q\n", b.dockerEntrypoint)
		os.Remove(b.dockerEntrypoint)
	}
}

func (b *Builder) removeBuilderContainer() {
	if b.containerID != "" {
		err := b.dockerCli.ContainerRemove(context.TODO(), b.containerID, types.ContainerRemoveOptions{Force: true})
		if err != nil {
			b.Debugf("failed to remove container (%s): %s\n", b.containerConf.Name, b.containerID)
		} else {
			b.Debugf("remove container (%s): %s\n", b.containerConf.Name, b.containerID)
		}
	}
}

func (b *Builder) closeDockerCli() {
	if b.dockerCli != nil {
		b.Debugln("close the docker client")
		b.dockerCli.Close()
	}
}

func (b *Builder) builderImageRef() string {
	return fmt.Sprintf("%s:go%s-tinygo%s-oras%s", b.repository, b.Builder.Go, b.Builder.TinyGo, b.Builder.Oras)
}

func (b *Builder) SpecYAMLPath() string {
	return fmt.Sprintf("%s/spec.yaml", b.tempDir)
}

func (b *Builder) TempDir() string {
	return b.tempDir
}

func (b *Builder) String() string {
	by, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return ""
	}
	return string(by)
}
