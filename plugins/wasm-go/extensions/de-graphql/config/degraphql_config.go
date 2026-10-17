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

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	DefaultEndpoint          string = "/graphql"
	DefaultConnectionTimeout uint32 = 5000
)

var gqlVariableRegex = regexp.MustCompile(`\$(\w+)\s*:\s*(String|Float|Int|Boolean)(!?)`)

type VariableType string

const (
	StringType  VariableType = "String"
	IntType     VariableType = "Int"
	FloatType   VariableType = "Float"
	BooleanType VariableType = "Boolean"
)

type Variable struct {
	name  string
	typ   VariableType
	blank bool
	value string
}

// graphqlRequestBody is the JSON document that is POSTed to the upstream
// GraphQL endpoint. Variables is a typed map so that encoding/json, not hand
// written string concatenation, owns the escaping of the operation text and of
// every variable value, and so that the emitted object can only ever contain
// the variables declared by the configured gql operation.
type graphqlRequestBody struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type DeGraphQLConfig struct {
	gql       string
	endpoint  string
	timeout   uint32
	domain    string
	variables []Variable
}

func (d *DeGraphQLConfig) SetEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		d.endpoint = DefaultEndpoint
	} else {
		d.endpoint = endpoint
	}
	return nil
}

func (d *DeGraphQLConfig) GetDomain() string {
	return d.domain
}

func (d *DeGraphQLConfig) SetDomain(domain string) {
	d.domain = domain
}

func (d *DeGraphQLConfig) GetEndpoint() string {
	return d.endpoint
}

func (d *DeGraphQLConfig) GetTimeout() uint32 {
	return d.timeout
}

func (d *DeGraphQLConfig) SetTimeout(timeout uint32) {
	if timeout <= 0 {
		d.timeout = DefaultConnectionTimeout
	} else {
		d.timeout = timeout
	}
}

func (d *DeGraphQLConfig) SetGql(gql string) error {
	if strings.TrimSpace(gql) == "" {
		return errors.New("gql can't be empty")
	}
	d.gql = gql
	d.variables = make([]Variable, 0)
	matches := gqlVariableRegex.FindAllStringSubmatch(d.gql, -1)
	if len(matches) > 0 {
		for _, subMatch := range matches {
			variable := Variable{}
			variable.name = subMatch[1]
			switch subMatch[2] {
			case "String":
				variable.typ = StringType
			case "Float":
				variable.typ = FloatType
			case "Int":
				variable.typ = IntType
			case "Boolean":
				variable.typ = BooleanType
			}
			variable.blank = subMatch[3] != "!"
			d.variables = append(d.variables, variable)
		}

	}
	return nil
}

func (d *DeGraphQLConfig) GetGql() string {
	return d.gql
}

func (d *DeGraphQLConfig) GetVersion() string {
	return "1.0.0"
}

func (d *DeGraphQLConfig) ParseGqlFromUrl(requestUrl string) (string, error) {
	if strings.TrimSpace(requestUrl) == "" {
		return "", errors.New("request url can't be empty")
	}

	parsedUrl, err := url.Parse(requestUrl)
	if err != nil {
		return "", fmt.Errorf("invalid request url %q: %w", requestUrl, err)
	}

	queryValues := parsedUrl.Query()
	values := make(map[string]string, len(queryValues))
	for k, v := range queryValues {
		var v1 string
		if len(v) > 1 {
			v1 = strings.Join(v, ",")
		} else {
			v1 = v[0]
		}
		values[k] = v1
	}

	// Build the variables object from the values that were actually supplied,
	// keeping only the variables declared by the configured gql operation. Any
	// other query parameter is ignored, so it can never shadow or extend the
	// operation the operator configured.
	variables := make(map[string]any, len(d.variables))
	for _, variable := range d.variables {
		val, ok := values[variable.name]
		if !ok {
			continue
		}
		typedValue, err := variable.parseValue(val)
		if err != nil {
			return "", err
		}
		variables[variable.name] = typedValue
	}

	// Marshal the body with encoding/json instead of concatenating strings.
	// Hand written JSON corrupted the document whenever a value contained a
	// backslash or a quote, and writing non-string values verbatim let a caller
	// inject extra JSON (for example `?id=1,"admin":true`) into the upstream
	// request body.
	body := graphqlRequestBody{Query: d.gql}
	if len(variables) > 0 {
		body.Variables = variables
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal graphql request body: %w", err)
	}
	return string(encoded), nil
}

// parseValue converts the raw query string supplied for this variable into the
// Go value that encoding/json renders as the JSON scalar required by the
// declared GraphQL type. Values that cannot be represented as that scalar are
// rejected, so a caller cannot smuggle extra JSON into the upstream request
// body through a variable the operator declared as Int, Float or Boolean.
func (v Variable) parseValue(raw string) (any, error) {
	switch v.typ {
	case StringType:
		return raw, nil
	case IntType:
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q for Int variable %q", raw, v.name)
		}
		return value, nil
	case FloatType:
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q for Float variable %q", raw, v.name)
		}
		return value, nil
	case BooleanType:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q for Boolean variable %q", raw, v.name)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("variable %q has unsupported type %q", v.name, v.typ)
	}
}
