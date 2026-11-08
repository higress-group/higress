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

package install

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	k8s "github.com/alibaba/higress/hgctl/pkg/kubernetes"
	"github.com/alibaba/higress/hgctl/pkg/plugin/build"
	"github.com/alibaba/higress/hgctl/pkg/plugin/config"
	"github.com/alibaba/higress/hgctl/pkg/plugin/option"
	"github.com/alibaba/higress/hgctl/pkg/plugin/types"
	"github.com/alibaba/higress/hgctl/pkg/plugin/utils"
	"github.com/alibaba/higress/v2/pkg/cmd/options"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/pkg/errors"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type installer struct {
	optionFile string
	bldOpts    option.BuildOptions
	insOpts    option.InstallOptions

	cli *k8s.WasmPluginClient
	w   io.Writer
	utils.Debugger
}

func NewCommand() *cobra.Command {
	var ins installer
	v := viper.New()

	installCmd := &cobra.Command{
		Use:     "install",
		Aliases: []string{"ins", "i"},
		Short:   "Install WASM plugin",
		Example: `  # Install WASM plugin using a WasmPlugin manifest
  hgctl plugin install -y plugin-conf.yaml

  # Install WASM plugin through the Golang WASM plugin project (do it by relying on option.yaml now)
  docker login
  hgctl plugin install -g ./
  `,

		PreRun: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ins.config(v, cmd))
		},

		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ins.install(cmd.PersistentFlags()))
		},
	}

	flags := installCmd.PersistentFlags()
	options.AddKubeConfigFlags(flags)
	option.AddOptionFileFlag(&ins.optionFile, flags)
	v.BindPFlags(flags)

	flags.StringP("namespace", "n", k8s.HigressNamespace, "Namespace where Higress was installed")
	v.BindPFlag("install.namespace", flags.Lookup("namespace"))
	v.SetDefault("install.namespace", k8s.DefaultHigressNamespace)

	flags.StringP("spec-yaml", "s", "./out/spec.yaml", "Use to validate WASM plugin configuration")
	v.BindPFlag("install.spec-yaml", flags.Lookup("spec-yaml"))
	v.SetDefault("install.spec-yaml", "./test/plugin-spec-yaml")

	// TODO(WeixinX):
	// - Change "--from-yaml (-y)" to "--from-oci (-o)" and implement command line interaction like "--from-go-src"
	// - Add "--from-jar (-j)"
	flags.StringP("from-yaml", "y", "./test/plugin-conf.yaml", "Install WASM plugin using a WasmPlugin manifest")
	v.BindPFlag("install.from-yaml", flags.Lookup("from-yaml"))
	v.SetDefault("install.from-yaml", "./test/plugin-conf.yaml")

	flags.StringP("from-go-src", "g", "", "Install WASM plugin through the Golang WASM plugin project")
	v.BindPFlag("install.from-go-src", flags.Lookup("from-go-src"))
	v.SetDefault("install.from-go-src", "")

	flags.BoolP("debug", "", false, "Enable debug mode")
	v.BindPFlag("install.debug", flags.Lookup("debug"))
	v.SetDefault("install.debug", false)

	return installCmd
}

func (ins *installer) config(v *viper.Viper, cmd *cobra.Command) error {
	allOpt, err := option.ParseOptions(ins.optionFile, v, cmd.PersistentFlags())
	if err != nil {
		return err
	}
	// TODO(WeixinX): Avoid relying on build options, add a new option "--push/--image" for installing from go src
	ins.bldOpts = allOpt.Build
	ins.insOpts = allOpt.Install

	dynCli, err := k8s.NewDynamicClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
	if err != nil {
		return errors.Wrap(err, "failed to build kubernetes dynamic client")
	}
	ins.cli = k8s.NewWasmPluginClient(dynCli)
	ins.w = cmd.OutOrStdout()
	ins.Debugger = utils.NewDefaultDebugger(ins.insOpts.Debug, ins.w)

	return nil
}

func (ins *installer) install(flags *pflag.FlagSet) (err error) {
	ins.Debugf("install option:\n%s\n", ins.String())

	if ins.insOpts.FromGoSrc == "" || flags.Changed("from-yaml") {
		err = ins.yamlHandler()
	} else {
		err = ins.goHandler()
	}
	return
}

