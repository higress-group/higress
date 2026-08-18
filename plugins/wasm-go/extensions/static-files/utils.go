package main

import (
	"net/http"
	"strings"
)

func convertHttpHeadersToStruct(responseHeaders http.Header) [][2]string {
	headerStruct := make([][2]string, len(responseHeaders))
	i := 0
	for key, values := range responseHeaders {
		headerStruct[i][0] = key
		headerStruct[i][1] = values[0]
		i++
	}
	return headerStruct
}

func getRootRequestPath(root string, path string) string {
	return root + path
}

func getAliasRequestPath(alias string, aliasPath string, path string) string {
	return strings.Replace(path, aliasPath, alias, -1)
}

func getIndexRequestPath(index []string, path string) *[]string {
	paths := make([]string, 0)
	for _, v := range index {
		if strings.HasSuffix(path, "/") {
			paths = append(paths, path+v)
		} else {
			paths = append(paths, path+"/"+v)
		}
	}
	return &paths
}

func contains(array []int, value int) bool {
	for _, v := range array {
		if v == value {
			return true
		}
	}
	return false
}
