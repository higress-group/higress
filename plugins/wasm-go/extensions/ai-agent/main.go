package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-agent/dashscope"
	prompttpl "github.com/alibaba/higress/plugins/wasm-go/extensions/ai-agent/promptTpl"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

// 用于统计函数的递归调用次数
const (
	ToolCallsCount   = "ToolCallsCount"
	StreamContextKey = "Stream"
)

// react的正则规则
const (
	ActionPattern      = `Action:\s*(.*?)[.\n]`
	ActionInputPattern = `Action Input:\s*(.*)`
	FinalAnswerPattern = `Final Answer:(.*)`
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-agent",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessResponseBodyBy(onHttpResponseBody),
	)
}

func parseConfig(gjson gjson.Result, c *PluginConfig, log log.Log) error {
	initResponsePromptTpl(gjson, c)

	err := initAPIs(gjson, c)
	if err != nil {
		return err
	}

	initReActPromptTpl(gjson, c)

	initLLMClient(gjson, c)

	initJsonResp(gjson, c)

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config PluginConfig, log log.Log) types.Action {
	ctx.DisableReroute()
	return types.ActionContinue
}

func firstReq(ctx wrapper.HttpContext, config PluginConfig, prompt string, rawRequest Request, log log.Log) types.Action {
	log.Debugf("[onHttpRequestBody] firstreq:%s", prompt)

	var userMessage Message
	userMessage.Role = "user"
	userMessage.Content = prompt

	newMessages := []Message{userMessage}
	rawRequest.Messages = newMessages
	if rawRequest.Stream {
		ctx.SetContext(StreamContextKey, struct{}{})
		rawRequest.Stream = false
	}

	// replace old message and resume request qwen
	newbody, err := json.Marshal(rawRequest)
	if err != nil {
		return types.ActionContinue
	} else {
		log.Debugf("[onHttpRequestBody] newRequestBody: %s", string(newbody))
		err := proxywasm.ReplaceHttpRequestBody(newbody)
		if err != nil {
			log.Debugf("failed replace err: %s", err.Error())
			proxywasm.SendHttpResponse(200, [][2]string{{"content-type", "application/json; charset=utf-8"}}, []byte(fmt.Sprintf(config.ReturnResponseTemplate, "替换失败"+err.Error())), -1)
		}
		log.Debug("[onHttpRequestBody] replace request success")
		return types.ActionContinue
	}
}

