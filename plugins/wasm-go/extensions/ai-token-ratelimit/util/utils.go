package util

import (
	"fmt"
	"sort"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/zmap/go-iptree/iptree"
)

const ConsumerHeader = "x-mse-consumer" // LimitByConsumer从该request header获取consumer的名字

// ParseIPNet 解析Ip段配置
func ParseIPNet(key string) (*iptree.IPTree, error) {
	tree := iptree.New()
	err := tree.AddByString(key, 0)
	if err != nil {
		return nil, fmt.Errorf("invalid IP[%s]", key)
	}
	return tree, nil
}

// ParseIP 解析IP
func ParseIP(source string) string {
	if strings.Contains(source, ".") {
		// parse ipv4
		return strings.Split(source, ":")[0]
	}
	// parse ipv6
	if strings.Contains(source, "]") {
		return strings.Split(source, "]")[0][1:]
	}
	return source
}

// ReconvertHeaders headers: map[string][]string -> [][2]string
func ReconvertHeaders(hs map[string][]string) [][2]string {
	var ret [][2]string
	for k, vs := range hs {
		for _, v := range vs {
			ret = append(ret, [2]string{k, v})
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i][0] < ret[j][0]
	})
	return ret
}

// ExtractCookieValueByKey 从cookie中提取key对应的value
func ExtractCookieValueByKey(cookie string, key string) (value string) {
	pairs := strings.Split(cookie, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 && kv[0] == key {
			value = kv[1]
			break
		}
	}
	return value
}

func GetRouteName() (string, error) {
	if raw, err := proxywasm.GetProperty([]string{"route_name"}); err != nil {
		return "-", err
	} else {
		return string(raw), nil
	}
}

func GetClusterName() (string, error) {
	if raw, err := proxywasm.GetProperty([]string{"cluster_name"}); err != nil {
		return "-", err
	} else {
		return string(raw), nil
	}
}

func GetConsumer() (string, error) {
	if consumer, err := proxywasm.GetHttpRequestHeader(ConsumerHeader); err != nil {
		return "none", err
	} else {
		return consumer, nil
	}
}
