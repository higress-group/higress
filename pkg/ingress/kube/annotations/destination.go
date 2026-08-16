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

package annotations

import (
	"bufio"
	"strconv"
	"strings"

	networking "istio.io/api/networking/v1alpha3"

	. "github.com/alibaba/higress/v2/pkg/ingress/log"
)

const (
	destinationKey = "destination"
)

var _ Parser = destination{}

type DestinationConfig struct {
	McpDestination []*networking.HTTPRouteDestination
	WeightSum      int64
	Protocols      map[string]string
}

type destination struct{}

func (a destination) Parse(annotations Annotations, config *Ingress, _ *GlobalContext) error {
	if !needDestinationConfig(annotations) {
		return nil
	}
	value, err := annotations.ParseStringForHigress(destinationKey)
	if err != nil {
		IngressLog.Errorf("parse destination error %v within ingress %s/%s", err, config.Namespace, config.Name)
		return nil
	}
	lines := splitLines(value)
	var destinations []*networking.HTTPRouteDestination
	protocols := map[string]string{}
	var weightSum int64
	for _, line := range lines {
		// fmt: [weight] <host>[:port] [subset]
		// example: 100% my-svc.DEFAULT-GROUP.xxxx.nacos:8080 v1
		pairs := strings.Fields(line)
		var weight int64 = 100
		var addrIndex int
		if len(pairs) == 0 {
			continue
		}
		if strings.HasSuffix(pairs[0], "%") {
			weight, err = strconv.ParseInt(strings.TrimSuffix(pairs[0], "%"), 10, 32)
			if err != nil {
				IngressLog.Errorf("parse destination atoi error %v within ingress %s/%s", err, config.Namespace, config.Name)
				return nil
			}
			addrIndex++
		}
		weightSum += weight
		if len(pairs) < addrIndex+1 {
			IngressLog.Errorf("destination %s has no address within ingress %s/%s", value, config.Namespace, config.Name)
			return nil
		}
		address, protocol := splitDestinationProtocol(pairs[addrIndex])
		host := address
		var port uint64
		colon := strings.LastIndex(address, ":")
		if colon != -1 {
			var err error
			port, err = strconv.ParseUint(address[colon+1:], 10, 32)
			if err == nil && port > 0 && port < 65536 {
				host = address[:colon]
			}
		}
		var subset string
		if len(pairs) >= addrIndex+2 {
			subset = pairs[addrIndex+1]
		}
		dest := &networking.HTTPRouteDestination{
			Destination: &networking.Destination{
				Host:   host,
				Subset: subset,
			},
			Weight: int32(weight),
		}
		if port > 0 {
			dest.Destination.Port = &networking.PortSelector{
				Number: uint32(port),
			}
		}
		if protocol != "" {
			protocols[destinationProtocolKey(host, uint32(port))] = protocol
		}
		IngressLog.Debugf("destination generated for ingress %s/%s: %v", config.Namespace, config.Name, dest)
		destinations = append(destinations, dest)
	}
	if weightSum != 100 {
		IngressLog.Warnf("destination has invalid weight sum %d within ingress %s/%s", weightSum, config.Namespace, config.Name)
	}
	config.Destination = &DestinationConfig{
		McpDestination: destinations,
		WeightSum:      weightSum,
	}
	if len(protocols) > 0 {
		config.Destination.Protocols = protocols
	}
	return nil
}

func (d *DestinationConfig) BackendProtocolForDestination(destination *networking.Destination) string {
	if d == nil || destination == nil {
		return ""
	}
	return d.Protocols[destinationProtocolKey(destination.Host, destination.GetPort().GetNumber())]
}

func (d *DestinationConfig) HasBackendProtocols() bool {
	return d != nil && len(d.Protocols) > 0
}

func needDestinationConfig(annotations Annotations) bool {
	return annotations.HasHigress(destinationKey)
}

func splitLines(s string) []string {
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func splitDestinationProtocol(address string) (string, string) {
	lowerAddress := strings.ToLower(address)
	if strings.HasPrefix(lowerAddress, "http://") {
		return address[len("http://"):], "HTTP"
	}
	if strings.HasPrefix(lowerAddress, "https://") {
		return address[len("https://"):], "HTTPS"
	}
	return address, ""
}

func destinationProtocolKey(host string, port uint32) string {
	return host + ":" + strconv.FormatUint(uint64(port), 10)
}
