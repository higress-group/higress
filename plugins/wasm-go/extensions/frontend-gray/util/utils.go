package util

import (
	"encoding/json"
	"hash/crc32"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/uuid"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/frontend-gray/config"

	"github.com/tidwall/gjson"
)

func GetRequestPath() string {
	requestPath, _ := proxywasm.GetHttpRequestHeader(":path")
	requestPath = path.Clean(requestPath)
	parsedURL, err := url.Parse(requestPath)
	if err == nil {
		requestPath = parsedURL.Path
	} else {
		return ""
	}
	return requestPath
}

func GetRouteName() (string, error) {
	if raw, err := proxywasm.GetProperty([]string{"route_name"}); err != nil {
		return "-", err
	} else {
		return string(raw), nil
	}
}
func IsRequestSkippedByHeaders(grayConfig *config.GrayConfig) bool {
	for headerKey, headerValue := range grayConfig.SkippedByHeaders {
		requestHeader, _ := proxywasm.GetHttpRequestHeader(headerKey)
		if requestHeader == headerValue {
			return true
		}
	}
	return false
}

func IsIndexRequest(requestPath string, indexPaths []string) bool {
	for _, prefix := range indexPaths {
		matchResult, err := doublestar.Match(prefix, requestPath)
		if err == nil && matchResult {
			return true
		}
	}
	return false
}

func IsGrayEnabled(requestPath string, grayConfig *config.GrayConfig) bool {
	if IsIndexRequest(requestPath, grayConfig.IndexPaths) {
		return true
	}
	// 当前路径中前缀为 SkippedPaths，则不走插件逻辑
	for _, prefix := range grayConfig.SkippedPaths {
		matchResult, err := doublestar.Match(prefix, requestPath)
		if err == nil && matchResult {
			return false
		}
	}

	//  如果是首页，进入插件逻辑
	if CheckIsHtmlRequest(requestPath) {
		return true
	}
	// 检查是否存在重写主机
	if grayConfig.Rewrite != nil && grayConfig.Rewrite.Host != "" {
		return true
	}
	// 检查header标识，判断是否需要跳过
	if IsRequestSkippedByHeaders(grayConfig) {
		return false
	}
	// 检查是否存在灰度版本配置
	return len(grayConfig.GrayDeployments) > 0
}

// 是否启用后端的灰度（全链路灰度）
func IsBackendGrayEnabled(grayConfig config.GrayConfig) bool {
	for _, deployment := range grayConfig.GrayDeployments {
		if deployment.BackendVersion != "" {
			return true
		}
	}
	return false
}

// GetCookieValue 根据 cookie 和 key 获取 cookie 值
func GetCookieValue(cookie string, key string) string {
	if cookie == "" {
		return ""
	}
	value := ""
	pairs := strings.Split(cookie, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 && kv[0] == key {
			value = kv[1]
			break
		}
	}
	return value
}

func ContainsValue(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

// headers: [][2]string -> map[string][]string
func ConvertHeaders(hs [][2]string) map[string][]string {
	ret := make(map[string][]string)
	for _, h := range hs {
		k, v := strings.ToLower(h[0]), h[1]
		ret[k] = append(ret[k], v)
	}
	return ret
}

// headers: map[string][]string -> [][2]string
func ReconvertHeaders(hs map[string][]string) [][2]string {
	var ret [][2]string
	for k, vs := range hs {
		for _, v := range vs {
			ret = append(ret, [2]string{k, v})
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i][0] < ret[j][0]
	})
	return ret
}

func GetRule(rules []*config.GrayRule, name string) *config.GrayRule {
	for _, rule := range rules {
		if rule.Name == name {
			return rule
		}
	}
	return nil
}

// 检查是否是页面
var indexSuffixes = []string{
	".html", ".htm", ".jsp", ".php", ".asp", ".aspx", ".erb", ".ejs", ".twig",
}

func CheckIsHtmlRequest(requestPath string) bool {
	if requestPath == "/" || requestPath == "" {
		return true
	}
	ext := path.Ext(requestPath)
	return ext == "" || ContainsValue(indexSuffixes, ext)
}

// SortKeysByLengthAndLexicographically 按长度降序和字典序排序键
func SortKeysByLengthAndLexicographically(matchRules map[string]string) []string {
	keys := make([]string, 0, len(matchRules))
	for prefix := range matchRules {
		keys = append(keys, prefix)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j]) // 按长度排序
		}
		return keys[i] < keys[j] // 按字典序排序
	})
	return keys
}

