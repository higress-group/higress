package main

import (
	"encoding/json"
	"errors"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v2"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model            string    `json:"model"`
	Messages         []Message `json:"messages"`
	FrequencyPenalty float64   `json:"frequency_penalty"`
	PresencePenalty  float64   `json:"presence_penalty"`
	Stream           bool      `json:"stream"`
	Temperature      float64   `json:"temperature"`
	Topp             int32     `json:"top_p"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Response struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Object  string   `json:"object"`
	Usage   Usage    `json:"usage"`
}

// 用于存放拆解出来的工具相关信息
type ToolsParam struct {
	ToolName    string   `yaml:"toolName"`
	Path        string   `yaml:"path"`
	Method      string   `yaml:"method"`
	ParamName   []string `yaml:"paramName"`
	Parameter   string   `yaml:"parameter"`
	Description string   `yaml:"description"`
}

// 用于存放拆解出来的api相关信息
type APIsParam struct {
	APIKey           APIKey       `yaml:"apiKey"`
	URL              string       `yaml:"url"`
	MaxExecutionTime int64        `yaml:"maxExecutionTime"`
	ToolsParam       []ToolsParam `yaml:"toolsParam"`
}

type Info struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

type Server struct {
	URL string `yaml:"url"`
}

// 给OpenAPI的get方法用的
type Parameter struct {
	Name        string `yaml:"name"`
	In          string `yaml:"in"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Schema      struct {
		Type    string   `yaml:"type"`
		Default string   `yaml:"default"`
		Enum    []string `yaml:"enum"`
	} `yaml:"schema"`
}

type Items struct {
	Type    string `yaml:"type"`
	Example string `yaml:"example"`
}

type Property struct {
	Description string   `yaml:"description"`
	Type        string   `yaml:"type"`
	Enum        []string `yaml:"enum,omitempty"`
	Items       *Items   `yaml:"items,omitempty"`
	MaxItems    int      `yaml:"maxItems,omitempty"`
	Example     string   `yaml:"example,omitempty"`
}

type Schema struct {
	Type       string              `yaml:"type"`
	Required   []string            `yaml:"required"`
	Properties map[string]Property `yaml:"properties"`
}

type MediaType struct {
	Schema Schema `yaml:"schema"`
}

// 给OpenAPI的post方法用的
type RequestBody struct {
	Required bool                 `yaml:"required"`
	Content  map[string]MediaType `yaml:"content"`
}

type PathItem struct {
	Description string      `yaml:"description"`
	Summary     string      `yaml:"summary"`
	OperationID string      `yaml:"operationId"`
	RequestBody RequestBody `yaml:"requestBody"`
	Parameters  []Parameter `yaml:"parameters"`
	Deprecated  bool        `yaml:"deprecated"`
}

type Paths map[string]map[string]PathItem

type Components struct {
	Schemas map[string]interface{} `yaml:"schemas"`
}

type API struct {
	OpenAPI    string     `yaml:"openapi"`
	Info       Info       `yaml:"info"`
	Servers    []Server   `yaml:"servers"`
	Paths      Paths      `yaml:"paths"`
	Components Components `yaml:"components"`
}

type APIKey struct {
	In    string `yaml:"in" json:"in"`
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value" json:"value"`
}

type APIProvider struct {
	// @Title zh-CN 服务名称
	// @Description zh-CN 带服务类型的完整 FQDN 名称，例如 my-redis.dns、redis.my-ns.svc.cluster.local
	ServiceName string `required:"true" yaml:"serviceName" json:"serviceName"`
	// @Title zh-CN 服务端口
	// @Description zh-CN 服务端口
	ServicePort int64 `required:"true" yaml:"servicePort" json:"servicePort"`
	// @Title zh-CN 服务域名
	// @Description zh-CN 服务域名，例如 restapi.amap.com
	Domain string `required:"true" yaml:"domain" json:"domain"`
	// @Title zh-CN 每一次请求api的超时时间
	// @Description zh-CN 每一次请求api的超时时间，单位毫秒，默认50000
	MaxExecutionTime int64 `yaml:"maxExecutionTime" json:"maxExecutionTime"`
	// @Title zh-CN 通义千问大模型服务的key
	// @Description zh-CN 通义千问大模型服务的key
	APIKey APIKey `required:"true" yaml:"apiKey" json:"apiKey"`
}

