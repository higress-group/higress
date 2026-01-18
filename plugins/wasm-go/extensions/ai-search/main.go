// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-search/engine"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-search/engine/arxiv"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-search/engine/bing"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-search/engine/elasticsearch"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-search/engine/google"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-search/engine/quark"
)

type SearchRewrite struct {
	client             wrapper.HttpClient
	url                string
	apiKey             string
	modelName          string
	timeoutMillisecond uint32
	prompt             string
	promptTemplate     string // Original prompt template before replacing placeholders
	maxCount           int
}

type Config struct {
	engine            []engine.SearchEngine
	promptTemplate    string
	referenceFormat   string
	defaultLanguage   string
	needReference     bool
	referenceLocation string // "head" or "tail"
	searchRewrite     *SearchRewrite
	defaultEnable     bool
}

const (
	DEFAULT_MAX_BODY_BYTES uint32 = 100 * 1024 * 1024
)

//go:embed prompts/full.md
var fullSearchPrompts string

//go:embed prompts/arxiv.md
var arxivSearchPrompts string

//go:embed prompts/internet.md
var internetSearchPrompts string

//go:embed prompts/private.md
var privateSearchPrompts string

//go:embed prompts/chinese-internet.md
var chineseInternetSearchPrompts string

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-search",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onStreamingResponseBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
	)
}

