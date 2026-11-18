package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)


type localProviderConfig struct {
	// Service 名称（k8s Service 名，不带 namespace）
	serviceName string
	// Service 所在 namespace（为空默认 default 或取网关同 namespace）
	namespace string
	// 服务端口，例如 80
	servicePort int64
	// 模型名称（vLLM 部署时的 model name）
	model string
	// 超时时间（毫秒）
	timeout uint32
}

type localProviderInitializer struct{}

func (l *localProviderInitializer) InitConfig(json gjson.Result) {
	// 从插件配置中读取参数
	localConfig.serviceName = json.Get("serviceName").String()
	localConfig.namespace = json.Get("namespace").String()
	localConfig.servicePort = json.Get("servicePort").Int()
	localConfig.model = json.Get("model").String()
	localConfig.timeout = uint32(json.Get("timeout").Int())
	if localConfig.timeout == 0 {
		localConfig.timeout = 10000
	}
}

func (l *localProviderInitializer) ValidateConfig() error {
	if localConfig.serviceName == "" {
		return errors.New("[local] serviceName is required, e.g. qwen3-vl-8b-instruct-qwen3-vl-8b-instruct-engine-service")
	}
	if localConfig.servicePort == 0 {
		return errors.New("[local] servicePort is required")
	}
	return nil
}

func (l *localProviderInitializer) CreateProvider(c ProviderConfig) (Provider, error) {
	// 用 K8sCluster：ClusterName() = outbound|<port>||<svc>.<ns>.svc.cluster.local
	// 这正是 Higress/Envoy 基于 k8s Service 自动生成的 cluster 名。
	// namespace 为空时 K8sCluster 默认用 "default"；若要跨 namespace 需在配置里指定。
	ns := localConfig.namespace
	if ns == "" {
		ns = "default"
	}
	return &LocalProvider{
		config: c,
		client: wrapper.NewClusterClient(wrapper.K8sCluster{
			ServiceName: localConfig.serviceName,
			Namespace:   ns,
			Port:        localConfig.servicePort,
		}),
	}, nil
}

var localConfig localProviderConfig

type LocalProvider struct {
	config ProviderConfig
	client wrapper.HttpClient
}

func (l *LocalProvider) GetProviderType() string {
	return ProviderTypeLocal
}

func (l *LocalProvider) CallArgs(imageUrl string) CallArgs {
	model := localConfig.model
	if model == "" {
		model = "Qwen/Qwen3-VL-8B-Instruct" // vLLM 部署的模型名
	}

	// 构造与 vLLM compatible-mode 兼容的请求
	ocrReq := OcrReq{
		Model: model,
		Messages: []chatMessage{
			{
				Role: "user",
				Content: []content{
					{
						Type: "image_url",
						ImageUrl: imageURL{
							URL: imageUrl,
						},
					},
					{
						Type: "text",
						Text: "Please describe the content of this image in detail. Extract all text and explain what is in the image. Answer in the same language as the user's question.",
					},
				},
			},
		},
	}
	body, _ := json.Marshal(ocrReq)

	return CallArgs{
		Method: http.MethodPost,
		Url:    "/v1/chat/completions", // vLLM compatible-mode 端点
		Headers: [][2]string{
			{"Content-Type", "application/json"},
		},
		Body:               body,
		TimeoutMillisecond: localConfig.timeout,
	}
}

func (l *LocalProvider) DoOCR(
	imageUrl string,
	callback func(imageContent string, err error),
) error {
	args := l.CallArgs(imageUrl)
	err := l.client.Call(args.Method, args.Url, args.Headers, args.Body,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			if statusCode != http.StatusOK {
				err := errors.New("failed to do local OCR due to status code: " + strconv.Itoa(statusCode))
				log.Errorf("local OCR failed, status: %d, body: %s", statusCode, responseBody)
				callback("", err)
				return
			}
			log.Debugf("local OCR response: %s", responseBody)
			resp, err := parseOcrResponse(responseBody)
			if err != nil {
				err = fmt.Errorf("failed to parse local OCR response: %v", err)
				callback("", err)
				return
			}
			if len(resp.Choices) == 0 {
				err = errors.New("no OCR response found")
				callback("", err)
				return
			}
			callback(resp.Choices[0].Message.Content, nil)
		},
		localConfig.timeout,
	)
	return err
}

func parseOcrResponse(responseBody []byte) (*OcrResp, error) {
	var resp OcrResp
	err := json.Unmarshal(responseBody, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
