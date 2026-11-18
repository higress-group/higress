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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func setUpServer(status int, body []byte) (string, func()) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(status)
		rw.Write(body)
	}))
	return server.URL, func() {
		server.Close()
	}
}

// mockBody is the response body of a mock server. The test goroutine
// replaces it while the provider's discovery goroutine may be requesting
// the server, so access is guarded by a mutex.
type mockBody struct {
	mu   sync.Mutex
	data []byte
}

func newMockBody(data string) *mockBody {
	return &mockBody{data: []byte(data)}
}

func (b *mockBody) set(data string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = []byte(data)
}

func (b *mockBody) get() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data
}

// nacosAddr and setNacosAddr access the provider's current address under
// the same lock the provider's discovery goroutine uses.
func nacosAddr(p *NacosAddressProvider) string {
	p.cond.L.Lock()
	defer p.cond.L.Unlock()
	return p.nacosAddr
}

func setNacosAddr(p *NacosAddressProvider, addr string) {
	p.cond.L.Lock()
	defer p.cond.L.Unlock()
	p.nacosAddr = addr
}

func setUpServerWithBody(status int, body *mockBody) (string, func()) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(status)
		rw.Write(body.get())
	}))
	return server.URL, func() {
		server.Close()
	}
}
func TestGetNacosAddress(t *testing.T) {
	goodURL, goodTearDown := setUpServer(200, []byte("1.1.1.1\n 2.2.2.2"))
	defer goodTearDown()
	badURL, badTearDown := setUpServer(200, []byte("abc\n 2.2.2.2"))
	defer badTearDown()
	errURL, errTearDown := setUpServer(503, []byte("1.1.1.1\n 2.2.2.2"))
	defer errTearDown()
	tests := []struct {
		name       string
		serverAddr string
		want       []string
	}{
		{
			"good",
			goodURL,
			[]string{"1.1.1.1", "2.2.2.2"},
		},
		{
			"bad",
			badURL,
			[]string{},
		},
		{
			"err",
			errURL,
			[]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewNacosAddressProvider(tt.serverAddr, "")
			timeout := time.NewTicker(1 * time.Second)
			var got string
			if len(tt.want) == 0 {
				select {
				case got = <-provider.GetNacosAddress(""):
					t.Errorf("GetNacosAddress() = %v, want empty", got)
				case <-timeout.C:
					return
				}
			}
			select {
			case got = <-provider.GetNacosAddress(""):
			case <-timeout.C:
				t.Error("GetNacosAddress timeout")
			}
			for _, value := range tt.want {
				if got == value {
					return
				}
			}
			t.Errorf("GetNacosAddress() = %v, want %v", got, tt.want)
		})
	}
}