// 首页Rewrite
func IndexRewrite(path, version string, matchRules map[string]string) string {
	// 使用新的排序函数
	keys := SortKeysByLengthAndLexicographically(matchRules)

	// 遍历排序后的键以找到最长匹配
	for _, prefix := range keys {
		if strings.HasPrefix(path, prefix) {
			rewrite := matchRules[prefix]
			newPath := strings.Replace(rewrite, "{version}", version, -1)
			return newPath
		}
	}
	return path
}

func PrefixFileRewrite(path, version string, matchRules map[string]string) string {
	// 对规则的键进行排序
	sortedKeys := SortKeysByLengthAndLexicographically(matchRules)

	// 遍历排序后的键
	for _, prefix := range sortedKeys {
		if strings.HasPrefix(path, prefix) {
			// 找到第一个匹配的前缀就停止,因为它是最长的匹配
			replacement := strings.Replace(matchRules[prefix], "{version}", version, 1)
			newPath := strings.Replace(path, prefix, replacement+"/", 1)
			return filepath.Clean(newPath)
		}
	}

	// 如果没有匹配,返回原始路径
	return path
}

func GetVersion(grayConfig config.GrayConfig, deployment *config.Deployment, xPreHigressVersion string) *config.Deployment {
	// cookie 中为空，返回当前版本
	if xPreHigressVersion == "" {
		return deployment
	}

	// cookie 中和当前版本不相同，返回cookie中值
	if xPreHigressVersion != deployment.Version {
		deployments := append(grayConfig.GrayDeployments, grayConfig.BaseDeployment)
		for _, curDeployment := range deployments {
			if curDeployment.Version == xPreHigressVersion {
				return curDeployment
			}
		}
	}
	return deployment
}

// 从cookie中解析出灰度信息
func getBySubKey(grayInfoStr string, graySubKey string) string {
	// 首先对 URL 编码的字符串进行解码
	jsonStr, err := url.QueryUnescape(grayInfoStr)
	if err != nil {
		return ""
	}
	// 使用 gjson 从 JSON 字符串中提取 graySubKey 对应的值
	value := gjson.Get(jsonStr, graySubKey)

	// 检查所提取的值是否存在
	if !value.Exists() {
		return ""
	}
	// 返回字符串形式的值
	return value.String()
}

func GetGrayKey(grayKeyValueByCookie string, grayKeyValueByHeader string, graySubKey string) string {
	grayKeyValue := grayKeyValueByCookie
	if grayKeyValueByCookie == "" {
		grayKeyValue = grayKeyValueByHeader
	}

	// 如果有子key, 尝试从子key中获取值
	if graySubKey != "" {
		subKeyValue := getBySubKey(grayKeyValue, graySubKey)
		if subKeyValue != "" {
			grayKeyValue = subKeyValue
		}
	}
	return grayKeyValue
}

// 如果基础部署或任何灰度部署中包含VersionPredicates，则认为是多版本配置
func IsSupportMultiVersion(grayConfig config.GrayConfig) bool {
	if len(grayConfig.BaseDeployment.VersionPredicates) > 0 {
		return true
	}
	for _, deployment := range grayConfig.GrayDeployments {
		if len(deployment.VersionPredicates) > 0 {
			return true
		}
	}
	return false
}

func GetConditionRules(rules []*config.GrayRule, grayKeyValue string, cookie string) string {
	ruleMaps := map[string]bool{}
	for _, grayRule := range rules {
		if grayRule.GrayKeyValue != nil && len(grayRule.GrayKeyValue) > 0 && grayKeyValue != "" {
			ruleMaps[grayRule.Name] = ContainsValue(grayRule.GrayKeyValue, grayKeyValue)
			continue
		} else if grayRule.GrayTagKey != "" && grayRule.GrayTagValue != nil && len(grayRule.GrayTagValue) > 0 {
			grayTagValue := GetCookieValue(cookie, grayRule.GrayTagKey)
			ruleMaps[grayRule.Name] = ContainsValue(grayRule.GrayTagValue, grayTagValue)
			continue
		} else {
			ruleMaps[grayRule.Name] = false
		}
	}
	jsonBytes, err := json.Marshal(ruleMaps)
	if err != nil {
		return ""
	}
	return string(jsonBytes)
}