type APIs struct {
	APIProvider APIProvider `required:"true" yaml:"apiProvider" json:"apiProvider"`
	API         string      `required:"true" yaml:"api" json:"api"`
}

type Template struct {
	Question    string `yaml:"question" json:"question"`
	Thought1    string `yaml:"thought1" json:"thought1"`
	Observation string `yaml:"observation" json:"observation"`
	Thought2    string `yaml:"thought2" json:"thought2"`
}

type PromptTemplate struct {
	Language   string   `required:"true" yaml:"language" json:"language"`
	CHTemplate Template `yaml:"chTemplate" json:"chTemplate"`
	ENTemplate Template `yaml:"enTemplate" json:"enTemplate"`
}

type LLMInfo struct {
	// @Title zh-CN 大模型服务名称
	// @Description zh-CN 带服务类型的完整 FQDN 名称
	ServiceName string `required:"true" yaml:"serviceName" json:"serviceName"`
	// @Title zh-CN 大模型服务端口
	// @Description zh-CN 服务端口
	ServicePort int64 `required:"true" yaml:"servicePort" json:"servicePort"`
	// @Title zh-CN 大模型服务域名
	// @Description zh-CN 大模型服务域名，例如 dashscope.aliyuncs.com
	Domain string `required:"true" yaml:"domain" json:"domain"`
	// @Title zh-CN 大模型服务的key
	// @Description zh-CN 大模型服务的key
	APIKey string `required:"true" yaml:"apiKey" json:"apiKey"`
	// @Title zh-CN 大模型服务的请求路径
	// @Description zh-CN 大模型服务的请求路径，如"/compatible-mode/v1/chat/completions"
	Path string `required:"true" yaml:"path" json:"path"`
	// @Title zh-CN 大模型服务的模型名称
	// @Description zh-CN 大模型服务的模型名称，如"qwen-max-0403"
	Model string `required:"true" yaml:"model" json:"model"`
	// @Title zh-CN 结束执行循环前的最大步数
	// @Description zh-CN 结束执行循环前的最大步数，比如2，设置为0，可能会无限循环，直到超时退出，默认15
	MaxIterations int64 `yaml:"maxIterations" json:"maxIterations"`
	// @Title zh-CN 每一次请求大模型的超时时间
	// @Description zh-CN 每一次请求大模型的超时时间，单位毫秒，默认50000
	MaxExecutionTime int64 `yaml:"maxExecutionTime" json:"maxExecutionTime"`
	// @Title zh-CN
	// @Description zh-CN 每一次请求大模型的输出token限制，默认1000
	MaxTokens int64 `yaml:"maxToken" json:"maxTokens"`
}

type JsonResp struct {
	// @Title zh-CN Enable
	// @Description zh-CN 是否要启用json格式化输出
	Enable bool `yaml:"enable" json:"enable"`
	// @Title zh-CN Json Schema
	// @Description zh-CN 用以验证响应json的Json Schema, 为空则只验证返回的响应是否为合法json
	JsonSchema map[string]interface{} `required:"false" json:"jsonSchema" yaml:"jsonSchema"`
}

type PluginConfig struct {
	// @Title zh-CN 返回 HTTP 响应的模版
	// @Description zh-CN 用 %s 标记需要被 cache value 替换的部分
	ReturnResponseTemplate string `required:"true" yaml:"returnResponseTemplate" json:"returnResponseTemplate"`
	// @Title zh-CN 工具服务商以及工具信息
	// @Description zh-CN 用于存储工具服务商以及工具信息
	APIs      []APIs               `required:"true" yaml:"apis" json:"apis"`
	APIClient []wrapper.HttpClient `yaml:"-" json:"-"`
	// @Title zh-CN llm信息
	// @Description zh-CN 用于存储llm使用信息
	LLMInfo        LLMInfo            `required:"true" yaml:"llm" json:"llm"`
	LLMClient      wrapper.HttpClient `yaml:"-" json:"-"`
	APIsParam      []APIsParam        `yaml:"-" json:"-"`
	PromptTemplate PromptTemplate     `yaml:"promptTemplate" json:"promptTemplate"`
	JsonResp       JsonResp           `yaml:"jsonResp" json:"jsonResp"`
}