func onHttpRequestBody(ctx wrapper.HttpContext, config PluginConfig, body []byte, log log.Log) types.Action {
	log.Debug("onHttpRequestBody start")
	defer log.Debug("onHttpRequestBody end")

	// 拿到请求
	var rawRequest Request
	err := json.Unmarshal(body, &rawRequest)
	if err != nil {
		log.Debugf("[onHttpRequestBody] body json umarshal err: %s", err.Error())
		return types.ActionContinue
	}
	log.Debugf("onHttpRequestBody rawRequest: %v", rawRequest)

	// 获取用户query
	var query string
	var history string
	messageLength := len(rawRequest.Messages)
	log.Debugf("[onHttpRequestBody] messageLength: %s", messageLength)
	if messageLength > 0 {
		query = rawRequest.Messages[messageLength-1].Content
		log.Debugf("[onHttpRequestBody] query: %s", query)
		if messageLength >= 3 {
			for i := 0; i < messageLength-1; i += 2 {
				history += "human: " + rawRequest.Messages[i].Content + "\nAI: " + rawRequest.Messages[i+1].Content
			}
		} else {
			history = ""
		}
	} else {
		return types.ActionContinue
	}

	if query == "" {
		log.Debug("parse query from request body failed")
		return types.ActionContinue
	}

	// 拼装agent prompt模板
	tool_desc := make([]string, 0)
	tool_names := make([]string, 0)
	for _, apisParam := range config.APIsParam {
		for _, tool_param := range apisParam.ToolsParam {
			tool_desc = append(tool_desc, fmt.Sprintf(prompttpl.TOOL_DESC, tool_param.ToolName, tool_param.Description, tool_param.Description, tool_param.Description, tool_param.Parameter), "\n")
			tool_names = append(tool_names, tool_param.ToolName)
		}
	}

	var prompt string
	if config.PromptTemplate.Language == "CH" {
		prompt = fmt.Sprintf(prompttpl.CH_Template,
			tool_desc,
			tool_names,
			config.PromptTemplate.CHTemplate.Question,
			config.PromptTemplate.CHTemplate.Thought1,
			config.PromptTemplate.CHTemplate.Observation,
			config.PromptTemplate.CHTemplate.Thought2,
			history,
			query)
	} else {
		prompt = fmt.Sprintf(prompttpl.EN_Template,
			tool_desc,
			tool_names,
			config.PromptTemplate.ENTemplate.Question,
			config.PromptTemplate.ENTemplate.Thought1,
			config.PromptTemplate.ENTemplate.Observation,
			config.PromptTemplate.ENTemplate.Thought2,
			history,
			query)
	}

	ctx.SetContext(ToolCallsCount, 0)

	// 清理历史对话记录
	dashscope.MessageStore.Clear()

	// 将请求加入到历史对话存储器中
	dashscope.MessageStore.AddForUser(prompt)

	// 开始第一次请求
	ret := firstReq(ctx, config, prompt, rawRequest, log)

	return ret
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config PluginConfig, log log.Log) types.Action {
	log.Debug("onHttpResponseHeaders start")
	defer log.Debug("onHttpResponseHeaders end")

	return types.ActionContinue
}

func extractJson(bodyStr string) (string, error) {
	// simply extract json from response body string
	startIndex := strings.Index(bodyStr, "{")
	endIndex := strings.LastIndex(bodyStr, "}") + 1

	// if not found
	if startIndex == -1 || startIndex >= endIndex {
		return "", errors.New("cannot find json in the response body")
	}

	jsonStr := bodyStr[startIndex:endIndex]

	// attempt to parse the JSON
	var result map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		return "", err
	}
	return jsonStr, nil
}

func completionContent(responseBody []byte, log log.Log, context string) (string, bool) {
	if !gjson.ValidBytes(responseBody) {
		log.Debugf("[%s] invalid completion response json", context)
		return "", false
	}
	content := gjson.GetBytes(responseBody, "choices.0.message.content")
	if !content.Exists() {
		log.Debugf("[%s] no choices in completion response", context)
		return "", false
	}
	return content.String(), true
}

func jsonFormat(llmClient wrapper.HttpClient, llmInfo LLMInfo, jsonSchema map[string]interface{}, assistantMessage Message, actionInput string, headers [][2]string, streamMode bool, rawResponse Response, log log.Log) string {
	prompt := fmt.Sprintf(prompttpl.Json_Resp_Template, jsonSchema, actionInput)

	messages := []dashscope.Message{{Role: "user", Content: prompt}}

	completion := dashscope.Completion{
		Model:    llmInfo.Model,
		Messages: messages,
	}

	completionSerialized, _ := json.Marshal(completion)
	var content string
	err := llmClient.Post(
		llmInfo.Path,
		headers,
		completionSerialized,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			// 得到gpt的返回结果
			if statusCode != http.StatusOK {
				log.Debugf("[jsonFormat] statusCode: %d", statusCode)
			}
			var ok bool
			content, ok = completionContent(responseBody, log, "jsonFormat")
			if !ok || content == "" {
				content = actionInput
			}
			log.Infof("[jsonFormat] content: %s", content)
			jsonStr, err := extractJson(content)
			if err != nil {
				log.Debugf("[onHttpRequestBody] extractJson err: %s", err.Error())
				jsonStr = content
			}

			if streamMode {
				stream(jsonStr, rawResponse, log)
			} else {
				noneStream(assistantMessage, jsonStr, rawResponse, log)
			}
		}, uint32(llmInfo.MaxExecutionTime))
	if err != nil {
		log.Debugf("[onHttpRequestBody] completion err: %s", err.Error())
		proxywasm.ResumeHttpRequest()
	}
	return content
}