func TestTrigger(t *testing.T) {
	body := newMockBody("1.1.1.1 ")
	url, tearDown := setUpServerWithBody(200, body)
	defer tearDown()
	provider := NewNacosAddressProvider(url, "xxxx")
	address := <-provider.GetNacosAddress("")
	if address != "1.1.1.1" {
		t.Errorf("got %s, want %s", address, "1.1.1.1")
	}
	body.set(" 2.2.2.2 ")
	tests := []struct {
		name    string
		trigger bool
		want    string
	}{
		{
			"no trigger",
			false,
			"1.1.1.1",
		},
		{
			"trigger",
			true,
			"2.2.2.2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.trigger {
				provider.Trigger()
			}
			timeout := time.NewTicker(1 * time.Second)
			select {
			case <-provider.GetNacosAddress("1.1.1.1"):
			case <-timeout.C:
			}
			if got := nacosAddr(provider); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestBackup(t *testing.T) {
	body := newMockBody("1.1.1.1 ")
	url, tearDown := setUpServerWithBody(200, body)
	defer tearDown()
	provider := NewNacosAddressProvider(url, "xxxx")
	address := <-provider.GetNacosAddress("")
	if address != "1.1.1.1" {
		t.Errorf("got %s, want %s", address, "1.1.1.1")
	}
	tests := []struct {
		name       string
		oldaddr    string
		newaddr    string
		triggerNum int
		want       string
	}{
		{
			"case1",
			"1.1.1.1",
			"1.1.1.1\n2.2.2.2",
			1,
			"2.2.2.2",
		},
		{
			"case2",
			"1.1.1.1",
			"3.3.3.3 1.1.1.1",
			1,
			"3.3.3.3",
		},
		{
			"case3",
			"1.1.1.1",
			"3.3.3.3 1.1.1.1",
			2,
			"1.1.1.1",
		},
		{
			"case4",
			"1.1.1.1",
			"3.3.3.3\n 1.1.1.1",
			3,
			"3.3.3.3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setNacosAddr(provider, tt.oldaddr)
			body.set(tt.newaddr)
			provider.addressDiscovery()
			for i := 0; i < tt.triggerNum; i++ {
				provider.Trigger()
			}
			timeout := time.NewTicker(1 * time.Second)
			var newAddr string
			select {
			case newAddr = <-provider.GetNacosAddress(""):
			case <-timeout.C:
			}
			if newAddr != tt.want {
				t.Errorf("got %s, want %s", newAddr, tt.want)
			}
		})
	}
}

func TestAbandonedGetNacosAddressDoesNotBlockProvider(t *testing.T) {
	body := newMockBody("1.1.1.1")
	url, tearDown := setUpServerWithBody(200, body)
	defer tearDown()
	provider := NewNacosAddressProvider(url, "xxxx")
	if address := <-provider.GetNacosAddress(""); address != "1.1.1.1" {
		t.Fatalf("got %s, want %s", address, "1.1.1.1")
	}

	// A caller that stops waiting, as updateNacosClient does on stop.
	_ = provider.GetNacosAddress("1.1.1.1")
	body.set("2.2.2.2")
	provider.addressDiscovery()
	// Let the abandoned goroutine wake up and deliver the new address.
	time.Sleep(100 * time.Millisecond)

	got := make(chan string, 1)
	go func() { got <- nacosAddr(provider) }()
	select {
	case addr := <-got:
		if addr != "2.2.2.2" {
			t.Errorf("got %s, want %s", addr, "2.2.2.2")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("provider lock is held by an abandoned GetNacosAddress call")
	}
}

func TestKeepIp(t *testing.T) {
	body := newMockBody("1.1.1.1")
	url, tearDown := setUpServerWithBody(200, body)
	defer tearDown()
	provider := NewNacosAddressProvider(url, "xxxx")
	address := <-provider.GetNacosAddress("")
	if address != "1.1.1.1" {
		t.Errorf("got %s, want %s", address, "1.1.1.1")
	}
	tests := []struct {
		name    string
		newAddr string
		want    string
	}{
		{
			"add ip",
			"1.1.1.1\n 2.2.2.2",
			"1.1.1.1",
		},
		{
			"remove ip",
			"2.2.2.2",
			"2.2.2.2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body.set(tt.newAddr)
			provider.addressDiscovery()
			timeout := time.NewTicker(1 * time.Second)
			select {
			case <-provider.GetNacosAddress("1.1.1.1"):
			case <-timeout.C:
			}
			if got := nacosAddr(provider); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMultiClient(t *testing.T) {
	body := newMockBody("1.1.1.1")
	url, tearDown := setUpServerWithBody(200, body)
	defer tearDown()
	provider := NewNacosAddressProvider(url, "xxxx")
	address := <-provider.GetNacosAddress("")
	if address != "1.1.1.1" {
		t.Errorf("got %s, want %s", address, "1.1.1.1")
	}
	body.set("2.2.2.2")
	tests := []struct {
		name     string
		oldAddrs []string
		want     []string
	}{
		{
			"case1",
			[]string{"1.1.1.1", "1.1.1.1"},
			[]string{"2.2.2.2", "2.2.2.2"},
		},
		{
			"case2",
			[]string{"2.2.2.2", "1.1.1.1"},
			[]string{"", "2.2.2.2"},
		},
		{
			"case3",
			[]string{"1.1.1.1", "2.2.2.2"},
			[]string{"2.2.2.2", ""},
		},
		{
			"case4",
			[]string{"2.2.2.2", "2.2.2.2"},
			[]string{"", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider.addressDiscovery()
			for i := 0; i < len(tt.oldAddrs); i++ {
				timeout := time.NewTicker(1 * time.Second)
				var newaddr string
				select {
				case newaddr = <-provider.GetNacosAddress(tt.oldAddrs[i]):
				case <-timeout.C:
				}
				if newaddr != tt.want[i] {
					t.Errorf("got %s, want %s", newaddr, tt.want[i])
				}
			}
		})
	}
}
