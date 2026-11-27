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

package configmap

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"
)

const (
	higressGzipEnvoyFilterName = "higress-config-gzip"
	compressionStrategyValues  = "DEFAULT_STRATEGY,FILTERED,HUFFMAN_ONLY,RLE,FIXED"
	compressionLevelValues     = "BEST_COMPRESSION,BEST_SPEED,COMPRESSION_LEVEL_1,COMPRESSION_LEVEL_2,COMPRESSION_LEVEL_3,COMPRESSION_LEVEL_4,COMPRESSION_LEVEL_5,COMPRESSION_LEVEL_6,COMPRESSION_LEVEL_7,COMPRESSION_LEVEL_8,COMPRESSION_LEVEL_9"
)

type Gzip struct {
	// Flag to control gzip
	Enable              bool     `json:"enable,omitempty"`
	MinContentLength    int32    `json:"minContentLength,omitempty"`
	ContentType         []string `json:"contentType,omitempty"`
	DisableOnEtagHeader bool     `json:"disableOnEtagHeader,omitempty"`
	// Value from 1 to 9 that controls the amount of internal memory used by zlib.
	// Higher values use more memory, but are faster and produce better compression results. The default value is 5.
	MemoryLevel int32 `json:"memoryLevel,omitempty"`
	//  Value from 9 to 15 that represents the base two logarithmic of the compressor’s window size.
	//  Larger window results in better compression at the expense of memory usage.
	//  The default is 12 which will produce a 4096 bytes window
	WindowBits int32 `json:"windowBits,omitempty"`
	// Value for Zlib’s next output buffer. If not set, defaults to 4096.
	ChunkSize int32 `json:"chunkSize,omitempty"`
	// A value used for selecting the zlib compression level.
	// From COMPRESSION_LEVEL_1 to COMPRESSION_LEVEL_9
	// BEST_COMPRESSION == COMPRESSION_LEVEL_9 , BEST_SPEED == COMPRESSION_LEVEL_1
	CompressionLevel string `json:"compressionLevel,omitempty"`
	// A value used for selecting the zlib compression strategy which is directly related to the characteristics of the content.
	// Most of the time “DEFAULT_STRATEGY”
	// Value is one of DEFAULT_STRATEGY, FILTERED, HUFFMAN_ONLY, RLE, FIXED
	CompressionStrategy string `json:"compressionStrategy,omitempty"`
}

func validGzip(g *Gzip) error {
	if g == nil {
		return nil
	}

	if g.MinContentLength <= 0 {
		return errors.New("minContentLength can not be less than zero")
	}

	if len(g.ContentType) == 0 {
		return errors.New("content type can not be empty")
	}

	if !(g.MemoryLevel >= 1 && g.MemoryLevel <= 9) {
		return errors.New("memory level need be between 1 and 9")
	}

	if !(g.WindowBits >= 9 && g.WindowBits <= 15) {
		return errors.New("window bits need be between 9 and 15")
	}

	if g.ChunkSize <= 0 {
		return errors.New("chunk size need be large than zero")
	}

	compressionLevels := strings.Split(compressionLevelValues, ",")
	isFound := false
	for _, v := range compressionLevels {
		if g.CompressionLevel == v {
			isFound = true
			break
		}
	}
	if !isFound {
		return fmt.Errorf("compressionLevel need be one of %s", compressionLevelValues)
	}

	isFound = false
	compressionStrategies := strings.Split(compressionStrategyValues, ",")
	for _, v := range compressionStrategies {
		if g.CompressionStrategy == v {
			isFound = true
			break
		}
	}
	if !isFound {
		return fmt.Errorf("compressionStrategy need be one of %s", compressionStrategyValues)
	}

	return nil
}

func compareGzip(old *Gzip, new *Gzip) (Result, error) {
	if old == nil && new == nil {
		return ResultNothing, nil
	}

	if new == nil {
		return ResultDelete, nil
	}

	if !reflect.DeepEqual(old, new) {
		return ResultReplace, nil
	}

	return ResultNothing, nil
}

func deepCopyGzip(gzip *Gzip) (*Gzip, error) {
	newGzip := NewDefaultGzip()
	newGzip.Enable = gzip.Enable
	newGzip.MinContentLength = gzip.MinContentLength
	newGzip.ContentType = make([]string, 0, len(gzip.ContentType))
	newGzip.ContentType = append(newGzip.ContentType, gzip.ContentType...)
	newGzip.DisableOnEtagHeader = gzip.DisableOnEtagHeader
	newGzip.MemoryLevel = gzip.MemoryLevel
	newGzip.WindowBits = gzip.WindowBits
	newGzip.ChunkSize = gzip.ChunkSize
	newGzip.CompressionLevel = gzip.CompressionLevel
	newGzip.CompressionStrategy = gzip.CompressionStrategy
	return newGzip, nil
}

func NewDefaultGzip() *Gzip {
	gzip := &Gzip{
		Enable:              true,
		MinContentLength:    1024,
		ContentType:         []string{"text/html", "text/css", "text/plain", "text/xml", "application/json", "application/javascript", "application/xhtml+xml", "image/svg+xml"},
		DisableOnEtagHeader: true,
		MemoryLevel:         5,
		WindowBits:          12,
		ChunkSize:           4096,
		CompressionLevel:    "BEST_COMPRESSION",
		CompressionStrategy: "DEFAULT_STRATEGY",
	}
	return gzip
}