func noneStream(assistantMessage Message, actionInput string, rawResponse Response, log log.Log) {
	if len(rawResponse.Choices) == 0 {
		log.Debug("[onHttpResponseBody] no choices in raw response")
		proxywasm.ResumeHttpResponse()
		return
	}
	assistantMessage.Role = "assistant"
	assistantMessage.Content = actionInput
	rawResponse.Choices[0].Message = assistantMessage
	newbody, err := json.Marshal(rawResponse)
	if err != nil {
		proxywasm.ResumeHttpResponse()
		return
	} else {
		proxywasm.ReplaceHttpResponseBody(newbody)

		log.Debug("[onHttpResponseBody] replace response success")
		proxywasm.ResumeHttpResponse()
	}
}

func stream(actionInput string, rawResponse Response, log log.Log) {
	headers := [][2]string{{"content-type", "text/event-stream; charset=utf-8"}}
	proxywasm.ReplaceHttpResponseHeaders(headers)
	// Remove quotes from actionInput
	actionInput = strings.Trim(actionInput, "\"")
	returnStreamResponseTemplate := `data:{"id":"%s","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],"model":"%s","object":"chat.completion","usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}` + "\n\ndata:[DONE]\n\n"
	newbody := fmt.Sprintf(returnStreamResponseTemplate, rawResponse.ID, actionInput, rawResponse.Model, rawResponse.Usage.PromptTokens, rawResponse.Usage.CompletionTokens, rawResponse.Usage.TotalTokens)
	log.Infof("[onHttpResponseBody] newResponseBody: ", newbody)
	proxywasm.ReplaceHttpResponseBody([]byte(newbody))

	log.Debug("[onHttpResponseBody] replace response success")
	proxywasm.ResumeHttpResponse()
}

func toolsCallResult(ctx wrapper.HttpContext, llmClient wrapper.HttpClient, llmInfo LLMInfo, jsonResp JsonResp, aPIsParam []APIsParam, aPIClient []wrapper.HttpClient, content string, rawResponse Response, log log.Log, statusCode int, responseBody []byte) {
	if statusCode != http.StatusOK {
		log.Debugf("statusCode: %d", statusCode)
	}
	log.Info("========function result========")
	log.Infof(string(responseBody))

	observation := "Observation: " + string(responseBody)

	dashscope.MessageStore.AddForUser(observation)

	completion := dashscope.Completion{
		Model:     llmInfo.Model,
		Messages:  dashscope.MessageStore,
		MaxTokens: llmInfo.MaxTokens,
	}

	headers := [][2]string{{"Content-Type", "application/json"}, {"Authorization", "Bearer " + llmInfo.APIKey}}
	completionSerialized, _ := json.Marshal(completion)
	err := llmClient.Post(
		llmInfo.Path,
		headers,
		completionSerialized,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			// 得到gpt的返回结果
			if statusCode != http.StatusOK {
				log.Debugf("[toolsCall] statusCode: %d", statusCode)
			}
			responseContent, ok := completionContent(responseBody, log, "toolsCall")
			if !ok {
				proxywasm.ResumeHttpRequest()
				return
			}
			log.Infof("[toolsCall] content: %s", responseContent)

			if responseContent != "" {
				retType, actionInput := toolsCall(ctx, llmClient, llmInfo, jsonResp, aPIsParam, aPIClient, responseContent, rawResponse, log)
				if retType == types.ActionContinue {
					// 得到了Final Answer
					var assistantMessage Message
					var streamMode bool
					if ctx.GetContext(StreamContextKey) == nil {
						streamMode = false
						if jsonResp.Enable {
							jsonFormat(llmClient, llmInfo, jsonResp.JsonSchema, assistantMessage, actionInput, headers, streamMode, rawResponse, log)
						} else {
							noneStream(assistantMessage, actionInput, rawResponse, log)
						}
					} else {
						streamMode = true
						if jsonResp.Enable {
							jsonFormat(llmClient, llmInfo, jsonResp.JsonSchema, assistantMessage, actionInput, headers, streamMode, rawResponse, log)
						} else {
							stream(actionInput, rawResponse, log)
						}
					}
				}
			} else {
				proxywasm.ResumeHttpRequest()
			}
		}, uint32(llmInfo.MaxExecutionTime))
	if err != nil {
		log.Debugf("[onHttpRequestBody] completion err: %s", err.Error())
		proxywasm.ResumeHttpRequest()
	}
}

