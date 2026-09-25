// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var errResourceAlreadyExists = errors.New("resource already exists")

type HigressClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

type HimarketClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
	jwtToken   string
}

// type ClientType string

// const (
// 	HigressClientType ClientType = "higress"
// 	HimarketClientType ClientType = "himarket"
// )

//	func NewClient(clientType ClientType, baseURL, username, password string) Client {
//		switch clientType {
//		case HimarketClientType:
//			return NewHimarketClient(baseURL, username, password)
//		case HigressClientType:
//			fallthrough
//		default:
//			return NewHigressClient(baseURL, username, password)
//		}
//	}
func NewHigressClient(baseURL, username, password string) *HigressClient {

	client := &HigressClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	return client
}

func NewHimarketClient(baseURL, username, password string) *HimarketClient {
	client := &HimarketClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	return client
}

func (c *HigressClient) Get(path string) ([]byte, error) {
	return c.request("GET", path, nil)
}

func (c *HigressClient) Post(path string, data interface{}) ([]byte, error) {
	return c.request("POST", path, data)
}

func (c *HigressClient) Put(path string, data interface{}) ([]byte, error) {
	return c.request("PUT", path, data)
}

func (c *HigressClient) Delete(path string) ([]byte, error) {
	return c.request("DELETE", path, nil)
}

func (c *HimarketClient) getJWTToken() error {
	loginURL := c.baseURL + "/api/v1/admins/login"

	loginData := map[string]string{
		"username": c.username,
		"password": c.password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return fmt.Errorf("failed to marshal login data: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("login failed with status code: %d", resp.StatusCode)
	}

	var response map[string]interface{}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read login response: %w", err)
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return fmt.Errorf("failed to parse login response: %w", err)
	}

	// fmt.Println(string(respBody))

	if data, ok := response["data"].(map[string]interface{}); ok {
		if token, ok := data["access_token"].(string); ok {
			c.jwtToken = token
			return nil
		}
	}

	return fmt.Errorf("token not found in login response: %v", response)
}

func (c *HimarketClient) Get(path string) ([]byte, error) {
	return c.request("GET", path, nil)
}

func (c *HimarketClient) Post(path string, data interface{}) ([]byte, error) {
	return c.request("POST", path, data)
}

func (c *HimarketClient) Put(path string, data interface{}) ([]byte, error) {
	return c.request("PUT", path, data)
}

func (c *HimarketClient) request(method, path string, data interface{}) ([]byte, error) {
	if c.jwtToken == "" {
		if err := c.getJWTToken(); err != nil {
			return nil, fmt.Errorf("failed to get JWT token: %w", err)
		}
	}

	url := c.baseURL + path

	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request data: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.jwtToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		return nil, errResourceAlreadyExists
	}

	if resp.StatusCode == 400 {
		return nil, fmt.Errorf("invalid resource definition")
	}

	if resp.StatusCode == 500 {
		return nil, fmt.Errorf("server internal error")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return respBody, nil
}

func (c *HigressClient) request(method, path string, data interface{}) ([]byte, error) {
	url := c.baseURL + path

	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request data: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		return nil, errResourceAlreadyExists
	}

	// fmt.Println(resp)

	if resp.StatusCode == 400 {
		return nil, fmt.Errorf("invalid resource definition")
	}

	if resp.StatusCode == 500 {
		return nil, fmt.Errorf("server internal error")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return respBody, nil
}
