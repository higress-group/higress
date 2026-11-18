package main

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/zmap/go-iptree/iptree"
)

const (
	DefaultRealIpHeader string = "X-Forwarded-For"
	DefaultDenyStatus   uint32 = 403
	DefaultDenyMessage  string = "Your IP address is blocked."
)
const (
	OriginSourceType = "origin-source"
	HeaderSourceType = "header"
)

type RestrictionConfig struct {
	IPSourceType string         `json:"ip_source_type"` //IP来源类型
	IPHeaderName string         `json:"ip_header_name"` //真实IP头
	Allow        *iptree.IPTree `json:"allow"`          //允许的IP
	Deny         *iptree.IPTree `json:"deny"`           //拒绝的IP
	Status       uint32         `json:"status"`         //被拒绝时返回的状态码
	Message      string         `json:"message"`        //被拒绝时返回的消息
	ResponseBody string         `json:"response_body"`  //自定义拒绝响应体，设置后覆盖默认JSON
	ContentType  string         `json:"content_type"`   //自定义Content-Type，默认 text/html
}

func main() {}

func init() {
	wrapper.SetCtx(
		"ip-restriction",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders))
}

func parseConfig(json gjson.Result, config *RestrictionConfig, log log.Log) error {
	sourceType := json.Get("ip_source_type")
	if sourceType.Exists() && sourceType.String() != "" {
		switch sourceType.String() {
		case HeaderSourceType:
			config.IPSourceType = HeaderSourceType
		case OriginSourceType:
			config.IPSourceType = OriginSourceType
		default:
			config.IPSourceType = OriginSourceType
		}
	} else {
		config.IPSourceType = OriginSourceType
	}

	header := json.Get("ip_header_name")
	if header.Exists() && header.String() != "" {
		config.IPHeaderName = header.String()
	} else {
		config.IPHeaderName = DefaultRealIpHeader
	}
	status := json.Get("status")
	if status.Exists() && status.Uint() > 1 {
		config.Status = uint32(status.Uint())
	} else {
		config.Status = DefaultDenyStatus
	}
	message := json.Get("message")
	if message.Exists() && message.String() != "" {
		config.Message = message.String()
	} else {
		config.Message = DefaultDenyMessage
	}
	config.ResponseBody = json.Get("response_body").String()
	config.ContentType = json.Get("content_type").String()
	allowNets, err := parseIPNets(json.Get("allow").Array())
	if err != nil {
		log.Error(err.Error())
		return err
	}
	denyNets, err := parseIPNets(json.Get("deny").Array())
	if err != nil {
		log.Error(err.Error())
		return err
	}
	if allowNets != nil && denyNets != nil {
		log.Warn("allow and deny cannot be set at the same time")
		return fmt.Errorf("allow and deny cannot be set at the same time")
	}
	if allowNets == nil && denyNets == nil {
		log.Warn("allow and deny cannot be empty at the same time")
		return fmt.Errorf("allow and deny cannot be empty at the same time")
	}
	config.Allow = allowNets
	config.Deny = denyNets
	return nil
}

func getDownStreamIp(config RestrictionConfig) (net.IP, error) {
	var (
		s   string
		err error
	)

	if config.IPSourceType == HeaderSourceType {
		s, err = proxywasm.GetHttpRequestHeader(config.IPHeaderName)
	} else {
		var bs []byte
		bs, err = proxywasm.GetProperty([]string{"source", "address"})
		s = string(bs)
	}
	if err != nil {
		return nil, err
	}
	ip := parseIP(s, config.IPSourceType == HeaderSourceType)
	realIP := net.ParseIP(ip)
	if realIP == nil {
		return nil, fmt.Errorf("invalid ip[%s]", ip)
	}
	return realIP, nil
}

func onHttpRequestHeaders(context wrapper.HttpContext, config RestrictionConfig, log log.Log) types.Action {
	realIp, err := getDownStreamIp(config)
	if err != nil {
		return deniedUnauthorized(config, "get_ip_failed")
	}
	allow := config.Allow
	deny := config.Deny
	if allow != nil {
		if realIp == nil {
			log.Error("realIp is nil, blocked")
			return deniedUnauthorized(config, "empty_ip")
		}
		if _, found, _ := allow.Get(realIp); !found {
			return deniedUnauthorized(config, "ip_not_allowed")
		}
	}
	if deny != nil {
		if realIp == nil {
			log.Error("realIp is nil, continue")
			return types.ActionContinue
		}
		if _, found, _ := deny.Get(realIp); found {
			return deniedUnauthorized(config, "ip_denied")
		}
	}
	return types.ActionContinue
}

func deniedUnauthorized(config RestrictionConfig, reason string) types.Action {
	var body []byte
	var headers [][2]string
	if config.ResponseBody != "" {
		body = []byte(config.ResponseBody)
		contentType := config.ContentType
		if contentType == "" {
			contentType = "text/html; charset=utf-8"
		}
		headers = [][2]string{{"content-type", contentType}}
	} else {
		body, _ = json.Marshal(map[string]string{
			"message": config.Message,
		})
	}
	_ = proxywasm.SendHttpResponseWithDetail(config.Status, "ip-restriction."+reason, headers, body, -1)
	return types.ActionContinue
}
