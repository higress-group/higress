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

package address

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"
	"istio.io/istio/pkg/log"
)

const (
	NACOS_PATH          = "/nacos/serverlist"
	MODULE_HEADER_KEY   = "Request-Module"
	MODULE_HEADER_VALUE = "Naming"
	DEFAULT_INTERVAL    = 30 * time.Second
)

type NacosAddressProvider struct {
	serverAddr      string
	nacosAddr       string
	nacosBackupAddr []string
	namespace       string
	stop            chan struct{}
	trigger         chan struct{}
	cond            *sync.Cond
	isStop          *atomic.Bool
	mutex           *sync.Mutex
}

func NewNacosAddressProvider(serverAddr, namespace string) *NacosAddressProvider {
	provider := &NacosAddressProvider{
		serverAddr: serverAddr,
		namespace:  namespace,
		stop:       make(chan struct{}),
		trigger:    make(chan struct{}, 1),
		cond:       sync.NewCond(new(sync.Mutex)),
		isStop:     atomic.NewBool(false),
		mutex:      &sync.Mutex{},
	}
	go provider.Run()
	return provider
}

func (p *NacosAddressProvider) Run() {
	ticker := time.NewTicker(DEFAULT_INTERVAL)
	defer ticker.Stop()
	p.addressDiscovery()
	for {
		select {
		case <-p.trigger:
			p.addressDiscovery()
		case <-ticker.C:
			p.addressDiscovery()
		case <-p.stop:
			return
		}
	}
}

func (p *NacosAddressProvider) Update(serverAddr, namespace string) {
	p.mutex.Lock()
	p.serverAddr = serverAddr
	p.namespace = namespace
	p.mutex.Unlock()
	p.addressDiscovery()
}

func (p *NacosAddressProvider) Trigger() {
	p.cond.L.Lock()
	oldAddr := p.nacosAddr
	if len(p.nacosBackupAddr) > 0 {
		p.nacosAddr = p.nacosBackupAddr[rand.Intn(len(p.nacosBackupAddr))]
		for i := len(p.nacosBackupAddr) - 1; i >= 0; i-- {
			if p.nacosBackupAddr[i] == p.nacosAddr {
				p.nacosBackupAddr = append(p.nacosBackupAddr[:i], p.nacosBackupAddr[i+1:]...)
			}
		}
		p.nacosBackupAddr = append(p.nacosBackupAddr, oldAddr)
	}
	p.cond.Broadcast()
	p.cond.L.Unlock()
	select {
	case p.trigger <- struct{}{}:
	default:
	}
}

func (p *NacosAddressProvider) Stop() {
	p.isStop.Store(true)
	p.stop <- struct{}{}
}

func (p *NacosAddressProvider) GetNacosAddress(oldAddress string) <-chan string {
	addressChan := make(chan string)
	go func() {
		var addr string
		p.cond.L.Lock()
		defer p.cond.L.Unlock()
		log.Debugf("get nacos address, p.nacosAddr: %s, oldAddress: %s", p.nacosAddr, oldAddress)
		for p.nacosAddr == oldAddress || p.nacosAddr == "" {
			if p.isStop.Load() {
				return
			}
			p.cond.Wait()
		}
		addr = p.nacosAddr
		addressChan <- addr
	}()
	return addressChan
}

func (p *NacosAddressProvider) addressDiscovery() {
	p.mutex.Lock()
	url := fmt.Sprintf("%s%s?namespace=%s", p.serverAddr, NACOS_PATH, p.namespace)
	p.mutex.Unlock()
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Errorf("create request failed, err:%v, url:%s", err, url)
		return
	}
	req.Header.Add(MODULE_HEADER_KEY, MODULE_HEADER_VALUE)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Errorf("get nacos address failed, err:%v, url:%s", err, url)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Errorf("get nacos address failed, statusCode:%d", resp.StatusCode)
		return
	}
	body, _ := ioutil.ReadAll(resp.Body)
	addresses := string(body)
	addrVec := strings.Fields(addresses)
	if len(addrVec) == 0 {
		return
	}
	needUpdate := true
	p.cond.L.Lock()
	defer p.cond.L.Unlock()
	for _, address := range addrVec {
		ip := net.ParseIP(address)
		if ip == nil {
			log.Errorf("ip parse failed, ip:%s", address)
			return
		}
		if p.nacosAddr == address {
			needUpdate = false
		}
	}
	p.nacosBackupAddr = addrVec
	if needUpdate {
		p.nacosAddr = addrVec[rand.Intn(len(addrVec))]
		p.cond.Broadcast()
		log.Infof("nacos address updated, address:%s", p.nacosAddr)
	}
	for i := len(p.nacosBackupAddr) - 1; i >= 0; i-- {
		if p.nacosBackupAddr[i] == p.nacosAddr {
			p.nacosBackupAddr = append(p.nacosBackupAddr[:i], p.nacosBackupAddr[i+1:]...)
		}
	}
}