func initResponsePromptTpl(gjson gjson.Result, c *PluginConfig) {
	//设置回复模板
	c.ReturnResponseTemplate = gjson.Get("returnResponseTemplate").String()
	if c.ReturnResponseTemplate == "" {
		c.ReturnResponseTemplate = `{"id":"error","choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],"model":"gpt-4o","object":"chat.completion","usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	}
}

func initAPIs(gjson gjson.Result, c *PluginConfig) error {
	//从插件配置中获取apis信息
	apis := gjson.Get("apis")
	if !apis.Exists() {
		return errors.New("apis is required")
	}
	if len(apis.Array()) == 0 {
		return errors.New("apis cannot be empty")
	}

	for _, item := range apis.Array() {
		serviceName := item.Get("apiProvider.serviceName")
		if !serviceName.Exists() || serviceName.String() == "" {
			return errors.New("apiProvider serviceName is required")
		}

		servicePort := item.Get("apiProvider.servicePort")
		if !servicePort.Exists() || servicePort.Int() == 0 {
			return errors.New("apiProvider servicePort is required")
		}

		domain := item.Get("apiProvider.domain")
		if !domain.Exists() || domain.String() == "" {
			return errors.New("apiProvider domain is required")
		}

		maxExecutionTime := item.Get("apiProvider.maxExecutionTime").Int()
		if maxExecutionTime == 0 {
			maxExecutionTime = 50000
		}

		apiKeyIn := item.Get("apiProvider.apiKey.in").String()

		apiKeyName := item.Get("apiProvider.apiKey.name")

		apiKeyValue := item.Get("apiProvider.apiKey.value")

		//根据多个toolsClientInfo的信息，分别初始化toolsClient
		apiClient := wrapper.NewClusterClient(wrapper.FQDNCluster{
			FQDN: serviceName.String(),
			Port: servicePort.Int(),
			Host: domain.String(),
		})

		c.APIClient = append(c.APIClient, apiClient)

		api := item.Get("api")
		if !api.Exists() || api.String() == "" {
			return errors.New("api is required")
		}

		var apiStruct API
		err := yaml.Unmarshal([]byte(api.String()), &apiStruct)
		if err != nil {
			return err
		}
		if len(apiStruct.Servers) == 0 || apiStruct.Servers[0].URL == "" {
			return errors.New("api servers is required")
		}

		var allTool_param []ToolsParam
		//拆除服务下面的每个api的path
		for path, pathmap := range apiStruct.Paths {
			//拆解出每个api对应的参数
			for method, submap := range pathmap {
				//把参数列表存起来
				var param ToolsParam
				param.Path = path
				param.ToolName = submap.OperationID
				if method == "get" {
					param.Method = "GET"
					paramName := make([]string, 0)
					for _, parammeter := range submap.Parameters {
						paramName = append(paramName, parammeter.Name)
					}
					param.ParamName = paramName
					out, _ := json.Marshal(submap.Parameters)
					param.Parameter = string(out)
					param.Description = submap.Description
				} else if method == "post" {
					param.Method = "POST"
					schema := submap.RequestBody.Content["application/json"].Schema
					param.ParamName = schema.Required
					param.Description = submap.Summary
					out, _ := json.Marshal(schema.Properties)
					param.Parameter = string(out)
				}
				allTool_param = append(allTool_param, param)
			}
		}
		apiParam := APIsParam{
			APIKey:           APIKey{In: apiKeyIn, Name: apiKeyName.String(), Value: apiKeyValue.String()},
			URL:              apiStruct.Servers[0].URL,
			MaxExecutionTime: maxExecutionTime,
			ToolsParam:       allTool_param,
		}

		c.APIsParam = append(c.APIsParam, apiParam)
	}
	return nil
}

func initReActPromptTpl(gjson gjson.Result, c *PluginConfig) {
	c.PromptTemplate.Language = gjson.Get("promptTemplate.language").String()
	if c.PromptTemplate.Language != "EN" && c.PromptTemplate.Language != "CH" {
		c.PromptTemplate.Language = "EN"
	}
	if c.PromptTemplate.Language == "EN" {
		c.PromptTemplate.ENTemplate.Question = gjson.Get("promptTemplate.enTemplate.question").String()
		if c.PromptTemplate.ENTemplate.Question == "" {
			c.PromptTemplate.ENTemplate.Question = "input question to answer"
		}
		c.PromptTemplate.ENTemplate.Thought1 = gjson.Get("promptTemplate.enTemplate.thought1").String()
		if c.PromptTemplate.ENTemplate.Thought1 == "" {
			c.PromptTemplate.ENTemplate.Thought1 = "consider previous and subsequent steps"
		}
		c.PromptTemplate.ENTemplate.Observation = gjson.Get("promptTemplate.enTemplate.observation").String()
		if c.PromptTemplate.ENTemplate.Observation == "" {
			c.PromptTemplate.ENTemplate.Observation = "action result"
		}
		c.PromptTemplate.ENTemplate.Thought2 = gjson.Get("promptTemplate.enTemplate.thought2").String()
		if c.PromptTemplate.ENTemplate.Thought2 == "" {
			c.PromptTemplate.ENTemplate.Thought2 = "I know what to respond"
		}
	} else if c.PromptTemplate.Language == "CH" {
		c.PromptTemplate.CHTemplate.Question = gjson.Get("promptTemplate.chTemplate.question").String()
		if c.PromptTemplate.CHTemplate.Question == "" {
			c.PromptTemplate.CHTemplate.Question = "输入要回答的问题"
		}
		c.PromptTemplate.CHTemplate.Thought1 = gjson.Get("promptTemplate.chTemplate.thought1").String()
		if c.PromptTemplate.CHTemplate.Thought1 == "" {
			c.PromptTemplate.CHTemplate.Thought1 = "考虑之前和之后的步骤"
		}
		c.PromptTemplate.CHTemplate.Observation = gjson.Get("promptTemplate.chTemplate.observation").String()
		if c.PromptTemplate.CHTemplate.Observation == "" {
			c.PromptTemplate.CHTemplate.Observation = "行动结果"
		}
		c.PromptTemplate.CHTemplate.Thought2 = gjson.Get("promptTemplate.chTemplate.thought2").String()
		if c.PromptTemplate.CHTemplate.Thought2 == "" {
			c.PromptTemplate.CHTemplate.Thought2 = "我知道该回应什么"
		}
	}
}

func initLLMClient(gjson gjson.Result, c *PluginConfig) {
	c.LLMInfo.APIKey = gjson.Get("llm.apiKey").String()
	c.LLMInfo.ServiceName = gjson.Get("llm.serviceName").String()
	c.LLMInfo.ServicePort = gjson.Get("llm.servicePort").Int()
	c.LLMInfo.Domain = gjson.Get("llm.domain").String()
	c.LLMInfo.Path = gjson.Get("llm.path").String()
	c.LLMInfo.Model = gjson.Get("llm.model").String()
	c.LLMInfo.MaxIterations = gjson.Get("llm.maxIterations").Int()
	if c.LLMInfo.MaxIterations == 0 {
		c.LLMInfo.MaxIterations = 15
	}
	c.LLMInfo.MaxExecutionTime = gjson.Get("llm.maxExecutionTime").Int()
	if c.LLMInfo.MaxExecutionTime == 0 {
		c.LLMInfo.MaxExecutionTime = 50000
	}
	c.LLMInfo.MaxTokens = gjson.Get("llm.maxTokens").Int()
	if c.LLMInfo.MaxTokens == 0 {
		c.LLMInfo.MaxTokens = 1000
	}

	c.LLMClient = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: c.LLMInfo.ServiceName,
		Port: c.LLMInfo.ServicePort,
		Host: c.LLMInfo.Domain,
	})
}

func initJsonResp(gjson gjson.Result, c *PluginConfig) {
	c.JsonResp.Enable = false
	if c.JsonResp.Enable = gjson.Get("jsonResp.enable").Bool(); c.JsonResp.Enable {
		c.JsonResp.JsonSchema = nil
		if jsonSchemaValue := gjson.Get("jsonResp.jsonSchema"); jsonSchemaValue.Exists() {
			if schemaValue, ok := jsonSchemaValue.Value().(map[string]interface{}); ok {
				c.JsonResp.JsonSchema = schemaValue
			}
		}
	}
}
