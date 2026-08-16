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
	"sync"

	"github.com/hudl/fargo"
	"istio.io/istio/pkg/log"
)

type Handler func(application *fargo.Application) error

/*
 * Plan can be used to get the latest information of a service
 *
 *                       (service B)  ┌──────┐
 *                      ┌────────────►│Plan B│
 *                      │   Timer     └──────┘
 *                      │
 * ┌───────────────┐    │
 * │ eureka-server ├────┤
 * └───────────────┘    │
 *                      │
 *                      │
 *                      │(service A)  ┌──────┐
 *                      └────────────►│Plan A│
 *                          Timer     └──────┘
 */

type Plan struct {
	client   EurekaHttpClient
	stop     chan struct{}
	stopOnce sync.Once
	handler  Handler
}

func NewPlan(client EurekaHttpClient, serviceName string, handler Handler) *Plan {
	p := &Plan{
		client:  client,
		stop:    make(chan struct{}),
		handler: handler,
	}

	ch := client.ScheduleAppUpdates(serviceName, p.stop)
	go p.watch(ch)
	return p
}

func (p *Plan) Stop() {
	p.stopOnce.Do(func() {
		close(p.stop)
	})
}

func (p *Plan) watch(ch <-chan fargo.AppUpdate) {
	for {
		select {
		case <-p.stop:
			log.Info("stop eureka plan")
			return
		case updateItem, ok := <-ch:
			if !ok {
				log.Info("eureka application update channel is closed")
				return
			}
			if updateItem.Err != nil {
				log.Errorf("get eureka application failed, error : %v", updateItem.Err)
				continue
			}
			if updateItem.App == nil {
				log.Warn("received nil eureka application update")
				continue
			}
			if err := p.handler(updateItem.App); err != nil {
				log.Errorf("handle eureka application failed, error : %v", err)
			}
		}
	}
}
