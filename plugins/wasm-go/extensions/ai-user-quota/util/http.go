// Copyright (c) 2024-2025
// Beijing Baolande Software Corporation
// All Rights Reserved

package util

import (
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
)

const (
	HeaderContentType = "Content-Type"

	MimeTypeTextPlain       = "text/plain"
	MimeTypeApplicationJson = "application/json"
)

// SendResponse 直接产出一个本地响应,无需再 ResumeHttpRequest
func SendResponse(statusCode uint32, statusCodeDetails string, contentType, body string) error {
	return proxywasm.SendHttpResponseWithDetail(statusCode, statusCodeDetails, CreateHeaders(HeaderContentType, contentType), []byte(body), -1)
}

func CreateHeaders(kvs ...string) [][2]string {
	if len(kvs)%2 != 0 {
		kvs = kvs[:len(kvs)-1]
	}
	headers := make([][2]string, 0, len(kvs)/2)
	for i := 0; i < len(kvs); i += 2 {
		headers = append(headers, [2]string{kvs[i], kvs[i+1]})
	}
	return headers
}

// GetRouteName 取当前 envoy 路由名,Ingress 派生路由带 "{namespace}/" 前缀
func GetRouteName() (string, error) {
	raw, err := proxywasm.GetProperty([]string{"route_name"})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// TrimNamespacePrefix 剥离路由名的 "{namespace}/" 前缀
func TrimNamespacePrefix(routeName string) string {
	if idx := strings.LastIndex(routeName, "/"); idx >= 0 {
		return routeName[idx+1:]
	}
	return routeName
}