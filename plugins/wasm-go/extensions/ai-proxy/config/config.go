package config

import (
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/provider"
	"github.com/tidwall/gjson"
)

// @Name ai-proxy
// @Category custom
// @Phase UNSPECIFIED_PHASE
// @Priority 0
// @Title zh-CN AI代理
// @Description zh-CN 通过AI助手提供智能对话服务
// @IconUrl https://img.alicdn.com/imgextra/i1/O1CN018iKKih1iVx287RltL_!!6000000004419-2-tps-42-42.png
// @Version 0.1.0
//
// @Contact.name CH3CHO
// @Contact.url https://github.com/CH3CHO
// @Contact.email ch3cho@qq.com
//
// @Example
// { "provider": { "type": "qwen", "apiToken": "YOUR_DASHSCOPE_API_TOKEN", "modelMapping": { "*": "qwen-turbo" } } }
// @End
type PluginConfig struct {
	// @Title zh-CN AI服务提供商配置
	// @Description zh-CN AI服务提供商配置，包含API接口、模型和知识库文件等信息
	providerConfigs []provider.ProviderConfig `required:"true" yaml:"providers"`

	activeProviderConfig *provider.ProviderConfig `yaml:"-"`
	activeProvider       provider.Provider        `yaml:"-"`
}

func (c *PluginConfig) FromJson(json gjson.Result) {
	if providersJson := json.Get("providers"); providersJson.Exists() && providersJson.IsArray() {
		c.providerConfigs = make([]provider.ProviderConfig, 0)
		for _, providerJson := range providersJson.Array() {
			providerConfig := provider.ProviderConfig{}
			providerConfig.FromJson(providerJson)
			c.providerConfigs = append(c.providerConfigs, providerConfig)
		}
	}

	if providerJson := json.Get("provider"); providerJson.Exists() && providerJson.IsObject() {
		// TODO: For legacy config support. To be removed later.
		providerConfig := provider.ProviderConfig{}
		providerConfig.FromJson(providerJson)
		c.providerConfigs = []provider.ProviderConfig{providerConfig}
		c.activeProviderConfig = &providerConfig
		// Legacy configuration is used and the active provider is determined.
		// We don't need to continue with the new configuration style.
		return
	}

	c.activeProviderConfig = nil

	activeProviderId := json.Get("activeProviderId").String()
	if activeProviderId != "" {
		for _, providerConfig := range c.providerConfigs {
			if providerConfig.GetId() == activeProviderId {
				c.activeProviderConfig = &providerConfig
				break
			}
		}
	}
}

func (c *PluginConfig) Validate() error {
	if c.activeProviderConfig == nil {
		return nil
	}
	if err := c.activeProviderConfig.Validate(); err != nil {
		return err
	}
	return nil
}

func (c *PluginConfig) Complete() error {
	if c.activeProviderConfig == nil {
		c.activeProvider = nil
		return nil
	}

	var err error

	c.activeProvider, err = provider.CreateProvider(*c.activeProviderConfig)
	if err != nil {
		return err
	}

	providerConfig := c.GetProviderConfig()
	return providerConfig.SetApiTokensFailover(c.activeProvider)
}

func (c *PluginConfig) GetProvider() provider.Provider {
	return c.activeProvider
}

func (c *PluginConfig) GetProviderConfig() *provider.ProviderConfig {
	return c.activeProviderConfig
}

// SetActiveProviderForTest replaces the runtime Provider after Complete(); intended for unit tests in package main only.
func (c *PluginConfig) SetActiveProviderForTest(p provider.Provider) {
	c.activeProvider = p
}
