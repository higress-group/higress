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

package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/hudl/fargo"
	"istio.io/istio/pkg/log"
)

var httpClient = http.DefaultClient

type EurekaHttpClient interface {
	GetApplications() (*Applications, error)
	GetApplication(name string) (*fargo.Application, error)
	ScheduleAppUpdates(name string, stop <-chan struct{}) <-chan fargo.AppUpdate
	GetDelta() (*Applications, error)
}

func NewEurekaHttpClient(config EurekaHttpConfig) EurekaHttpClient {
	return &eurekaHttpClient{config}
}

type EurekaHttpConfig struct {
	BaseUrl               string
	ConnectTimeoutSeconds int // default 30
	PollInterval          int // default 30
	Retries               int // default 3
	RetryDelayTime        int // default 100ms
	EnableDelta           bool
}

func NewDefaultConfig() EurekaHttpConfig {
	return EurekaHttpConfig{
		ConnectTimeoutSeconds: 10,
		PollInterval:          30,
		EnableDelta:           true,
		Retries:               3,
		RetryDelayTime:        100,
	}
}

type eurekaHttpClient struct {
	EurekaHttpConfig
}

func (e *eurekaHttpClient) GetApplications() (*Applications, error) {
	return e.getApplications("/apps")
}

func (e *eurekaHttpClient) GetApplication(name string) (*fargo.Application, error) {
	return e.getApplication("/apps/" + name)
}

func (e *eurekaHttpClient) ScheduleAppUpdates(name string, stop <-chan struct{}) <-chan fargo.AppUpdate {
	updates := make(chan fargo.AppUpdate)

	consume := func(app *fargo.Application, err error) {
		// Drop attempted sends when the consumer hasn't received the last buffered update
		select {
		case updates <- fargo.AppUpdate{App: app, Err: err}:
		default:
		}
	}

	go func() {
		ticker := time.NewTicker(time.Duration(e.PollInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				close(updates)
				return
			case <-ticker.C:
				consume(e.GetApplication(name))
			}
		}
	}()

	return updates
}

func (e *eurekaHttpClient) GetDelta() (*Applications, error) {
	if !e.EnableDelta {
		return nil, fmt.Errorf("failed to get DeltaApplication, enableDelta is false")
	}
	return e.getApplications("/apps/delta")
}

func (c *eurekaHttpClient) getApplications(path string) (*Applications, error) {
	res, code, err := c.request(path)
	if err != nil {
		log.Errorf("Failed to get applications, err: %v", err)
		return nil, err
	}

	if code != 200 {
		log.Warnf("Failed to get applications, http code : %v", code)
	}

	var rj fargo.GetAppsResponseJson
	if err = json.Unmarshal(res, &rj); err != nil {
		log.Errorf("Failed to unmarshal response body to fargo.GetAppResponseJson, error: %v", err)
		return nil, err
	}

	apps := map[string]*fargo.Application{}
	for idx := range rj.Response.Applications {
		ignore := false
		app := rj.Response.Applications[idx]
		for _, instance := range app.Instances {
			if ip := net.ParseIP(instance.IPAddr); ip == nil {
				log.Warnf("the Non-IP IPAddr %s is not allowed, please check your app: %s", instance.IPAddr, app.Name)
				ignore = true
				break
			}
		}
		if ignore {
			continue
		}
		apps[app.Name] = app
	}

	for name, app := range apps {
		log.Debugf("Parsing metadata for app %v", name)
		if err := app.ParseAllMetadata(); err != nil {
			log.Errorf("Failed to parse metadata for app %v: %v", name, err)
			return nil, err
		}
	}

	return &Applications{
		Apps:         apps,
		HashCode:     rj.Response.AppsHashcode,
		VersionDelta: rj.Response.VersionsDelta,
	}, nil
}

func (c *eurekaHttpClient) getApplication(path string) (*fargo.Application, error) {
	res, code, err := c.request(path)
	if err != nil {
		log.Errorf("Failed to get application, err: %v", err)
		return nil, err
	}

	if code != 200 {
		log.Warnf("Failed to get application, http code : %v", code)
	}

	var rj fargo.GetAppResponseJson
	if err = json.Unmarshal(res, &rj); err != nil {
		log.Errorf("Failed to unmarshal response body to fargo.GetAppResponseJson, error: %v", err)
		return nil, err
	}

	return &rj.Application, nil
}

func (c *eurekaHttpClient) request(urlPath string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", c.getUrl(urlPath), nil)
	if err != nil {
		log.Errorf("Failed to new a Request, error: %v", err.Error())
		return nil, -1, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	retryConfig := []retry.Option{
		retry.Attempts(uint(c.Retries)),
		retry.Delay(time.Duration(c.RetryDelayTime)),
	}

	resp := &http.Response{}
	err = retry.Do(func() error {
		resp, err = httpClient.Do(req)
		return err
	}, retryConfig...)

	if err != nil {
		log.Errorf("Failed to get response from eureka-server, error : %v", err)
		return nil, -1, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Failed to read request body, error : %v", err)
		return nil, -1, err
	}

	log.Infof("Get eureka response from url=%v", req.URL)
	return body, resp.StatusCode, nil
}

func (c *eurekaHttpClient) getUrl(path string) string {
	return "http://" + c.BaseUrl + "/eureka" + path
}
