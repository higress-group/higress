package cache

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestProviderConfigTimeout(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		wantTimeout int64
		wantErr     string
	}{
		{
			name:        "use default when omitted",
			config:      `{"type":"redis","serviceName":"redis.static"}`,
			wantTimeout: 10000,
		},
		{
			name:        "accept minimum",
			config:      `{"type":"redis","serviceName":"redis.static","timeout":1}`,
			wantTimeout: 1,
		},
		{
			name:        "accept maximum",
			config:      `{"type":"redis","serviceName":"redis.static","timeout":4294967295}`,
			wantTimeout: math.MaxUint32,
		},
		{
			name:    "reject zero",
			config:  `{"type":"redis","serviceName":"redis.static","timeout":0}`,
			wantErr: "cache service timeout must be between 1 and 4294967295 milliseconds",
		},
		{
			name:    "reject negative",
			config:  `{"type":"redis","serviceName":"redis.static","timeout":-1}`,
			wantErr: "cache service timeout must be between 1 and 4294967295 milliseconds",
		},
		{
			name:    "reject uint32 overflow",
			config:  `{"type":"redis","serviceName":"redis.static","timeout":4294967296}`,
			wantErr: "cache service timeout must be between 1 and 4294967295 milliseconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config ProviderConfig
			config.FromJson(gjson.Parse(tt.config))

			err := config.Validate()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantTimeout, int64(config.timeout))
		})
	}
}

func TestProviderConfigServicePort(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		wantPort int
		wantErr  string
	}{
		{
			name:     "use static service default when omitted",
			config:   `{"type":"redis","serviceName":"redis.static"}`,
			wantPort: 80,
		},
		{
			name:     "accept positive port",
			config:   `{"type":"redis","serviceName":"redis.static","servicePort":6379}`,
			wantPort: 6379,
		},
		{
			name:    "reject zero",
			config:  `{"type":"redis","serviceName":"redis.static","servicePort":0}`,
			wantErr: "cache service port must be greater than 0",
		},
		{
			name:    "reject negative",
			config:  `{"type":"redis","serviceName":"redis.static","servicePort":-1}`,
			wantErr: "cache service port must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config ProviderConfig
			config.FromJson(gjson.Parse(tt.config))

			err := config.Validate()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantPort, config.servicePort)
		})
	}
}
