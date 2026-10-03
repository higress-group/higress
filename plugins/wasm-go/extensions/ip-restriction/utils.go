package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/asergeyev/nradix"
	"github.com/tidwall/gjson"
	"github.com/zmap/go-iptree/iptree"

	"github.com/higress-group/wasm-go/pkg/log"
)

// parseIPNets 解析Ip段配置
func parseIPNets(array []gjson.Result) (*iptree.IPTree, error) {
	if len(array) == 0 {
		return nil, nil
	} else {
		tree := iptree.New()
		for _, result := range array {
			err := tree.AddByString(result.String(), 0)
			if err != nil {
				if errors.Is(err, nradix.ErrNodeBusy) {
					// ErrNodeBusy means the IP already exists in the tree
					log.Warnf("ignore duplicate IP [%s]", result.String())
				} else {
					return nil, fmt.Errorf("add IP [%s] into tree failed: %v", result.String(), err)
				}
			}
		}
		return tree, nil
	}
}

// parseIP 解析IP
func parseIP(source string, fromHeader bool) string {

	if fromHeader {
		source = strings.Split(source, ",")[0]
	}
	source = strings.Trim(source, " ")
	if strings.Contains(source, ".") {
		// parse ipv4
		return strings.Split(source, ":")[0]
	}
	//parse ipv6
	if strings.Contains(source, "]") {
		// strings.Split always yields at least one element, but a value such as
		// "]" or "]:80" leaves an empty first element, so the leading bracket
		// must be removed with TrimPrefix instead of slicing.
		return strings.TrimPrefix(strings.Split(source, "]")[0], "[")
	}
	return source
}