type GzipController struct {
	Namespace    string
	gzip         atomic.Value
	Name         string
	eventHandler ItemEventHandler
}

func NewGzipController(namespace string) *GzipController {
	gzipController := &GzipController{
		Namespace: namespace,
		gzip:      atomic.Value{},
		Name:      "gzip",
	}
	gzipController.SetGzip(NewDefaultGzip())
	return gzipController
}

func (g *GzipController) GetName() string {
	return g.Name
}

func (t *GzipController) SetGzip(gzip *Gzip) {
	t.gzip.Store(gzip)
}

func (g *GzipController) GetGzip() *Gzip {
	value := g.gzip.Load()
	if value != nil {
		if gzip, ok := value.(*Gzip); ok {
			return gzip
		}
	}
	return nil
}

func (g *GzipController) AddOrUpdateHigressConfig(name util.ClusterNamespacedName, old *HigressConfig, new *HigressConfig) error {
	if err := validGzip(new.Gzip); err != nil {
		IngressLog.Errorf("data:%+v convert to gzip , error: %+v", new.Gzip, err)
		return nil
	}

	result, _ := compareGzip(old.Gzip, new.Gzip)

	switch result {
	case ResultReplace:
		if newGzip, err := deepCopyGzip(new.Gzip); err != nil {
			IngressLog.Infof("gzip deepcopy error:%v", err)
		} else {
			g.SetGzip(newGzip)
			IngressLog.Infof("AddOrUpdate Higress config gzip")
			g.eventHandler(higressGzipEnvoyFilterName)
			IngressLog.Infof("send event with filter name:%s", higressGzipEnvoyFilterName)
		}
	case ResultDelete:
		g.SetGzip(NewDefaultGzip())
		IngressLog.Infof("Delete Higress config gzip")
		g.eventHandler(higressGzipEnvoyFilterName)
		IngressLog.Infof("send event with filter name:%s", higressGzipEnvoyFilterName)
	}

	return nil
}

func (g *GzipController) ValidHigressConfig(higressConfig *HigressConfig) error {
	if higressConfig == nil {
		return nil
	}
	if higressConfig.Gzip == nil {
		return nil
	}

	return validGzip(higressConfig.Gzip)
}

func (g *GzipController) ConstructEnvoyFilters() ([]*config.Config, error) {
	configs := make([]*config.Config, 0)
	gzip := g.GetGzip()
	namespace := g.Namespace

	if gzip == nil {
		return configs, nil
	}

	if gzip.Enable == false {
		return configs, nil
	}

	gzipStruct := g.constructGzipStruct(gzip, namespace)
	if len(gzipStruct) == 0 {
		return configs, nil
	}

	gzipPatchValue, err := util.BuildPatchStruct(gzipStruct)
	if err != nil {
		return nil, err
	}

	config := &config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.EnvoyFilter,
			Name:             higressGzipEnvoyFilterName,
			Namespace:        namespace,
		},
		Spec: &networking.EnvoyFilter{
			ConfigPatches: []*networking.EnvoyFilter_EnvoyConfigObjectPatch{
				{
					ApplyTo: networking.EnvoyFilter_HTTP_FILTER,
					Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
						Context: networking.EnvoyFilter_GATEWAY,
						ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
							Listener: &networking.EnvoyFilter_ListenerMatch{
								FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
									Filter: &networking.EnvoyFilter_ListenerMatch_FilterMatch{
										Name: "envoy.filters.network.http_connection_manager",
										SubFilter: &networking.EnvoyFilter_ListenerMatch_SubFilterMatch{
											Name: "envoy.filters.http.cors",
										},
									},
								},
							},
						},
					},
					Patch: &networking.EnvoyFilter_Patch{
						Operation: networking.EnvoyFilter_Patch_INSERT_BEFORE,
						Value:     gzipPatchValue,
					},
				},
			},
		},
	}

	configs = append(configs, config)
	return configs, nil
}

func (g *GzipController) RegisterItemEventHandler(eventHandler ItemEventHandler) {
	g.eventHandler = eventHandler
}

func (g *GzipController) constructGzipStruct(gzip *Gzip, namespace string) string {
	gzipConfig := ""
	contentType := ""
	index := 0
	for _, v := range gzip.ContentType {
		contentType = contentType + fmt.Sprintf("\"%s\"", v)
		if index < len(gzip.ContentType)-1 {
			contentType = contentType + ","
		}
		index++
	}
	structFmt := `{
   "name": "envoy.filters.http.compressor",
   "typed_config": {
      "@type": "type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor",
      "response_direction_config": {
         "common_config": {
            "min_content_length": %d,
            "content_type": [%s]
         },
        "disable_on_etag_header": %t
      },
      "request_direction_config": {
         "common_config": {
            "enabled": {
               "default_value": false,
               "runtime_key": "request_compressor_enabled"
            }
         }
      },
      "compressor_library": {
         "name": "text_optimized",
         "typed_config": {
            "@type": "type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip",
            "memory_level": %d,
            "window_bits": %d,
            "check_size": %d,
            "compression_level": "%s",
            "compression_strategy": "%s"
         }
      }
   }
}`
	gzipConfig = fmt.Sprintf(structFmt, gzip.MinContentLength, contentType, gzip.DisableOnEtagHeader,
		gzip.MemoryLevel, gzip.WindowBits, gzip.ChunkSize, gzip.CompressionLevel, gzip.CompressionStrategy)
	return gzipConfig
}