func outputParser(response string, log log.Log) (string, string) {
	log.Debugf("Raw response:%s", response)

	start := strings.Index(response, "```")
	end := strings.LastIndex(response, "```")

	var jsonStr string
	if start != -1 && end != -1 {
		jsonStr = strings.TrimSpace(response[start+3 : end])
	} else {
		jsonStr = response
	}

	log.Debugf("Extracted JSON string:%s", jsonStr)

	var action map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &action)
	if err == nil {
		var actionName, actionInput string
		for key, value := range action {
			if strings.Contains(strings.ToLower(key), "input") {
				actionInput = fmt.Sprintf("%v", value)
			} else {
				actionName = fmt.Sprintf("%v", value)
			}
		}
		if actionName != "" && actionInput != "" {
			return actionName, actionInput
		}
	}
	if err != nil {
		log.Debugf("json parse err: %s", err.Error())
	} else {
		log.Debug("json parse missing action or action_input")
	}
	// Fallback to regex parsing if JSON unmarshaling fails
	pattern := `\{\s*"action":\s*"([^"]+)",\s*"action_input":\s*((?:\{[^}]+\})|"[^"]+")\s*\}`
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(jsonStr)

	if len(match) == 3 {
		action := match[1]
		actionInput := match[2]
		log.Debugf("Parsed action:%s, action_input:%s", action, actionInput)
		return action, actionInput
	}

	log.Debug("No valid action and action_input found in the response")
	return "", ""
}

