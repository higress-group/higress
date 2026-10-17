package util

import (
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/higress-group/wasm-go/pkg/log"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/frontend-gray/config"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestGetCookieValue(t *testing.T) {
	var tests = []struct {
		cookie, cookieKey, output string
	}{
		{"", "uid", ""},
		{`cna=pf_9be76347560439f3b87daede1b485e37; uid=111`, "uid", "111"},
		{`cna=pf_9be76347560439f3b87daede1b485e37; userid=222`, "userid", "222"},
		{`uid=333`, "uid", "333"},
		{`cna=pf_9be76347560439f3b87daede1b485e37;`, "uid", ""},
		{`user; other=alice`, "user", ""},
		{`user=alice=admin; other=value`, "user", "alice=admin"},
	}
	for _, test := range tests {
		testName := test.cookie
		t.Run(testName, func(t *testing.T) {
			output := GetCookieValue(test.cookie, test.cookieKey)
			assert.Equal(t, test.output, output)
		})
	}
}

// 测试首页Rewrite重写
func TestIndexRewrite(t *testing.T) {
	matchRules := map[string]string{
		"/app1": "/mfe/app1/{version}/index.html",
		"/":     "/mfe/app1/{version}/index.html",
	}

	var tests = []struct {
		path, output string
	}{
		{"/app1/", "/mfe/app1/v1.0.0/index.html"},
		{"/app123", "/mfe/app1/v1.0.0/index.html"},
		{"/app1/index.html", "/mfe/app1/v1.0.0/index.html"},
		{"/app1/index.jsp", "/mfe/app1/v1.0.0/index.html"},
		{"/app1/xxx", "/mfe/app1/v1.0.0/index.html"},
		{"/xxxx", "/mfe/app1/v1.0.0/index.html"},
	}
	for _, test := range tests {
		testName := test.path
		t.Run(testName, func(t *testing.T) {
			output := IndexRewrite(testName, "v1.0.0", matchRules)
			assert.Equal(t, test.output, output)
		})
	}
}

func TestIndexRewrite2(t *testing.T) {
	matchRules := map[string]string{
		"/":       "/{version}/index.html",
		"/sta":    "/sta/{version}/index.html",
		"/static": "/static/{version}/index.html",
	}

	var tests = []struct {
		path, output string
	}{
		{"/static123", "/static/v1.0.0/index.html"},
		{"/static", "/static/v1.0.0/index.html"},
		{"/sta", "/sta/v1.0.0/index.html"},
		{"/", "/v1.0.0/index.html"},
	}
	for _, test := range tests {
		testName := test.path
		t.Run(testName, func(t *testing.T) {
			output := IndexRewrite(testName, "v1.0.0", matchRules)
			assert.Equal(t, test.output, output)
		})
	}
}

func TestPrefixFileRewrite(t *testing.T) {
	matchRules := map[string]string{
		// 前缀匹配
		"/":             "/mfe/app1/{version}",
		"/app2/":        "/mfe/app1/{version}",
		"/app1/":        "/mfe/app1/{version}",
		"/app1/prefix2": "/mfe/app1/{version}",
		"/mfe/app1":     "/mfe/app1/{version}",
	}

	var tests = []struct {
		path, output string
	}{
		{"/js/a.js", "/mfe/app1/v1.0.0/js/a.js"},
		{"/app2/js/a.js", "/mfe/app1/v1.0.0/js/a.js"},
		{"/app1/js/a.js", "/mfe/app1/v1.0.0/js/a.js"},
		{"/app1/prefix2/js/a.js", "/mfe/app1/v1.0.0/js/a.js"},
		{"/app1/prefix2/js/a.js", "/mfe/app1/v1.0.0/js/a.js"},
		{"/mfe/app1/js/a.js", "/mfe/app1/v1.0.0/js/a.js"},
	}
	for _, test := range tests {
		testName := test.path
		t.Run(testName, func(t *testing.T) {
			output := PrefixFileRewrite(testName, "v1.0.0", matchRules)
			assert.Equal(t, test.output, output)
		})
	}
}

func TestCheckIsHtmlRequest(t *testing.T) {
	var tests = []struct {
		p      string
		output bool
	}{
		{"/js/a.js", false},
		{"/js/a.js", false},
		{"/images/a.png", false},
		{"/index", true},
		{"/index.html", true},
		{"/demo.php", true},
	}
	for _, test := range tests {
		testPath := test.p
		t.Run(testPath, func(t *testing.T) {
			output := CheckIsHtmlRequest(testPath)
			assert.Equal(t, test.output, output)
		})
	}
}
func TestReplaceHtml(t *testing.T) {
	var tests = []struct {
		name  string
		input string
	}{
		{"demo", `{"injection":{"head":["<script>console.log('Head')</script>"],"body":{"first":["<script>console.log('BodyFirst')</script>"],"last":["<script>console.log('BodyLast')</script>"]},"last":["<script>console.log('BodyLast')</script>"]},"html": "<!DOCTYPE html>\n   <html lang=\"zh-CN\">\n<head>\n<title>app1</title>\n<meta charset=\"utf-8\" />\n</head>\n<body>\n\t测试替换html版本\n\t<br />\n\t版本: {version}\n\t<br />\n\t<script src=\"./{version}/a.js\"></script>\n</body>\n</html>"}`},
		{"demo-noBody", `{"injection":{"head":["<script>console.log('Head')</script>"],"body":{"first":["<script>console.log('BodyFirst')</script>"],"last":["<script>console.log('BodyLast')</script>"]},"last":["<script>console.log('BodyLast')</script>"]},"html": "<!DOCTYPE html>\n   <html lang=\"zh-CN\">\n<head>\n<title>app1</title>\n<meta charset=\"utf-8\" />\n</head>\n</html>"}`},
	}
	for _, test := range tests {
		testName := test.name
		t.Run(testName, func(t *testing.T) {
			grayConfig := &config.GrayConfig{}
			config.JsonToGrayConfig(gjson.Parse(test.input), grayConfig)
			result := InjectContent(grayConfig.Html, grayConfig.Injection, "")
			t.Logf("result-----: %v", result)
		})
	}
}

func TestIsIndexRequest(t *testing.T) {
	var tests = []struct {
		name   string
		input  string
		output bool
	}{
		{"/api/user.json", "/api/**", true},
		{"/api/blade-auth/oauth/captcha", "/api/**", true},
	}
	for _, test := range tests {
		testName := test.name
		t.Run(testName, func(t *testing.T) {
			matchResult, _ := doublestar.Match(test.input, testName)
			assert.Equal(t, test.output, matchResult)
		})
	}
}

// TestFilterGrayRuleMissingRule 回归用例：启用的灰度部署缺少同名规则时，
// FilterGrayRule 必须跳过它并回退到 BaseDeployment，而不是解引用 nil 触发 panic。
// 历史上该场景会 panic，panic 被 wasm 框架 recover 后请求静默回退到基线版本。
func TestFilterGrayRuleMissingRule(t *testing.T) {
	baseDeployment := &config.Deployment{Name: "base", Version: "base", BackendVersion: "base-backend"}

	t.Run("deployment without matching rule falls back to base", func(t *testing.T) {
		grayConfig := &config.GrayConfig{}
		grayConfig.BaseDeployment = baseDeployment
		grayConfig.Rules = []*config.GrayRule{
			{Name: "inner-user", GrayKeyValue: []string{"00000001"}},
		}
		grayConfig.GrayDeployments = []*config.Deployment{
			{Name: "beta-user", Enabled: true, Version: "gray"},
		}
		assert.NotPanics(t, func() {
			deployment := FilterGrayRule(grayConfig, "00000001", "")
			assert.Same(t, baseDeployment, deployment)
		})
	})

	t.Run("empty rules slice falls back to base", func(t *testing.T) {
		grayConfig := &config.GrayConfig{}
		grayConfig.BaseDeployment = baseDeployment
		grayConfig.GrayDeployments = []*config.Deployment{
			{Name: "beta-user", Enabled: true, Version: "gray"},
		}
		assert.NotPanics(t, func() {
			deployment := FilterGrayRule(grayConfig, "00000001", "")
			assert.Same(t, baseDeployment, deployment)
		})
	})

	t.Run("empty rules slice with cookie tag falls back to base", func(t *testing.T) {
		grayConfig := &config.GrayConfig{}
		grayConfig.BaseDeployment = baseDeployment
		grayConfig.GrayDeployments = []*config.Deployment{
			{Name: "beta-user", Enabled: true, Version: "gray"},
		}
		assert.NotPanics(t, func() {
			deployment := FilterGrayRule(grayConfig, "", "level=level3")
			assert.Same(t, baseDeployment, deployment)
		})
	})

	t.Run("unmatched deployment is skipped but later matched deployment still wins", func(t *testing.T) {
		betaDeployment := &config.Deployment{Name: "beta-user", Enabled: true, Version: "gray"}
		innerDeployment := &config.Deployment{Name: "inner-user", Enabled: true, Version: "gray-inner"}
		grayConfig := &config.GrayConfig{}
		grayConfig.BaseDeployment = baseDeployment
		grayConfig.Rules = []*config.GrayRule{
			{Name: "inner-user", GrayKeyValue: []string{"00000001"}},
		}
		grayConfig.GrayDeployments = []*config.Deployment{betaDeployment, innerDeployment}
		assert.NotPanics(t, func() {
			deployment := FilterGrayRule(grayConfig, "00000001", "")
			assert.Same(t, innerDeployment, deployment)
		})
	})

	t.Run("matching rule still selects gray deployment", func(t *testing.T) {
		grayDeployment := &config.Deployment{Name: "inner-user", Enabled: true, Version: "gray"}
		grayConfig := &config.GrayConfig{}
		grayConfig.BaseDeployment = baseDeployment
		grayConfig.Rules = []*config.GrayRule{
			{Name: "inner-user", GrayKeyValue: []string{"00000001"}},
		}
		grayConfig.GrayDeployments = []*config.Deployment{grayDeployment}
		assert.NotPanics(t, func() {
			deployment := FilterGrayRule(grayConfig, "00000001", "")
			assert.Same(t, grayDeployment, deployment)
		})
	})

	t.Run("matching cookie tag still selects gray deployment", func(t *testing.T) {
		grayDeployment := &config.Deployment{Name: "beta-user", Enabled: true, Version: "gray"}
		grayConfig := &config.GrayConfig{}
		grayConfig.BaseDeployment = baseDeployment
		grayConfig.Rules = []*config.GrayRule{
			{Name: "beta-user", GrayTagKey: "level", GrayTagValue: []string{"level3", "level5"}},
		}
		grayConfig.GrayDeployments = []*config.Deployment{grayDeployment}
		assert.NotPanics(t, func() {
			deployment := FilterGrayRule(grayConfig, "", "level=level3")
			assert.Same(t, grayDeployment, deployment)
		})
	})

	t.Run("no gray deployments falls back to base", func(t *testing.T) {
		grayConfig := &config.GrayConfig{}
		grayConfig.BaseDeployment = baseDeployment
		grayConfig.Rules = []*config.GrayRule{
			{Name: "inner-user", GrayKeyValue: []string{"00000001"}},
		}
		assert.NotPanics(t, func() {
			deployment := FilterGrayRule(grayConfig, "00000001", "")
			assert.Same(t, baseDeployment, deployment)
		})
	})
}

// noopLog 实现 wasm-go 的 log.Log 接口，供单元测试注入以静默日志。
// FilterGrayRule 跳过缺少同名规则的部署时会写 warning 日志，而 log 包在测试
// 环境下 pluginLog 默认为 nil，直接调用会 panic，所以按仓库内其它插件测试的
// 约定注入一个空实现。
type noopLog struct{}

func (noopLog) Trace(msg string)                          {}
func (noopLog) Tracef(format string, args ...interface{}) {}
func (noopLog) Debug(msg string)                          {}
func (noopLog) Debugf(format string, args ...interface{}) {}
func (noopLog) Info(msg string)                           {}
func (noopLog) Infof(format string, args ...interface{})  {}
func (noopLog) Warn(msg string)                           {}
func (noopLog) Warnf(format string, args ...interface{})  {}
func (noopLog) Error(msg string)                          {}
func (noopLog) Errorf(format string, args ...interface{}) {}
func (noopLog) Critical(msg string)                       {}
func (noopLog) Criticalf(format string, args ...interface{}) {
}
func (noopLog) ResetID(pluginID string) {}

func init() {
	log.SetPluginLog(noopLog{})
}