func parseConfig(json gjson.Result, config *Config) error {
	config.defaultEnable = true // Default to true if not specified
	if json.Get("defaultEnable").Exists() {
		config.defaultEnable = json.Get("defaultEnable").Bool()
	}
	config.needReference = json.Get("needReference").Bool()
	if config.needReference {
		config.referenceFormat = json.Get("referenceFormat").String()
		if config.referenceFormat == "" {
			config.referenceFormat = "**References:**\n%s"
		} else if !strings.Contains(config.referenceFormat, "%s") {
			return fmt.Errorf("invalid referenceFormat:%s", config.referenceFormat)
		}

		config.referenceLocation = json.Get("referenceLocation").String()
		if config.referenceLocation == "" {
			config.referenceLocation = "head" // Default to head if not specified
		} else if config.referenceLocation != "head" && config.referenceLocation != "tail" {
			return fmt.Errorf("invalid referenceLocation:%s, must be 'head' or 'tail'", config.referenceLocation)
		}
	}
	config.defaultLanguage = json.Get("defaultLang").String()
	config.promptTemplate = json.Get("promptTemplate").String()
	if config.promptTemplate == "" {
		if config.needReference {
			config.promptTemplate = `# 以下内容是基于用户发送的消息的搜索结果:
{search_results}
在我给你的搜索结果中，每个结果都是[webpage X begin]...[webpage X end]格式的，X代表每篇文章的数字索引。请在适当的情况下在句子末尾引用上下文。请按照引用编号[X]的格式在答案中对应部分引用上下文。如果一句话源自多个上下文，请列出所有相关的引用编号，例如[3][5]，切记不要将引用集中在最后返回引用编号，而是在答案对应部分列出。
在回答时，请注意以下几点：
- 今天是北京时间：{cur_date}。
- 并非搜索结果的所有内容都与用户的问题密切相关，你需要结合问题，对搜索结果进行甄别、筛选。
- 对于列举类的问题（如列举所有航班信息），尽量将答案控制在10个要点以内，并告诉用户可以查看搜索来源、获得完整信息。优先提供信息完整、最相关的列举项；如非必要，不要主动告诉用户搜索结果未提供的内容。
- 对于创作类的问题（如写论文），请务必在正文的段落中引用对应的参考编号，例如[3][5]，不能只在文章末尾引用。你需要解读并概括用户的题目要求，选择合适的格式，充分利用搜索结果并抽取重要信息，生成符合用户要求、极具思想深度、富有创造力与专业性的答案。你的创作篇幅需要尽可能延长，对于每一个要点的论述要推测用户的意图，给出尽可能多角度的回答要点，且务必信息量大、论述详尽。
- 如果回答很长，请尽量结构化、分段落总结。如果需要分点作答，尽量控制在5个点以内，并合并相关的内容。
- 对于客观类的问答，如果问题的答案非常简短，可以适当补充一到两句相关信息，以丰富内容。
- 你需要根据用户要求和回答内容选择合适、美观的回答格式，确保可读性强。
- 你的回答应该综合多个相关网页来回答，不能重复引用一个网页。
- 除非用户要求，否则你回答的语言需要和用户提问的语言保持一致。

# 用户消息为：
{question}`
		} else {
			config.promptTemplate = `# 以下内容是基于用户发送的消息的搜索结果:
{search_results}
在我给你的搜索结果中，每个结果都是[webpage begin]...[webpage end]格式的。
在回答时，请注意以下几点：
- 今天是北京时间：{cur_date}。
- 并非搜索结果的所有内容都与用户的问题密切相关，你需要结合问题，对搜索结果进行甄别、筛选。
- 对于列举类的问题（如列举所有航班信息），尽量将答案控制在10个要点以内。如非必要，不要主动告诉用户搜索结果未提供的内容。
- 对于创作类的问题（如写论文），你需要解读并概括用户的题目要求，选择合适的格式，充分利用搜索结果并抽取重要信息，生成符合用户要求、极具思想深度、富有创造力与专业性的答案。你的创作篇幅需要尽可能延长，对于每一个要点的论述要推测用户的意图，给出尽可能多角度的回答要点，且务必信息量大、论述详尽。
- 如果回答很长，请尽量结构化、分段落总结。如果需要分点作答，尽量控制在5个点以内，并合并相关的内容。
- 对于客观类的问答，如果问题的答案非常简短，可以适当补充一到两句相关信息，以丰富内容。
- 你需要根据用户要求和回答内容选择合适、美观的回答格式，确保可读性强。
- 你的回答应该综合多个相关网页来回答，但回答中不要给出网页的引用来源。
- 除非用户要求，否则你回答的语言需要和用户提问的语言保持一致。

# 用户消息为：
{question}`
		}
	}
	if !strings.Contains(config.promptTemplate, "{search_results}") ||
		!strings.Contains(config.promptTemplate, "{question}") {
		return fmt.Errorf("invalid promptTemplate, must contains {search_results} and {question}:%s", config.promptTemplate)
	}
	var internetExists, privateExists, arxivExists bool
	var onlyQuark bool = true
	for _, e := range json.Get("searchFrom").Array() {
		switch e.Get("type").String() {
		case "bing":
			searchEngine, err := bing.NewBingSearch(&e)
			if err != nil {
				return fmt.Errorf("bing search engine init failed:%s", err)
			}
			config.engine = append(config.engine, searchEngine)
			internetExists = true
			onlyQuark = false
		case "google":
			searchEngine, err := google.NewGoogleSearch(&e)
			if err != nil {
				return fmt.Errorf("google search engine init failed:%s", err)
			}
			config.engine = append(config.engine, searchEngine)
			internetExists = true
			onlyQuark = false
		case "arxiv":
			searchEngine, err := arxiv.NewArxivSearch(&e)
			if err != nil {
				return fmt.Errorf("arxiv search engine init failed:%s", err)
			}
			config.engine = append(config.engine, searchEngine)
			arxivExists = true
			onlyQuark = false
		case "elasticsearch":
			searchEngine, err := elasticsearch.NewElasticsearchSearch(&e, config.needReference)
			if err != nil {
				return fmt.Errorf("elasticsearch search engine init failed:%s", err)
			}
			config.engine = append(config.engine, searchEngine)
			privateExists = true
			onlyQuark = false
		case "quark":
			searchEngine, err := quark.NewQuarkSearch(&e)
			if err != nil {
				return fmt.Errorf("quark search engine init failed:%s", err)
			}
			config.engine = append(config.engine, searchEngine)
			internetExists = true
		default:
			return fmt.Errorf("unknown search engine:%s", e.Get("type").String())
		}
	}
	searchRewriteJson := json.Get("searchRewrite")
	if searchRewriteJson.Exists() {
		searchRewrite := &SearchRewrite{}
		llmServiceName := searchRewriteJson.Get("llmServiceName").String()
		if llmServiceName == "" {
			return errors.New("llm_service_name not found")
		}
		llmServicePort := searchRewriteJson.Get("llmServicePort").Int()
		if llmServicePort == 0 {
			return errors.New("llmServicePort not found")
		}
		searchRewrite.client = wrapper.NewClusterClient(wrapper.FQDNCluster{
			FQDN: llmServiceName,
			Port: llmServicePort,
		})
		llmApiKey := searchRewriteJson.Get("llmApiKey").String()
		searchRewrite.apiKey = llmApiKey
		llmUrl := searchRewriteJson.Get("llmUrl").String()
		if llmUrl == "" {
			return errors.New("llmUrl not found")
		}
		searchRewrite.url = llmUrl
		llmModelName := searchRewriteJson.Get("llmModelName").String()
		if llmModelName == "" {
			return errors.New("llmModelName not found")
		}
		searchRewrite.modelName = llmModelName
		llmTimeout := searchRewriteJson.Get("timeoutMillisecond").Uint()
		if llmTimeout == 0 {
			llmTimeout = 30000
		}
		searchRewrite.timeoutMillisecond = uint32(llmTimeout)

		maxCount := searchRewriteJson.Get("maxCount").Int()
		if maxCount == 0 {
			maxCount = 3 // Default value
		}
		searchRewrite.maxCount = int(maxCount)
		// The consideration here is that internet searches are generally available, but arxiv and private sources may not be.
		if arxivExists {
			if privateExists {
				// private + internet + arxiv
				searchRewrite.prompt = fullSearchPrompts
			} else {
				// internet + arxiv
				searchRewrite.prompt = arxivSearchPrompts
			}
		} else if privateExists {
			// private + internet
			searchRewrite.prompt = privateSearchPrompts
		} else if internetExists {
			// only internet
			if onlyQuark {
				// When only quark is used, use chinese-internet.md
				searchRewrite.prompt = chineseInternetSearchPrompts
			} else {
				searchRewrite.prompt = internetSearchPrompts
			}
		}

		// Store the original prompt template before replacing placeholders
		searchRewrite.promptTemplate = searchRewrite.prompt
		// Replace {max_count} placeholder in the prompt with the configured value
		searchRewrite.prompt = strings.Replace(searchRewrite.prompt, "{max_count}", fmt.Sprintf("%d", searchRewrite.maxCount), -1)
		config.searchRewrite = searchRewrite
	}
	if len(config.engine) == 0 {
		return fmt.Errorf("no available search engine found")
	}
	log.Debugf("ai search enabled, config: %#v", config)
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	ctx.DisableReroute()
	contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
	// The request does not have a body.
	if contentType == "" {
		return types.ActionContinue
	}
	if !strings.Contains(contentType, "application/json") {
		log.Warnf("content is not json, can't process: %s", contentType)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	ctx.SetRequestBodyBufferLimit(DEFAULT_MAX_BODY_BYTES)
	_ = proxywasm.RemoveHttpRequestHeader("Accept-Encoding")
	_ = proxywasm.RemoveHttpRequestHeader("Content-Length")
	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	// Check if plugin should be enabled based on config and request
	webSearchOptions := gjson.GetBytes(body, "web_search_options")
	if !config.defaultEnable {
		// When defaultEnable is false, we need to check if web_search_options exists in the request
		if !webSearchOptions.Exists() {
			log.Debugf("Plugin disabled by config and no web_search_options in request")
			return types.ActionContinue
		}
		log.Debugf("Plugin enabled by web_search_options in request")
	}

	var queryIndex int
	var query string
	messages := gjson.GetBytes(body, "messages").Array()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Get("role").String() == "user" {
			queryIndex = i
			query = messages[i].Get("content").String()
			break
		}
	}
	if query == "" {
		log.Errorf("not found user query in body:%s", body)
		return types.ActionContinue
	}
	searchRewrite := config.searchRewrite
	if searchRewrite != nil {
		// Check if web_search_options.search_context_size exists and adjust maxCount accordingly
		if webSearchOptions.Exists() {
			searchContextSize := webSearchOptions.Get("search_context_size").String()
			if searchContextSize != "" {
				originalMaxCount := searchRewrite.maxCount
				switch searchContextSize {
				case "low":
					searchRewrite.maxCount = 1
					log.Debugf("Setting maxCount to 1 based on search_context_size=low")
				case "medium":
					searchRewrite.maxCount = 3
					log.Debugf("Setting maxCount to 3 based on search_context_size=medium")
				case "high":
					searchRewrite.maxCount = 5
					log.Debugf("Setting maxCount to 5 based on search_context_size=high")
				default:
					log.Warnf("Unknown search_context_size value: %s, using configured maxCount: %d",
						searchContextSize, searchRewrite.maxCount)
				}

				// If maxCount changed, regenerate the prompt from the template
				if originalMaxCount != searchRewrite.maxCount && searchRewrite.promptTemplate != "" {
					searchRewrite.prompt = strings.Replace(
						searchRewrite.promptTemplate,
						"{max_count}",
						fmt.Sprintf("%d", searchRewrite.maxCount),
						-1)
				}
			}
		}
		startTime := time.Now()
		rewritePrompt := strings.Replace(searchRewrite.prompt, "{question}", query, 1)
		rewriteBody, _ := sjson.SetBytes([]byte(fmt.Sprintf(
			`{"stream":false,"max_tokens":4096,"model":"%s","messages":[{"role":"user","content":""}]}`,
			searchRewrite.modelName)), "messages.0.content", rewritePrompt)
		err := searchRewrite.client.Post(searchRewrite.url,
			[][2]string{
				{"Content-Type", "application/json"},
				{"Authorization", fmt.Sprintf("Bearer %s", searchRewrite.apiKey)},
			}, rewriteBody,
			func(statusCode int, responseHeaders http.Header, responseBody []byte) {
				if statusCode != http.StatusOK {
					log.Errorf("search rewrite failed, status: %d, request url: %s, request cluster: %s, search rewrite model: %s",
						statusCode, searchRewrite.url, searchRewrite.client.ClusterName(), searchRewrite.modelName)
					// After a rewrite failure, no further search is performed, thus quickly identifying the failure.
					proxywasm.ResumeHttpRequest()
					return
				}

				content := gjson.GetBytes(responseBody, "choices.0.message.content").String()
				log.Infof("LLM rewritten query response: %s (took %v), original search query:%s",
					strings.ReplaceAll(content, "\n", `\n`), time.Since(startTime), query)
				if strings.Contains(content, "none") {
					log.Debugf("no search required")
					proxywasm.ResumeHttpRequest()
					return
				}

				// Parse search queries from LLM response
				var searchContexts []engine.SearchContext
				for _, line := range strings.Split(content, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}

					parts := strings.SplitN(line, ":", 2)
					if len(parts) != 2 {
						continue
					}

					engineType := strings.TrimSpace(parts[0])
					queryStr := strings.TrimSpace(parts[1])

					var ctx engine.SearchContext
					ctx.Language = config.defaultLanguage

					switch {
					case engineType == "internet":
						ctx.EngineType = engineType
						ctx.Querys = []string{queryStr}
					case engineType == "private":
						ctx.EngineType = engineType
						ctx.Querys = strings.Split(queryStr, ",")
						for i := range ctx.Querys {
							ctx.Querys[i] = strings.TrimSpace(ctx.Querys[i])
						}
					default:
						// Arxiv category
						ctx.EngineType = "arxiv"
						ctx.ArxivCategory = engineType
						ctx.Querys = strings.Split(queryStr, ",")
						for i := range ctx.Querys {
							ctx.Querys[i] = strings.TrimSpace(ctx.Querys[i])
						}
					}

					if len(ctx.Querys) > 0 {
						searchContexts = append(searchContexts, ctx)
						if ctx.ArxivCategory != "" {
							// Conduct i/nquiries in all areas to increase recall.
							backupCtx := ctx
							backupCtx.ArxivCategory = ""
							searchContexts = append(searchContexts, backupCtx)
						}
					}
				}

				if len(searchContexts) == 0 {
					log.Errorf("no valid search contexts found")
					proxywasm.ResumeHttpRequest()
					return
				}
				if types.ActionContinue == executeSearch(ctx, config, queryIndex, body, searchContexts) {
					proxywasm.ResumeHttpRequest()
				}
			}, searchRewrite.timeoutMillisecond)
		if err != nil {
			log.Errorf("search rewrite call llm service failed:%s", err)
			// After a rewrite failure, no further search is performed, thus quickly identifying the failure.
			return types.ActionContinue
		}
		return types.ActionPause
	}

	// Execute search without rewrite
	return executeSearch(ctx, config, queryIndex, body, []engine.SearchContext{{
		Querys:   []string{query},
		Language: config.defaultLanguage,
	}})
}

