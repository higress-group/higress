module github.com/alibaba/higress/plugins/wasm-go/extensions/oauth

go 1.19

replace github.com/alibaba/higress/plugins/wasm-go => ../..

require (
	github.com/alibaba/higress/plugins/wasm-go v1.3.1
	github.com/golang-jwt/jwt/v5 v5.1.0
	github.com/google/uuid v1.4.0
	github.com/pkg/errors v0.9.1
	github.com/tetratelabs/proxy-wasm-go-sdk v0.22.0
	github.com/tidwall/gjson v1.17.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/higress-group/nottinygc v0.0.0-20231101025119-e93c4c2f8520 // indirect
	github.com/magefile/mage v1.14.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/testify v1.8.3 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
)
