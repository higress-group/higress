package util

import (
	"fmt"
	"net"
	"net/netip"
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

// ParseIP parses a client IP from a value that may be a bare address, an
// address with a port, or a bracketed IPv6 address with an optional port. It
// accepts IPv4, IPv6 and IPv4-mapped IPv6 (for example "::ffff:192.0.2.1"),
// strips brackets, port and IPv6 zone, and returns the canonical address.
// Invalid or malformed values return an empty string so callers can fail
// closed instead of silently skipping the rate-limit rule.
func ParseIP(source string) string {
	addr, ok := parseIP(source)
	if !ok {
		return ""
	}
	return addr.String()
}

func parseIP(source string) (netip.Addr, bool) {
	s := strings.TrimSpace(source)
	if s == "" {
		return netip.Addr{}, false
	}

	// Bare IPv4, IPv6, or IPv4-mapped IPv6. Parsing the whole value first
	// avoids the old "contains a dot means IPv4, split on colon" heuristic,
	// which turned "::ffff:192.0.2.1" into an empty string.
	if addr, err := netip.ParseAddr(s); err == nil {
		return normalizeAddr(addr), true
	}

	// Bracketed IPv6 without a port, for example "[2001:db8::1]".
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		if addr, err := netip.ParseAddr(s[1 : len(s)-1]); err == nil {
			return normalizeAddr(addr), true
		}
		return netip.Addr{}, false
	}

	// Address with a port, for example "192.0.2.1:8080" or
	// "[2001:db8::1]:443". The port text is irrelevant to rate limiting, so
	// only the host part is validated; this preserves the previous tolerance
	// for values such as "[::1]:99999".
	if host, _, err := net.SplitHostPort(s); err == nil {
		if addr, err := netip.ParseAddr(host); err == nil {
			return normalizeAddr(addr), true
		}
	}
	return netip.Addr{}, false
}

// normalizeAddr removes an IPv6 zone and unwraps IPv4-mapped IPv6 addresses so
// that the same client always maps to the same rate-limit key and IPv4 CIDR.
func normalizeAddr(addr netip.Addr) netip.Addr {
	if addr.Zone() != "" {
		addr = addr.WithZone("")
	}
	return addr.Unmap()
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
