package util

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
)

const (
	HeaderContentType   = "Content-Type"
	HeaderPath          = ":path"
	HeaderAuthority     = ":authority"
	HeaderAuthorization = "Authorization"

	HeaderOriginalPath = "X-ENVOY-ORIGINAL-PATH"
	HeaderOriginalHost = "X-ENVOY-ORIGINAL-HOST"
	HeaderOriginalAuth = "X-HI-ORIGINAL-AUTH"

	MimeTypeTextPlain       = "text/plain"
	MimeTypeApplicationJson = "application/json"
)

var (
	RegRetrieveBatchPath                        = regexp.MustCompile(`^.*/v1/batches/(?P<batch_id>[^/]+)$`)
	RegCancelBatchPath                          = regexp.MustCompile(`^.*/v1/batches/(?P<batch_id>[^/]+)/cancel$`)
	RegRetrieveFilePath                         = regexp.MustCompile(`^.*/v1/files/(?P<file_id>[^/]+)$`)
	RegRetrieveFileContentPath                  = regexp.MustCompile(`^.*/v1/files/(?P<file_id>[^/]+)/content$`)
	RegRetrieveVideoPath                        = regexp.MustCompile(`^.*/v1/videos/(?P<video_id>[^/]+)$`)
	RegRetrieveVideoContentPath                 = regexp.MustCompile(`^.*/v1/videos/(?P<video_id>[^/]+)/content$`)
	RegVideoRemixPath                           = regexp.MustCompile(`^.*/v1/videos/(?P<video_id>[^/]+)/remix$`)
	RegRetrieveFineTuningJobPath                = regexp.MustCompile(`^.*/v1/fine_tuning/jobs/(?P<fine_tuning_job_id>[^/]+)$`)
	RegRetrieveFineTuningJobEventsPath          = regexp.MustCompile(`^.*/v1/fine_tuning/jobs/(?P<fine_tuning_job_id>[^/]+)/events$`)
	RegRetrieveFineTuningJobCheckpointsPath     = regexp.MustCompile(`^.*/v1/fine_tuning/jobs/(?P<fine_tuning_job_id>[^/]+)/checkpoints$`)
	RegCancelFineTuningJobPath                  = regexp.MustCompile(`^.*/v1/fine_tuning/jobs/(?P<fine_tuning_job_id>[^/]+)/cancel$`)
	RegResumeFineTuningJobPath                  = regexp.MustCompile(`^.*/v1/fine_tuning/jobs/(?P<fine_tuning_job_id>[^/]+)/resume$`)
	RegPauseFineTuningJobPath                   = regexp.MustCompile(`^.*/v1/fine_tuning/jobs/(?P<fine_tuning_job_id>[^/]+)/pause$`)
	RegFineTuningCheckpointPermissionPath       = regexp.MustCompile(`^.*/v1/fine_tuning/checkpoints/(?P<fine_tuned_model_checkpoint>[^/]+)/permissions$`)
	RegDeleteFineTuningCheckpointPermissionPath = regexp.MustCompile(`^.*/v1/fine_tuning/checkpoints/(?P<fine_tuned_model_checkpoint>[^/]+)/permissions/(?P<permission_id>[^/]+)$`)
	RegGeminiGenerateContent                    = regexp.MustCompile(`^.*/(?P<api_version>[^/]+)/models/(?P<model>[^:]+):generateContent`)
	RegGeminiStreamGenerateContent              = regexp.MustCompile(`^.*/(?P<api_version>[^/]+)/models/(?P<model>[^:]+):streamGenerateContent`)
)

type ErrorHandlerFunc func(statusCodeDetails string, err error) error

var ErrorHandler ErrorHandlerFunc = func(statusCodeDetails string, err error) error {
	return proxywasm.SendHttpResponseWithDetail(500, statusCodeDetails, CreateHeaders(HeaderContentType, MimeTypeTextPlain), []byte(err.Error()), -1)
}

func CreateHeaders(kvs ...string) [][2]string {
	headers := make([][2]string, 0, len(kvs)/2)
	for i := 0; i < len(kvs); i += 2 {
		headers = append(headers, [2]string{kvs[i], kvs[i+1]})
	}
	return headers
}

func OverwriteRequestPath(path string) error {
	return proxywasm.ReplaceHttpRequestHeader(HeaderPath, path)
}

func OverwriteRequestAuthorization(credential string) error {
	return proxywasm.ReplaceHttpRequestHeader(HeaderAuthorization, credential)
}

func OverwriteRequestHostHeader(headers http.Header, host string) {
	headers.Set(HeaderAuthority, host)
}

func OverwriteRequestPathHeader(headers http.Header, path string) {
	headers.Set(HeaderPath, path)
}

func OverwriteRequestPathHeaderByCapability(headers http.Header, apiName string, mapping map[string]string) {
	originPath := GetOriginalRequestPath()
	mappedPath := MapRequestPathByCapability(apiName, originPath, mapping)
	if mappedPath == "" {
		return
	}
	headers.Set(HeaderPath, mappedPath)
	log.Debugf("[OverwriteRequestPath] originPath=%s, mappedPath=%s", originPath, mappedPath)
}

