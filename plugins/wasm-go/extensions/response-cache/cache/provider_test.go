package cache

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 回归：显式配置的 Redis timeout / servicePort 必须在窄化前校验，
// 非法值不得进入 Redis client 初始化（issue #4358）
func TestProviderConfigRangeValidation(t *testing.T) {
	base := `{"type":"redis","serviceName":"gpt-cache"`

	parse := func(t *testing.T, raw string) *ProviderConfig {
		t.Helper()
		pc := &ProviderConfig{}
		pc.FromJson(gjson.Parse(raw))
		return pc
	}

	t.Run("defaults are kept when fields are absent", func(t *testing.T) {
		pc := parse(t, base+`}`)
		require.NoError(t, pc.Validate())
		require.Equal(t, uint32(10000), pc.timeout)
		require.Equal(t, 6379, pc.servicePort)

		static := parse(t, `{"type":"redis","serviceName":"gpt-cache.static"}`)
		require.NoError(t, static.Validate())
		require.Equal(t, 80, static.servicePort)
	})

	t.Run("accepts timeout at legal boundaries", func(t *testing.T) {
		pc := parse(t, base+`,"timeout":1}`)
		require.NoError(t, pc.Validate())
		require.Equal(t, uint32(1), pc.timeout)

		pc = parse(t, base+`,"timeout":4294967295}`)
		require.NoError(t, pc.Validate())
		require.Equal(t, uint32(math.MaxUint32), pc.timeout)
	})

	t.Run("accepts servicePort at legal boundary", func(t *testing.T) {
		pc := parse(t, base+`,"servicePort":1}`)
		require.NoError(t, pc.Validate())
		require.Equal(t, 1, pc.servicePort)
	})

	t.Run("rejects explicit timeout below 1", func(t *testing.T) {
		for _, raw := range []string{
			base + `,"timeout":0}`,
			base + `,"timeout":-1}`,
		} {
			pc := parse(t, raw)
			require.ErrorContains(t, pc.Validate(), "timeout")
		}
	})

	t.Run("rejects timeout above uint32 range", func(t *testing.T) {
		pc := parse(t, base+`,"timeout":4294967296}`)
		require.ErrorContains(t, pc.Validate(), "timeout")
	})

	t.Run("rejects explicit servicePort below 1", func(t *testing.T) {
		for _, raw := range []string{
			base + `,"servicePort":0}`,
			base + `,"servicePort":-1}`,
		} {
			pc := parse(t, raw)
			require.ErrorContains(t, pc.Validate(), "servicePort")
		}
	})

	t.Run("rejects servicePort beyond transmittable int range", func(t *testing.T) {
		pc := parse(t, base+`,"servicePort":4294967296}`)
		require.ErrorContains(t, pc.Validate(), "servicePort")
	})

	// 回归：同一配置对象先解析非法配置、再解析合法配置时，
	// 上一次解析的 parseError 不得残留（对象复用安全）
	t.Run("reusing a config object resets stale parse errors", func(t *testing.T) {
		pc := &ProviderConfig{}

		pc.FromJson(gjson.Parse(base + `,"timeout":-1}`))
		require.ErrorContains(t, pc.Validate(), "timeout")

		pc.FromJson(gjson.Parse(base + `}`))
		require.NoError(t, pc.Validate())
		require.Equal(t, uint32(10000), pc.timeout)
		require.Equal(t, 6379, pc.servicePort)
	})

	// 反向复用：合法后再解析非法，必须重新报错而不是漏放
	t.Run("reusing a config object still detects new violations", func(t *testing.T) {
		pc := &ProviderConfig{}

		pc.FromJson(gjson.Parse(base + `}`))
		require.NoError(t, pc.Validate())

		pc.FromJson(gjson.Parse(base + `,"servicePort":0}`))
		require.ErrorContains(t, pc.Validate(), "servicePort")
	})
}