func (ins *installer) yamlHandler() error {
	return ins.doInstall(true)
}

func (ins *installer) goHandler() error {
	// 0. ensure output.type == image
	if ins.bldOpts.Output.Type != "image" {
		return errors.New("output type must be image")
	}

	// 1. build the WASM plugin project and push the image to the registry
	bld, err := build.NewBuilder(func(b *build.Builder) error {
		b.BuildOptions = ins.bldOpts
		b.Debug = ins.insOpts.Debug
		b.WithManualClean() // keep spec.yaml
		b.WithWriter(ins.w)
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to initialize builder")
	}
	err = bld.Build()
	if err != nil {
		bld.Debugln("clean up for error ...")
		bld.CleanupForError()
		return errors.Wrap(err, "failed to build and push wasm plugin")
	}
	defer bld.Cleanup()

	// 2. command-line interaction lets the user enter the wasm plugin configuration
	specPath := bld.SpecYAMLPath()
	spec, err := types.ParseSpecYAML(specPath)
	if err != nil {
		return errors.Wrapf(err, "failed to parse spec.yaml: %s", specPath)
	}
	vld, err := buildSchemaValidator(spec)
	if err != nil {
		return err
	}

	example := spec.GetConfigExample()
	schema := spec.Spec.ConfigSchema.OpenAPIV3Schema
	printer := utils.DefaultPrinter()
	asker := NewWasmPluginSpecConfAsker(
		NewIngressAsker(bld.Model, schema, vld, printer),
		NewDomainAsker(bld.Model, schema, vld, printer),
		NewGlobalConfAsker(bld.Model, schema, vld, printer),
		printer,
	)

	printer.Yesln("Please enter the configurations for the WASM plugin you want to install:")
	printer.Yesln("Configuration example:")
	printer.Yesf("\n%s\n", example)

	err = asker.Ask()
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			printer.Noln(askInterrupted)
			return nil
		}
		panic(err)
	}

	// 3. generate the WasmPlugin manifest
	wpc := asker.resp
	if err != nil {
		return errors.Wrap(err, "failed to marshal wasm plugin config")
	}
	// get the parameters of plugin-conf.yaml from spec.yaml
	pc, err := config.ExtractPluginConfFrom(spec, wpc.String(), bld.Output.Dest)
	if err != nil {
		return errors.Wrapf(err, "failed to get the parameters of plugin-conf.yaml from %s", specPath)
	}
	ins.Debugf("plugin-conf.yaml params:\n%s\n", pc.String())
	if err = config.GenPluginConfYAML(pc, bld.TempDir()); err != nil {
		return errors.Wrap(err, "failed to generate plugin-conf.yaml")
	}

	// 4. install by the manifest
	ins.insOpts.FromYaml = bld.TempDir() + "/plugin-conf.yaml"
	if err = ins.doInstall(false); err != nil {
		return err
	}
	return nil
}

func (ins *installer) doInstall(validate bool) error {
	f, err := os.Open(ins.insOpts.FromYaml)
	if err != nil {
		return err
	}
	defer f.Close()

	// multiple WASM plugins are separated by '---' in yaml, but we only handle first one
	// TODO(WeixinX): Use WasmPlugin Object type instead of Unstructured
	obj := &unstructured.Unstructured{}
	dc := k8syaml.NewYAMLOrJSONDecoder(f, 4096)
	if err = dc.Decode(obj); err != nil {
		return errors.Wrapf(err, "failed to parse wasm plugin from manifest %q", ins.insOpts.FromYaml)
	}

	if !isValidAPIVersion(obj) {
		fmt.Fprintf(ins.w, "Warning: wasm plugin %q has invalid apiVersion, automatically modified: %q -> %q\n",
			obj.GetName(), obj.GetAPIVersion(), k8s.HigressExtAPIVersion)
		obj.SetAPIVersion(k8s.HigressExtAPIVersion)
	}
	if !isValidKind(obj) {
		fmt.Fprintf(ins.w, "Warning: wasm plugin %q has invalid kind, automatically modified: %q -> %q\n",
			obj.GetName(), obj.GetKind(), k8s.WasmPluginKind)
		obj.SetKind(k8s.WasmPluginKind)
	}
	namespace := ins.insOpts.Namespace
	if namespace == "" {
		namespace = k8s.HigressNamespace
	}
	if obj.GetNamespace() != namespace {
		fmt.Fprintf(ins.w, "Warning: wasm plugin %q has invalid namespace, automatically modified: %q -> %q\n",
			obj.GetName(), obj.GetNamespace(), namespace)
		obj.SetNamespace(namespace)
	}

	// validate wasm plugin config
	if validate {
		if wps, ok := obj.Object["spec"].(map[string]interface{}); ok {
			if err = ins.validateWasmPluginConfig(wps); err != nil {
				return err
			}
		} else {
			return errors.New("failed to get the spec filed of wasm plugin")
		}
		ins.Debugln("successfully validated wasm plugin config")
	}

	result, err := ins.cli.CreateInNamespace(context.TODO(), namespace, obj)
	if err != nil {
		if k8serr.IsAlreadyExists(err) {
			fmt.Fprintf(ins.w, "wasm plugin %q already exists\n",
				fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()))
			return nil
		}
		return errors.Wrapf(err, "failed to install wasm plugin %q",
			fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()))
	}

	fmt.Fprintf(ins.w, "Installed wasm plugin %q\n", fmt.Sprintf("%s/%s", result.GetNamespace(), result.GetName()))

	return nil
}

