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

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tidwall/pretty"
)

const (
	ContentTypeApplicationJson = "application/json"
	ContentTypeFormUrlencoded  = "application/x-www-form-urlencoded"
	ContentTypeMultipartForm   = "multipart/form-data"
)

var (
	errGetRequestHost = errors.New("failed to get request host")
	errGetRequestPath = errors.New("failed to get request path")
	errEmptyBody      = errors.New("body is empty")
	errBodyType       = errors.New("unsupported body type")
	errGetContentType = errors.New("failed to get content-type from http context")
	errRemove         = errors.New("failed to remove")
	errRename         = errors.New("failed to rename")
	errReplace        = errors.New("failed to replace")
	errAdd            = errors.New("failed to add")
	errAppend         = errors.New("failed to append")
	errMap            = errors.New("failed to map")
	errDedupe         = errors.New("failed to dedupe")
	errContentTypeFmt = "unsupported content-type: %s"
)

func isValidOperation(op string) bool {
	switch op {
	case "remove", "rename", "replace", "add", "append", "map", "dedupe":
		return true
	default:
		return false
	}
}

func isValidMapSource(source string) bool {
	switch source {
	case "headers", "querys", "body":
		return true
	default:
		return false
	}
}

func parseQueryByPath(path string) (map[string][]string, error) {
	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}

	qs := make(map[string][]string)
	for k, vs := range u.Query() {
		qs[k] = vs
	}
	return qs, nil
}

func constructPath(path string, qs map[string][]string) (string, error) {
	u, err := url.Parse(path)
	if err != nil {
		return path, err
	}

	query := url.Values{}
	for k, vs := range qs {
		for _, v := range vs {
			query.Add(k, v)
		}
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// 返回值为 map[string]interface{} 或 map[string][]string，使用时断言即可
func parseBody(contentType string, body []byte) (interface{}, error) {
	if len(body) == 0 {
		return nil, errEmptyBody
	}

	typ, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	switch typ {
	case ContentTypeApplicationJson:
		return map[string]interface{}{"body": body}, nil

	case ContentTypeFormUrlencoded:
		ret := make(map[string][]string)
		kvs, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		for k, vs := range kvs {
			ret[k] = vs
		}
		return ret, nil

	case ContentTypeMultipartForm:
		ret := make(map[string][]string)
		mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			formName := p.FormName()
			fileName := p.FileName()
			contentType := p.Header.Get("Content-Type") // 获取文件的 MIME 类型

			if formName == "" {
				continue
			}
			formValue, err := io.ReadAll(p)
			if err != nil {
				return nil, err
			}

			if fileName == "" {
				ret[formName] = append(ret[formName], string(formValue))
			} else {
				// 文件字段：记录文件名和 base64 内容
				ret[formName+".filename"] = append(ret[formName+".filename"], fileName)
				ret[formName+".content-type"] = append(ret[formName+".content-type"], contentType) // 保存 MIME 类型
				encoded := base64.StdEncoding.EncodeToString(formValue)
				ret[formName+".content"] = append(ret[formName+".content"], encoded)
				ret[formName] = append(ret[formName], "")
			}

		}
		return ret, nil

	default:
		return nil, errors.Errorf(errContentTypeFmt, contentType)
	}
}

func constructBody(contentType string, body interface{}) ([]byte, error) {
	typ, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	switch typ {
	case ContentTypeApplicationJson:
		bd, ok := body.(map[string]interface{})["body"].([]byte)
		if !ok {
			return nil, errBodyType
		}
		return pretty.Pretty(bd), nil

	case ContentTypeFormUrlencoded:
		bd, ok := body.(map[string][]string)
		if !ok {
			return nil, errBodyType
		}
		query := url.Values{}
		for k, vs := range bd {
			for _, v := range vs {
				query.Add(k, v)
			}
		}
		return []byte(query.Encode()), nil

	case ContentTypeMultipartForm:
		bd, ok := body.(map[string][]string)
		if !ok {
			return nil, errBodyType
		}
		buf := new(bytes.Buffer)
		w := multipart.NewWriter(buf)
		if err = w.SetBoundary(params["boundary"]); err != nil {
			return nil, err
		}

		// 先处理文件字段（因为需要 .filename 和 .content）
		processed := make(map[string]bool) // 避免重复处理

		for k, vs := range bd {
			if strings.HasSuffix(k, ".filename") || strings.HasSuffix(k, ".content") || strings.HasSuffix(k, ".content-type") {
				continue
			}

			// 判断是否是文件字段
			filenames, hasFilename := bd[k+".filename"]
			contents, hasContent := bd[k+".content"]

			if hasFilename && hasContent {
				// 是文件字段
				processed[k] = true
				for i := 0; i < len(filenames) && i < len(contents); i++ {
					filename := filenames[i]
					content64 := contents[i]
					// 获取对应的 MIME 类型
					var contentType string
					if contentTypes, exists := bd[k+".content-type"]; exists && i < len(contentTypes) {
						contentType = contentTypes[i]
					} else {
						contentType = "application/octet-stream" // 默认值
					}
					content, err := base64.StdEncoding.DecodeString(content64)
					if err != nil {
						return nil, errors.Wrap(err, "decode base64 content failed")
					}

					// 创建带 Content-Type 的表单文件
					h := make(textproto.MIMEHeader)
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, k, filename))
					h.Set("Content-Type", contentType)

					fw, err := w.CreatePart(h)
					if err != nil {
						return nil, err
					}
					_, err = fw.Write(content)
					if err != nil {
						return nil, err
					}
				}
			} else {
				for _, v := range vs {
					if err = w.WriteField(k, v); err != nil {
						return nil, err
					}
				}
			}
		}
		if err = w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil

	default:
		return nil, errors.Errorf(errContentTypeFmt, contentType)
	}
}

func isValidRequestContentType(contentType string) bool {
	typ, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return typ == ContentTypeApplicationJson || typ == ContentTypeFormUrlencoded || typ == ContentTypeMultipartForm
}

func isValidResponseContentType(contentType string) bool {
	typ, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return typ == ContentTypeApplicationJson
}

func convertByJsonType(typ string, value string) (ret interface{}, err error) {
	switch strings.ToLower(typ) {
	case "object":
		err = json.Unmarshal([]byte(value), &ret)
	case "boolean":
		ret, err = strconv.ParseBool(value)
	case "number":
		ret, err = strconv.ParseFloat(value, 64)
	case "string":
		fallthrough
	default:
		ret = value
	}
	return
}

func isValidJsonType(typ string) bool {
	switch typ {
	case "object", "boolean", "number", "string":
		return true
	default:
		return false
	}
}

// headers: [][2]string -> map[string][]string
func convertHeaders(hs [][2]string) map[string][]string {
	ret := make(map[string][]string)
	for _, h := range hs {
		k, v := strings.ToLower(h[0]), h[1]
		ret[k] = append(ret[k], v)
	}
	return ret
}

// headers: map[string][]string -> [][2]string
func reconvertHeaders(hs map[string][]string) [][2]string {
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