func executeSearch(ctx wrapper.HttpContext, config Config, queryIndex int, body []byte, searchContexts []engine.SearchContext) types.Action {
	searchResultGroups := make([][]engine.SearchResult, len(config.engine))
	var finished int
	var searching int
	for i := 0; i < len(config.engine); i++ {
		configEngine := config.engine[i]

		// Check if engine needs to execute for any of the search contexts
		var needsExecute bool
		for _, searchCtx := range searchContexts {
			if configEngine.NeedExectue(searchCtx) {
				needsExecute = true
				break
			}
		}
		if !needsExecute {
			continue
		}

		// Process all search contexts for this engine
		for _, searchCtx := range searchContexts {
			if !configEngine.NeedExectue(searchCtx) {
				continue
			}
			args := configEngine.CallArgs(searchCtx)
			index := i
			err := configEngine.Client().Call(args.Method, args.Url, args.Headers, args.Body,
				func(statusCode int, responseHeaders http.Header, responseBody []byte) {
					defer func() {
						finished++
						if finished == searching {
							// Merge search results from all engines with deduplication
							var mergedResults []engine.SearchResult
							seenLinks := make(map[string]bool)
							for _, results := range searchResultGroups {
								for _, result := range results {
									if !seenLinks[result.Link] {
										seenLinks[result.Link] = true
										mergedResults = append(mergedResults, result)
									}
								}
							}
							if len(mergedResults) == 0 {
								log.Warnf("no search result found, searchContexts:%#v", searchContexts)
								proxywasm.ResumeHttpRequest()
								return
							}
							// Format search results for prompt template
							var formattedResults []string
							var formattedReferences []string
							for j, result := range mergedResults {
								if config.needReference {
									formattedResults = append(formattedResults,
										fmt.Sprintf("[webpage %d begin]\n%s\n[webpage %d end]", j+1, result.Content, j+1))
									formattedReferences = append(formattedReferences,
										fmt.Sprintf("[%d] [%s](%s)", j+1, result.Title, result.Link))
								} else {
									formattedResults = append(formattedResults,
										fmt.Sprintf("[webpage begin]\n%s\n[webpage end]", result.Content))
								}
							}
							// Prepare template variables
							curDate := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006年1月2日")
							searchResults := strings.Join(formattedResults, "\n")
							log.Debugf("searchResults: %s", searchResults)
							// Fill prompt template
							prompt := strings.Replace(config.promptTemplate, "{search_results}", searchResults, 1)
							prompt = strings.Replace(prompt, "{question}", searchContexts[0].Querys[0], 1)
							prompt = strings.Replace(prompt, "{cur_date}", curDate, 1)
							// Update request body with processed prompt
							modifiedBody, err := sjson.SetBytes(body, fmt.Sprintf("messages.%d.content", queryIndex), prompt)
							if err != nil {
								log.Errorf("modify request message content failed, err:%v, body:%s", err, body)
							} else {
								log.Debugf("modifeid body:%s", modifiedBody)
								proxywasm.ReplaceHttpRequestBody(modifiedBody)
								if config.needReference {
									ctx.SetContext("References", strings.Join(formattedReferences, "\n\n"))
								}
							}
							proxywasm.ResumeHttpRequest()
						}
					}()
					if statusCode != http.StatusOK {
						log.Errorf("search call failed, status: %d, engine: %#v", statusCode, configEngine)
						return
					}
					// Append results to existing slice for this engine
					searchResultGroups[index] = append(searchResultGroups[index], configEngine.ParseResult(searchCtx, responseBody)...)
				}, args.TimeoutMillisecond)
			if err != nil {
				log.Errorf("search call failed, engine: %#v", configEngine)
				continue
			}
			searching++
		}
	}
	if searching > 0 {
		return types.ActionPause
	}
	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	if !config.needReference {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	proxywasm.RemoveHttpResponseHeader("content-length")
	contentType, err := proxywasm.GetHttpResponseHeader("Content-Type")
	if err != nil || !strings.HasPrefix(contentType, "text/event-stream") {
		if err != nil {
			log.Errorf("unable to load content-type header from response: %v", err)
		}
		ctx.BufferResponseBody()
		ctx.SetResponseBodyBufferLimit(DEFAULT_MAX_BODY_BYTES)
	}
	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	references := ctx.GetStringContext("References", "")
	if references == "" {
		return types.ActionContinue
	}
	content := gjson.GetBytes(body, "choices.0.message.content").String()
	var modifiedContent string
	formattedReferences := fmt.Sprintf(config.referenceFormat, references)

	if strings.HasPrefix(strings.TrimLeftFunc(content, unicode.IsSpace), "<think>") {
		thinkEnd := strings.Index(content, "</think>")
		if thinkEnd != -1 {
			if config.referenceLocation == "tail" {
				// Add references at the end
				modifiedContent = content + fmt.Sprintf("\n\n%s", formattedReferences)
			} else {
				// Default: add references after </think> tag
				modifiedContent = content[:thinkEnd+8] +
					fmt.Sprintf("\n%s\n\n%s", formattedReferences, content[thinkEnd+8:])
			}
		}
	}

	if modifiedContent == "" {
		if config.referenceLocation == "tail" {
			// Add references at the end
			modifiedContent = fmt.Sprintf("%s\n\n%s", content, formattedReferences)
		} else {
			// Default: add references at the beginning
			modifiedContent = fmt.Sprintf("%s\n\n%s", formattedReferences, content)
		}
	}

	body, err := sjson.SetBytes(body, "choices.0.message.content", modifiedContent)
	if err != nil {
		log.Errorf("modify response message content failed, err:%v, body:%s", err, body)
		return types.ActionContinue
	}
	proxywasm.ReplaceHttpResponseBody(body)
	return types.ActionContinue
}

const (
	PARTIAL_MESSAGE_CONTEXT_KEY = "partialMessage"
	BUFFER_CONTENT_CONTEXT_KEY  = "bufferContent"
	BUFFER_SIZE                 = 30
)

func onStreamingResponseBody(ctx wrapper.HttpContext, config Config, chunk []byte, isLastChunk bool) []byte {
	if ctx.GetBoolContext("ReferenceAppended", false) {
		return chunk
	}
	references := ctx.GetStringContext("References", "")
	if references == "" {
		return chunk
	}
	chunk = wrapper.UnifySSEChunk(chunk)
	var partialMessage []byte
	partialMessageI := ctx.GetContext(PARTIAL_MESSAGE_CONTEXT_KEY)
	log.Debugf("[handleStreamChunk] buffer content: %v", ctx.GetContext(BUFFER_CONTENT_CONTEXT_KEY))
	if partialMessageI != nil {
		partialMessage = append(partialMessageI.([]byte), chunk...)
	} else {
		partialMessage = chunk
	}
	messages := strings.Split(string(partialMessage), "\n\n")
	var newMessages []string
	for i, msg := range messages {
		if i < len(messages)-1 {
			newMsg := processSSEMessage(ctx, msg, fmt.Sprintf(config.referenceFormat, references), config.referenceLocation == "tail")
			if newMsg != "" {
				newMessages = append(newMessages, newMsg)
			}
		}
	}
	if !strings.HasSuffix(string(partialMessage), "\n\n") {
		ctx.SetContext(PARTIAL_MESSAGE_CONTEXT_KEY, []byte(messages[len(messages)-1]))
	} else {
		ctx.SetContext(PARTIAL_MESSAGE_CONTEXT_KEY, nil)
	}
	if len(newMessages) > 0 {
		return []byte(fmt.Sprintf("%s\n\n", strings.Join(newMessages, "\n\n")))
	} else {
		return []byte("")
	}
}

func processSSEMessage(ctx wrapper.HttpContext, sseMessage string, references string, tailReference bool) string {
	log.Debugf("single sse message: %s", sseMessage)
	subMessages := strings.Split(sseMessage, "\n")
	var message string
	for _, msg := range subMessages {
		if strings.HasPrefix(msg, "data:") {
			message = msg
			break
		}
	}
	if len(message) < 6 {
		log.Errorf("[processSSEMessage] invalid message: %s", message)
		return sseMessage
	}
	// Skip the prefix "data:"
	bodyJson := message[5:]
	if strings.TrimSpace(bodyJson) == "[DONE]" {
		return sseMessage
	}
	bodyJson = strings.TrimPrefix(bodyJson, " ")
	bodyJson = strings.TrimSuffix(bodyJson, "\n")

	// If tailReference is true, only check if this is the last message
	if tailReference {
		// Check if this is the last message in the stream (finish_reason is "stop")
		finishReason := gjson.Get(bodyJson, "choices.0.finish_reason").String()
		if finishReason == "stop" {
			// This is the last message, append references at the end
			deltaContent := gjson.Get(bodyJson, "choices.0.delta.content").String()
			modifiedMessage, err := sjson.Set(bodyJson, "choices.0.delta.content", deltaContent+fmt.Sprintf("\n\n%s", references))
			if err != nil {
				log.Errorf("update message failed:%s", err)
			}
			ctx.SetContext("ReferenceAppended", true)
			return fmt.Sprintf("data: %s", modifiedMessage)
		}
		// Not the last message, return original message
		return sseMessage
	}

	// Original head reference logic
	deltaContent := gjson.Get(bodyJson, "choices.0.delta.content").String()
	// Skip the preceding content that might be empty due to the presence of a separate reasoning_content field.
	if deltaContent == "" {
		return sseMessage
	}
	bufferContent := ctx.GetStringContext(BUFFER_CONTENT_CONTEXT_KEY, "") + deltaContent
	if len(bufferContent) < BUFFER_SIZE {
		ctx.SetContext(BUFFER_CONTENT_CONTEXT_KEY, bufferContent)
		return ""
	}
	if !ctx.GetBoolContext("FirstMessageChecked", false) {
		ctx.SetContext("FirstMessageChecked", true)
		if !strings.Contains(strings.TrimLeftFunc(bufferContent, unicode.IsSpace), "<think>") {
			modifiedMessage, err := sjson.Set(bodyJson, "choices.0.delta.content", fmt.Sprintf("%s\n\n%s", references, bufferContent))
			if err != nil {
				log.Errorf("update message failed:%s", err)
			}
			ctx.SetContext("ReferenceAppended", true)
			return fmt.Sprintf("data: %s", modifiedMessage)
		}
	}
	// Content has <think> prefix
	// Check for complete </think> tag
	thinkEnd := strings.Index(bufferContent, "</think>")
	if thinkEnd != -1 {
		modifiedContent := bufferContent[:thinkEnd+8] +
			fmt.Sprintf("\n%s\n\n%s", references, bufferContent[thinkEnd+8:])
		modifiedMessage, err := sjson.Set(bodyJson, "choices.0.delta.content", modifiedContent)
		if err != nil {
			log.Errorf("update message failed:%s", err)
		}
		ctx.SetContext("ReferenceAppended", true)
		return fmt.Sprintf("data: %s", modifiedMessage)
	}

	// Check for partial </think> tag at end of buffer
	// Look for any partial match that could be completed in next message
	for i := 1; i < len("</think>"); i++ {
		if strings.HasSuffix(bufferContent, "</think>"[:i]) {
			// Store only the partial match for the next message
			ctx.SetContext(BUFFER_CONTENT_CONTEXT_KEY, bufferContent[len(bufferContent)-i:])
			// Return the content before the partial match
			modifiedMessage, err := sjson.Set(bodyJson, "choices.0.delta.content", bufferContent[:len(bufferContent)-i])
			if err != nil {
				log.Errorf("update message failed:%s", err)
			}
			return fmt.Sprintf("data: %s", modifiedMessage)
		}
	}

	ctx.SetContext(BUFFER_CONTENT_CONTEXT_KEY, "")
	return sseMessage
}
