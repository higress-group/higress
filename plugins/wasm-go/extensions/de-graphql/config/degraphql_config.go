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
	"errors"
	"net/url"
	"strings"

	"regexp"
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

	url, _ := url.Parse(requestUrl)

	queryValues := url.Query()
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

	variables := make([]Variable, 0, len(d.variables))
	for _, variable := range d.variables {
		val, ok := values[variable.name]
		// TODO validate variable type and blank
		if ok {
			variables = append(variables, Variable{
				name:  variable.name,
				typ:   variable.typ,
				blank: variable.blank,
				value: val,
			})
		}
	}

	var build strings.Builder

	// write query
	build.WriteString("{\"query\":")
	build.WriteString("\"")
	build.WriteString(getJsonStr(d.gql))
	build.WriteString("\"")

	// write variables
	if len(variables) > 0 {
		index := 0
		build.WriteString(",")
		build.WriteString("\"variables\":{")
		for _, variable := range variables {
			build.WriteString("\"")
			build.WriteString(variable.name)
			build.WriteString("\":")
			if variable.typ == StringType {
				build.WriteString("\"")
				build.WriteString(getJsonStr(variable.value))
				build.WriteString("\"")
			} else {
				build.WriteString(variable.value)
			}
			if index < len(variables)-1 {
				build.WriteString(",")
			}
			index++
		}
		build.WriteString("}")
	}

	build.WriteString("}")
	return build.String(), nil
}

func getJsonStr(str string) string {
	d := strings.ReplaceAll(str, "\"", "\\\"")
	return strings.ReplaceAll(d, "\n", "\\n")
}
