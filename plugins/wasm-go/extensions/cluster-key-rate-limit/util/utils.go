package util

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zmap/go-iptree/iptree"
)

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
