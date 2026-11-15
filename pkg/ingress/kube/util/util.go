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

package util

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"istio.io/istio/pilot/pkg/model"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"istio.io/istio/pkg/cluster"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/alibaba/higress/v2/pkg/ingress/log"
)

const (
	DefaultDomainSuffix = "cluster.local"

	// IngressClassAnnotation is the annotation on ingress resources for the class of controllers
	// responsible for it
	IngressClassAnnotation = "kubernetes.io/ingress.class"
)

var domainSuffix = os.Getenv("DOMAIN_SUFFIX")

type ClusterNamespacedName struct {
	types.NamespacedName
	ClusterId cluster.ID
}

func (c ClusterNamespacedName) String() string {
	return c.ClusterId.String() + "/" + c.NamespacedName.String()
}

func GetDomainSuffix() string {
	if len(domainSuffix) != 0 {
		return domainSuffix
	}
	return DefaultDomainSuffix
}

func SplitNamespacedName(name string) types.NamespacedName {
	nsName := strings.Split(name, "/")
	if len(nsName) == 2 {
		return types.NamespacedName{
			Namespace: nsName[0],
			Name:      nsName[1],
		}
	}

	return types.NamespacedName{
		Name: nsName[0],
	}
}

// CreateDestinationRuleName create the same format of DR name with ops.
func CreateDestinationRuleName(istioCluster cluster.ID, namespace, name string) string {
	format := path.Join(istioCluster.String(), namespace, name)
	hash := md5.Sum([]byte(format))
	return hex.EncodeToString(hash[:])
}

func MessageToStruct(msg proto.Message) (*_struct.Struct, error) {
	if msg == nil {
		return nil, errors.New("nil message")
	}

	buf := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, msg); err != nil {
		return nil, err
	}

	pbs := &_struct.Struct{}
	if err := jsonpb.Unmarshal(buf, pbs); err != nil {
		return nil, err
	}

	return pbs, nil
}

func CreateServiceFQDN(namespace, name string) string {
	if domainSuffix == "" {
		domainSuffix = DefaultDomainSuffix
	}
	return fmt.Sprintf("%s.%s.svc.%s", name, namespace, domainSuffix)
}

// BuildPatchStruct unmarshals an Envoy patch value into a protobuf Struct.
// The error must be handled by callers: falling back to an empty Struct would
// emit a degenerate patch that silently drops the intended configuration.
func BuildPatchStruct(config string) (*_struct.Struct, error) {
	val := &_struct.Struct{}
	if err := jsonpb.Unmarshal(strings.NewReader(config), val); err != nil {
		IngressLog.Errorf("build patch struct failed, err:%v", err)
		return nil, fmt.Errorf("failed to build patch struct: %v", err)
	}
	return val, nil
}

type ServiceInfo struct {
	model.NamespacedName
	Port uint32
}

// convertToPort converts a port string to a uint32.
func convertToPort(v string) (uint32, error) {
	p, err := strconv.ParseUint(v, 10, 32)
	if err != nil || p > 65535 {
		return 0, fmt.Errorf("invalid port %s: %v", v, err)
	}
	return uint32(p), nil
}

func ParseServiceInfo(service string, ingressNamespace string) (ServiceInfo, error) {
	parts := strings.Split(service, ":")
	namespacedName := SplitNamespacedName(parts[0])

	if namespacedName.Name == "" {
		return ServiceInfo{}, errors.New("service name can not be empty")
	}

	if namespacedName.Namespace == "" {
		namespacedName.Namespace = ingressNamespace
	}

	var port uint32
	if len(parts) == 2 {
		// If port parse fail, we ignore port and pick the first one.
		port, _ = convertToPort(parts[1])
	}

	return ServiceInfo{
		NamespacedName: model.NamespacedName{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Port: port,
	}, nil
}
