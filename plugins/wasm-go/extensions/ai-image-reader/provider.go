package main

import (
	"errors"
	"github.com/tidwall/gjson"
)

const (
	ProviderTypeDashscope = "dashscope"
	ProviderTypeLocal     = "local"
)

type providerInitializer interface {
	InitConfig(json gjson.Result)
	ValidateConfig() error
	CreateProvider(ProviderConfig) (Provider, error)
}

var (
	providerInitializers = map[string]providerInitializer{
		ProviderTypeDashscope: &dashScopeProviderInitializer{},
		ProviderTypeLocal:     &localProviderInitializer{},
	}
)

type ProviderConfig struct {
	// @Title zh-CN 文字识别服务提供者类型
	// @Description zh-CN 文字识别服务提供者类型，例如 DashScope
	typ string
	// @Title zh-CN DashScope 文字识别服务名称
	// @Description zh-CN 文字识别服务名称
	serviceName string
	// @Title zh-CN 文字识别服务域名
	// @Description zh-CN 文字识别服务域名
	serviceHost string
	// @Title zh-CN 文字识别服务端口
	// @Description zh-CN 文字识别服务端口
	servicePort int64
	// @Title zh-CN 文字识别服务超时时间
	// @Description zh-CN 文字识别服务超时时间
	timeout uint32
	// @Title zh-CN 文字识别服务使用的模型
	// @Description zh-CN 用于文字识别的模型名称, 在 DashScope 中默认为 "qwen-vl-ocr"
	model string

	initializer providerInitializer
}

func (c *ProviderConfig) FromJson(json gjson.Result) {
	c.typ = json.Get("type").String()
	i, has := providerInitializers[c.typ]
	if has {
		i.InitConfig(json)
		c.initializer = i
	}
	c.serviceName = json.Get("serviceName").String()
	c.serviceHost = json.Get("serviceHost").String()
	c.servicePort = json.Get("servicePort").Int()
	c.timeout = uint32(json.Get("timeout").Int())
	c.model = json.Get("model").String()
	if c.timeout == 0 {
		c.timeout = 10000
	}
}

func (c *ProviderConfig) Validate() error {
	if c.typ == "" {
		return errors.New("ocr service provider type is required")
	}
	if c.serviceName == "" {
		return errors.New("ocr service name is required")
	}
	if c.typ == "" {
		return errors.New("ocr service type is required")
	}
	if c.initializer == nil {
		return errors.New("unknown ocr service provider type: " + c.typ)
	}
	if err := c.initializer.ValidateConfig(); err != nil {
		return err
	}
	return nil
}

func (c *ProviderConfig) GetProviderType() string {
	return c.typ
}

func CreateProvider(pc ProviderConfig) (Provider, error) {
	initializer, has := providerInitializers[pc.typ]
	if !has {
		return nil, errors.New("unknown provider type: " + pc.typ)
	}
	return initializer.CreateProvider(pc)
}

type CallArgs struct {
	Method             string
	Url                string
	Headers            [][2]string
	Body               []byte
	TimeoutMillisecond uint32
}

type Provider interface {
	GetProviderType() string
	CallArgs(imageUrl string) CallArgs
	DoOCR(
		imageUrl string,
		callback func(imageContent string, err error)) error
}
