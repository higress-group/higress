package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertHttpHeadersToStruct(t *testing.T) {
	cases := []struct {
		name        string
		httpHeaders map[string][]string
		expected    [][2]string
	}{
		{
			name:        "empty header",
			httpHeaders: map[string][]string{},
			expected:    [][2]string{},
		},
		{
			name: "headers with content type",
			httpHeaders: map[string][]string{
				"Content-Type": []string{"application/json"},
			},
			expected: [][2]string{
				{"Content-Type", "application/json"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := convertHttpHeadersToStruct(c.httpHeaders)
			require.Equal(t, c.expected, result)
		})
	}
}

func TestGetRootRequestPath(t *testing.T) {
	cases := []struct {
		name     string
		root     string
		path     string
		expected string
	}{
		{
			name:     "test1",
			root:     "/a",
			path:     "/index.html",
			expected: "/a/index.html",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := getRootRequestPath(c.root, c.path)
			require.Equal(t, c.expected, result)
		})
	}
}

func TestGetAliasRequestPath(t *testing.T) {
	cases := []struct {
		name      string
		alias     string
		aliasPath string
		path      string
		expected  string
	}{
		{
			name:      "test1",
			alias:     "/b",
			aliasPath: "/a",
			path:      "/a/index.html",
			expected:  "/b/index.html",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := getAliasRequestPath(c.alias, c.aliasPath, c.path)
			require.Equal(t, c.expected, result)
		})
	}
}

func TestGetIndexRequestPath(t *testing.T) {
	cases := []struct {
		name     string
		index    []string
		path     string
		expected []string
	}{
		{
			name:     "test1",
			index:    []string{"index.html", "index.php"},
			path:     "/a",
			expected: []string{"/a/index.html", "/a/index.php"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := *getIndexRequestPath(c.index, c.path)
			require.Equal(t, c.expected, result)
		})
	}
}

func TestContains(t *testing.T) {
	cases := []struct {
		name     string
		array    []int
		value    int
		expected bool
	}{
		{
			name:     "multi value, contains",
			array:    []int{404, 304},
			value:    404,
			expected: true,
		},
		{
			name:     "multi value, no contains",
			array:    []int{404, 304},
			value:    200,
			expected: false,
		},
		{
			name:     "one value contains",
			array:    []int{200},
			value:    200,
			expected: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := contains(c.array, c.value)
			require.Equal(t, c.expected, result)
		})
	}
}