func MapRequestPathByCapability(apiName string, originPath string, mapping map[string]string) string {
	/**
	这里实现不太优雅，理应通过 apiName 来判断使用哪个正则替换
	但 ApiName 定义在 provider 中， 而 provider 中又引用了 util
	会导致循环引用
	**/
	mappedPath, exist := mapping[apiName]
	if !exist {
		return ""
	}
	mappedPathOnly := mappedPath
	mappedQuery := ""
	if queryIndex := strings.Index(mappedPathOnly, "?"); queryIndex >= 0 {
		mappedPathOnly = mappedPathOnly[:queryIndex]
		mappedQuery = mappedPath[queryIndex:]
	}
	// 将查询字符串从原始路径中剥离，避免干扰正则匹配 video_id 等占位符
	pathOnly := originPath
	query := ""
	if queryIndex := strings.Index(originPath, "?"); queryIndex >= 0 {
		pathOnly = originPath[:queryIndex]
		query = originPath[queryIndex:]
	}
	if strings.Contains(mappedPath, "{") && strings.Contains(mappedPath, "}") {
		replacements := []struct {
			regx *regexp.Regexp
			key  string
		}{
			{RegRetrieveFilePath, "file_id"},
			{RegRetrieveFileContentPath, "file_id"},
			{RegRetrieveBatchPath, "batch_id"},
			{RegCancelBatchPath, "batch_id"},
			{RegRetrieveVideoPath, "video_id"},
			{RegRetrieveVideoContentPath, "video_id"},
			{RegVideoRemixPath, "video_id"},
		}

		for _, r := range replacements {
			if r.regx.MatchString(pathOnly) {
				subMatch := r.regx.FindStringSubmatch(pathOnly)
				if subMatch == nil {
					continue
				}
				index := r.regx.SubexpIndex(r.key)
				if index < 0 || index >= len(subMatch) {
					continue
				}
				id := subMatch[index]
				mappedPathOnly = strings.Replace(mappedPathOnly, "{"+r.key+"}", id, 1)
			}
		}
	}
	if mappedQuery != "" {
		mappedPath = mappedPathOnly + mappedQuery
	} else {
		mappedPath = mappedPathOnly
	}
	if query != "" {
		// 保留原始查询参数，例如 variant=thumbnail
		if strings.Contains(mappedPath, "?") {
			mappedPath = mappedPath + "&" + strings.TrimPrefix(query, "?")
		} else {
			mappedPath += query
		}
	}
	return mappedPath
}

func GetOriginalRequestPath() string {
	path, err := proxywasm.GetHttpRequestHeader(HeaderOriginalPath)
	if path != "" && err == nil {
		return path
	}
	if path, err = proxywasm.GetHttpRequestHeader(HeaderPath); err == nil {
		return path
	}
	return ""
}

func SetOriginalRequestPath(path string) {
	_ = proxywasm.ReplaceHttpRequestHeader(HeaderOriginalPath, path)
}

func GetOriginalRequestHost() string {
	host, err := proxywasm.GetHttpRequestHeader(HeaderOriginalHost)
	if host != "" && err == nil {
		return host
	}
	if host, err = proxywasm.GetHttpRequestHeader(HeaderAuthority); err == nil {
		return host
	}
	return ""
}

func SetOriginalRequestHost(host string) {
	_ = proxywasm.ReplaceHttpRequestHeader(HeaderOriginalHost, host)
}

func GetOriginalRequestAuth() string {
	auth, err := proxywasm.GetHttpRequestHeader(HeaderOriginalAuth)
	if auth != "" && err == nil {
		return auth
	}
	if auth, err = proxywasm.GetHttpRequestHeader(HeaderAuthorization); err == nil {
		return auth
	}
	return ""
}

func SetOriginalRequestAuth(auth string) {
	_ = proxywasm.ReplaceHttpRequestHeader(HeaderOriginalAuth, auth)
}

func OverwriteRequestAuthorizationHeader(headers http.Header, credential string) {
	headers.Set(HeaderAuthorization, credential)
}

func HeaderToSlice(header http.Header) [][2]string {
	slice := make([][2]string, 0, len(header))
	for key, values := range header {
		for _, value := range values {
			slice = append(slice, [2]string{key, value})
		}
	}
	return slice
}

func SliceToHeader(slice [][2]string) http.Header {
	header := make(http.Header)
	for _, pair := range slice {
		key := pair[0]
		value := pair[1]
		header.Add(key, value)
	}
	return header
}

func GetRequestHeaders() http.Header {
	header, _ := proxywasm.GetHttpRequestHeaders()
	return SliceToHeader(header)
}

func GetResponseHeaders() http.Header {
	headers, _ := proxywasm.GetHttpResponseHeaders()
	return SliceToHeader(headers)
}

func ReplaceRequestHeaders(headers http.Header) {
	headerSlice := HeaderToSlice(headers)
	_ = proxywasm.ReplaceHttpRequestHeaders(headerSlice)
}

func ReplaceResponseHeaders(headers http.Header) {
	headerSlice := HeaderToSlice(headers)
	_ = proxywasm.ReplaceHttpResponseHeaders(headerSlice)
}