func GetGrayWeightUniqueId(cookie string, uniqueGrayTag string) string {
	uniqueId, _ := proxywasm.GetHttpRequestHeader(uniqueGrayTag)
	if uniqueId == "" {
		uniqueId = GetCookieValue(cookie, uniqueGrayTag)
	}
	if uniqueId == "" {
		uniqueId = strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	return uniqueId
}

// FilterGrayRule 过滤灰度规则
func FilterGrayRule(grayConfig *config.GrayConfig, grayKeyValue string, cookie string) *config.Deployment {
	if grayConfig.GrayWeight > 0 {
		uniqueId := GetGrayWeightUniqueId(cookie, grayConfig.UniqueGrayTag)
		// 计算哈希后取模
		mod := crc32.ChecksumIEEE([]byte(uniqueId)) % 100
		isGray := mod < uint32(grayConfig.GrayWeight)
		if isGray {
			for _, deployment := range grayConfig.GrayDeployments {
				if deployment.Enabled && deployment.Weight > 0 {
					return deployment
				}
			}
		}
		return grayConfig.BaseDeployment
	}

	for _, deployment := range grayConfig.GrayDeployments {
		grayRule := GetRule(grayConfig.Rules, deployment.Name)
		// 首先：先校验用户名单ID
		if grayRule.GrayKeyValue != nil && len(grayRule.GrayKeyValue) > 0 && grayKeyValue != "" {
			if ContainsValue(grayRule.GrayKeyValue, grayKeyValue) {
				return deployment
			}
		}
		//	第二：校验Cookie中的 GrayTagKey
		if grayRule.GrayTagKey != "" && grayRule.GrayTagValue != nil && len(grayRule.GrayTagValue) > 0 {
			grayTagValue := GetCookieValue(cookie, grayRule.GrayTagKey)
			if ContainsValue(grayRule.GrayTagValue, grayTagValue) {
				return deployment
			}
		}
	}
	return grayConfig.BaseDeployment
}

// FilterMultiVersionGrayRule 过滤多版本灰度规则
func FilterMultiVersionGrayRule(grayConfig *config.GrayConfig, grayKeyValue string, cookie string, requestPath string) *config.Deployment {
	// 首先根据灰度键值获取当前部署
	currentDeployment := FilterGrayRule(grayConfig, grayKeyValue, cookie)

	// 创建一个新的部署对象，初始化版本为当前部署的版本
	deployment := &config.Deployment{
		Version: currentDeployment.Version,
	}

	// 对版本谓词的键进行排序
	keys := SortKeysByLengthAndLexicographically(currentDeployment.VersionPredicates)

	// 遍历排序后的键
	for _, prefix := range keys {
		// 如果请求路径以当前前缀开头
		if strings.HasPrefix(requestPath, prefix) {
			deployment.Version = currentDeployment.VersionPredicates[prefix]
			return deployment
		}
	}
	return deployment
}

// InjectContent 用于将内容注入到 HTML 文档的指定位置
func InjectContent(originalHtml string, injectionConfig *config.Injection, globalConfigValue string) string {
	heads := injectionConfig.Head
	if globalConfigValue != "" {
		heads = append([]string{globalConfigValue}, injectionConfig.Head...)
	}
	headInjection := strings.Join(heads, "\n")
	bodyFirstInjection := strings.Join(injectionConfig.Body.First, "\n")
	bodyLastInjection := strings.Join(injectionConfig.Body.Last, "\n")

	// 使用 strings.Builder 来提高性能
	var sb strings.Builder
	// 预分配内存，避免多次内存分配
	sb.Grow(len(originalHtml) + len(headInjection) + len(bodyFirstInjection) + len(bodyLastInjection))
	sb.WriteString(originalHtml)

	modifiedHtml := sb.String()

	// 注入到头部
	modifiedHtml = strings.ReplaceAll(modifiedHtml, "</head>", headInjection+"\n</head>")
	// 注入到body头
	modifiedHtml = strings.ReplaceAll(modifiedHtml, "<body>", "<body>\n"+bodyFirstInjection)
	// 注入到body尾
	modifiedHtml = strings.ReplaceAll(modifiedHtml, "</body>", bodyLastInjection+"\n</body>")

	return modifiedHtml
}

func FixLocalStorageKey(newHtml string, localStorageGrayKey string) string {
	if localStorageGrayKey != "" {
		localStr := strings.ReplaceAll(`<script>
	!function(){var o,e,n="@@X_GRAY_KEY",t=document.cookie.split("; ").filter(function(o){return 0===o.indexOf(n+"=")});try{"undefined"!=typeof localStorage&&null!==localStorage&&(o=localStorage.getItem(n),e=0<t.length?decodeURIComponent(t[0].split("=")[1]):null,o)&&o.indexOf("=")<0&&e&&e!==o&&(document.cookie=n+"="+encodeURIComponent(o)+"; path=/;",window.location.reload())}catch(o){}}();
	</script>
	`, "@@X_GRAY_KEY", localStorageGrayKey)
		newHtml = strings.ReplaceAll(newHtml, "<body>", "<body>\n"+localStr)
	}
	return newHtml
}
