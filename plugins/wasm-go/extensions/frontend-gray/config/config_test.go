package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestJsonToGrayConfig(t *testing.T) {
	allConfigData := `{"grayKey":"userid","rules":[{"name":"inner-user","grayKeyValue":["00000001","00000005"]},{"name":"beta-user","grayKeyValue":["00000002","00000003"],"grayTagKey":"level","grayTagValue":["level3","level5"]}],"deploy":{"base":{"version":"base"},"gray":[{"name":"beta-user","version":"gray","enabled":true}]}}`
	var tests = []struct {
		testName string
		grayKey  string
		json     string
	}{
		{"完整的数据", "userid", allConfigData},
	}
	for _, test := range tests {
		testName := test.testName
		t.Run(testName, func(t *testing.T) {
			var grayConfig = &GrayConfig{}
			JsonToGrayConfig(gjson.Parse(test.json), grayConfig)
			assert.Equal(t, test.grayKey, grayConfig.GrayKey)
		})
	}
}

// TestJsonToGrayConfigRuleValidation 覆盖灰度部署与灰度规则名字一致性的校验：
// 启用且未配置 weight 的 grayDeployments 必须能匹配到同名 rules，否则配置解析
// 直接返回错误，让问题在插件启动时暴露，而不是等到每个 HTML 请求再 panic。
func TestJsonToGrayConfigRuleValidation(t *testing.T) {
	t.Run("enabled deployment without matching rule returns error", func(t *testing.T) {
		json := `{"grayKey":"userid","rules":[{"name":"inner-user","grayKeyValue":["00000001"]}],"baseDeployment":{"version":"base"},"grayDeployments":[{"name":"beta-user","version":"gray","enabled":true}]}`
		grayConfig := &GrayConfig{}
		err := JsonToGrayConfig(gjson.Parse(json), grayConfig)
		assert.ErrorContains(t, err, "beta-user")
	})

	t.Run("enabled deployment with rules omitted returns error", func(t *testing.T) {
		json := `{"grayKey":"userid","baseDeployment":{"version":"base"},"grayDeployments":[{"name":"beta-user","version":"gray","enabled":true}]}`
		grayConfig := &GrayConfig{}
		err := JsonToGrayConfig(gjson.Parse(json), grayConfig)
		assert.ErrorContains(t, err, "beta-user")
	})

	t.Run("weighted deployment does not require a matching rule", func(t *testing.T) {
		json := `{"grayKey":"userid","rules":[{"name":"inner-user","grayKeyValue":["00000001"]}],"baseDeployment":{"version":"base"},"grayDeployments":[{"name":"beta-user","version":"gray","enabled":true,"weight":80}]}`
		grayConfig := &GrayConfig{}
		err := JsonToGrayConfig(gjson.Parse(json), grayConfig)
		assert.NoError(t, err)
		assert.Equal(t, 80, grayConfig.GrayWeight)
	})

	t.Run("enabled deployment with matching rule passes validation", func(t *testing.T) {
		json := `{"grayKey":"userid","rules":[{"name":"inner-user","grayKeyValue":["00000001"]}],"baseDeployment":{"version":"base"},"grayDeployments":[{"name":"inner-user","version":"gray","enabled":true}]}`
		grayConfig := &GrayConfig{}
		err := JsonToGrayConfig(gjson.Parse(json), grayConfig)
		assert.NoError(t, err)
		assert.Len(t, grayConfig.GrayDeployments, 1)
	})

	t.Run("disabled deployment without matching rule is ignored", func(t *testing.T) {
		json := `{"grayKey":"userid","rules":[{"name":"inner-user","grayKeyValue":["00000001"]}],"baseDeployment":{"version":"base"},"grayDeployments":[{"name":"beta-user","version":"gray","enabled":false}]}`
		grayConfig := &GrayConfig{}
		err := JsonToGrayConfig(gjson.Parse(json), grayConfig)
		assert.NoError(t, err)
		assert.Empty(t, grayConfig.GrayDeployments)
	})
}

// TestJsonToGrayConfigWeightedDeploymentKeepsParsing 确认带 weight 的部署仍然会被
// 正常解析并写入 GrayWeight，避免新增的规则校验误伤比例灰度配置。
func TestJsonToGrayConfigWeightedDeploymentKeepsParsing(t *testing.T) {
	json := `{"grayKey":"userid","rules":[{"name":"inner-user","grayKeyValue":["00000001"]}],"baseDeployment":{"version":"base","backendVersion":"base-backend"},"grayDeployments":[{"name":"beta-user","version":"gray","enabled":true,"weight":80,"backendVersion":"gray-backend"}]}`
	grayConfig := &GrayConfig{}
	err := JsonToGrayConfig(gjson.Parse(json), grayConfig)
	assert.NoError(t, err)
	assert.Equal(t, 80, grayConfig.GrayWeight)
	assert.Len(t, grayConfig.GrayDeployments, 1)
	assert.Equal(t, "beta-user", grayConfig.GrayDeployments[0].Name)
	assert.Equal(t, "gray-backend", grayConfig.GrayDeployments[0].BackendVersion)
	assert.Equal(t, "base-backend", grayConfig.BaseDeployment.BackendVersion)
}