func toolsCall(ctx wrapper.HttpContext, llmClient wrapper.HttpClient, llmInfo LLMInfo, jsonResp JsonResp, aPIsParam []APIsParam, aPIClient []wrapper.HttpClient, content string, rawResponse Response, log log.Log) (types.Action, string) {
	dashscope.MessageStore.AddForAssistant(content)

	action, actionInput := outputParser(content, log)

	// 得到最终答案
	if action == "Final Answer" {
		return types.ActionContinue, actionInput
	}
	count := ctx.GetContext(ToolCallsCount).(int)
	count++
	log.Debugf("toolCallsCount:%d, config.LLMInfo.MaxIterations=%d", count, llmInfo.MaxIterations)
	// 函数递归调用次数，达到了预设的循环次数，强制结束
	if int64(count) > llmInfo.MaxIterations {
		ctx.SetContext(ToolCallsCount, 0)
		return types.ActionContinue, ""
	} else {
		ctx.SetContext(ToolCallsCount, count)
	}

	// 没得到最终答案

	var urlStr string
	var headers [][2]string
	var apiClient wrapper.HttpClient
	var method string
	var reqBody []byte
	var maxExecutionTime int64

	for i, apisParam := range aPIsParam {
		maxExecutionTime = apisParam.MaxExecutionTime
		for _, tools_param := range apisParam.ToolsParam {
			if action == tools_param.ToolName {
				log.Infof("calls %s", tools_param.ToolName)
				log.Infof("actionInput: %s", actionInput)

				// 将大模型需要的参数反序列化
				var data map[string]interface{}
				if err := json.Unmarshal([]byte(actionInput), &data); err != nil {
					log.Debugf("Error: %s", err.Error())
					return types.ActionContinue, ""
				}

				method = tools_param.Method

				// 组装 URL 和请求体
				urlStr = apisParam.URL + tools_param.Path

				// 解析URL模板以查找路径参数
				urlParts := strings.Split(urlStr, "/")
				for i, part := range urlParts {
					if strings.Contains(part, "{") && strings.Contains(part, "}") {
						for _, param := range tools_param.ParamName {
							paramNameInPath := part[1 : len(part)-1]
							if paramNameInPath == param {
								if value, ok := data[param]; ok {
									// 删除已经使用过的
									delete(data, param)
									// 替换模板中的占位符
									urlParts[i] = url.QueryEscape(value.(string))
								}
							}
						}
					}
				}

				// 重新组合URL
				urlStr = strings.Join(urlParts, "/")

				queryParams := make([][2]string, 0)
				if method == "GET" {
					for _, param := range tools_param.ParamName {
						if value, ok := data[param]; ok {
							queryParams = append(queryParams, [2]string{param, fmt.Sprintf("%v", value)})
						}
					}
				} else if method == "POST" {
					var err error
					reqBody, err = json.Marshal(data)
					if err != nil {
						log.Debugf("Error marshaling JSON: %s", err.Error())
						return types.ActionContinue, ""
					}
				}

				// 组装 headers 和 key
				headers = [][2]string{{"Content-Type", "application/json"}}
				if apisParam.APIKey.Name != "" {
					if apisParam.APIKey.In == "query" {
						queryParams = append(queryParams, [2]string{apisParam.APIKey.Name, apisParam.APIKey.Value})
					} else if apisParam.APIKey.In == "header" {
						headers = append(headers, [2]string{"Authorization", apisParam.APIKey.Name + " " + apisParam.APIKey.Value})
					}
				}

				if len(queryParams) > 0 {
					// 将 key 拼接到 url 后面
					urlStr += "?"
					for i, param := range queryParams {
						if i != 0 {
							urlStr += "&"
						}
						urlStr += url.QueryEscape(param[0]) + "=" + url.QueryEscape(param[1])
					}
				}

				log.Debugf("url: %s", urlStr)

				apiClient = aPIClient[i]
				break
			}
		}
	}

	if apiClient != nil {
		err := apiClient.Call(
			method,
			urlStr,
			headers,
			reqBody,
			func(statusCode int, responseHeaders http.Header, responseBody []byte) {
				toolsCallResult(ctx, llmClient, llmInfo, jsonResp, aPIsParam, aPIClient, content, rawResponse, log, statusCode, responseBody)
			}, uint32(maxExecutionTime))
		if err != nil {
			log.Debugf("tool calls error: %s", err.Error())
			proxywasm.ResumeHttpRequest()
		}
	} else {
		return types.ActionContinue, ""
	}

	return types.ActionPause, ""
}

// 从response接收到firstreq的大模型返回
func onHttpResponseBody(ctx wrapper.HttpContext, config PluginConfig, body []byte, log log.Log) types.Action {
	log.Debugf("onHttpResponseBody start")
	defer log.Debugf("onHttpResponseBody end")

	// 初始化接收gpt返回内容的结构体
	var rawResponse Response
	err := json.Unmarshal(body, &rawResponse)
	if err != nil {
		log.Debugf("[onHttpResponseBody] body to json err: %s", err.Error())
		return types.ActionContinue
	}
	if len(rawResponse.Choices) == 0 {
		log.Debug("[onHttpResponseBody] no choices in response")
		return types.ActionContinue
	}
	content := rawResponse.Choices[0].Message.Content
	log.Infof("first content: %s", content)
	// 如果gpt返回的内容不是空的
	if content != "" {
		// 进入agent的循环思考，工具调用的过程中
		retType, _ := toolsCall(ctx, config.LLMClient, config.LLMInfo, config.JsonResp, config.APIsParam, config.APIClient, content, rawResponse, log)
		return retType
	} else {
		return types.ActionContinue
	}
}