func isValidAPIVersion(obj *unstructured.Unstructured) bool {
	return obj.GetAPIVersion() == k8s.HigressExtAPIVersion
}

func isValidKind(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == k8s.WasmPluginKind
}

func (ins *installer) validateWasmPluginConfig(wps map[string]interface{}) error {
	spec, err := types.ParseSpecYAML(ins.insOpts.SpecYaml)
	if err != nil {
		return errors.Wrapf(err, "failed to parse %s", ins.insOpts.SpecYaml)
	}
	vld, err := buildSchemaValidator(spec)
	if err != nil {
		return errors.Wrapf(err, "failed to build schema validator")
	}

	if dc, ok := wps["defaultConfig"].(map[string]interface{}); ok {
		if ok, err = validate(vld, dc); !ok {
			return errors.Wrap(err, "failed to validate default config")
		}

		// debug
		b, _ := utils.MarshalYamlWithIndent(dc, 2)
		ins.Debugf("default config:\n%s\n", string(b))
	}

	if mrs, ok := wps["matchRules"].([]interface{}); ok {
		for _, mr := range mrs {
			if r, ok := mr.(map[string]interface{}); ok {
				if _, ok = r["ingress"]; ok {
					ing, err := decodeIngressMatchRule(r)
					if err != nil {
						return errors.Wrap(err, "failed to parse ingress match rule")
					}
					if ok, err = validate(vld, ing.Config); !ok {
						return errors.Wrap(err, "failed to validate ingress match rule")
					}

					ins.Debugf("ingress match rule:\n%s\n", ing.String())

				} else if _, ok = r["domain"]; ok {
					dom, err := decodeDomainMatchRule(r)
					if err != nil {
						return errors.Wrap(err, "failed to parse domain match rule")
					}
					if ok, err = validate(vld, dom.Config); !ok {
						return errors.Wrap(err, "failed to validate ingress match rule")
					}

					ins.Debugf("domain match rule:\n%s\n", dom.String())
				}
			}
		}
	}

	return nil
}

func buildSchemaValidator(spec *types.WasmPluginMeta) (*jsonschema.Schema, error) {
	if spec == nil {
		return nil, errors.New("spec is nil")
	}

	schema := spec.Spec.ConfigSchema.OpenAPIV3Schema
	if schema == nil {
		return nil, errors.New("spec has no config schema")
	}

	b, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}

	c := jsonschema.NewCompiler()
	c.Draft = jsonschema.Draft4
	err = c.AddResource("schema.json", strings.NewReader(string(b)))
	vld, err := c.Compile("schema.json")
	if err != nil {
		errors.Wrap(err, "failed to compile schema")
	}

	return vld, nil
}

func (ins *installer) String() string {
	b, err := json.MarshalIndent(ins.insOpts, "", "  ")
	if err != nil {
		return ""
	}
	return fmt.Sprintf("OptionFile: %s\n%s", ins.optionFile, string(b))
}
