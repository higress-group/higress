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
	"sync/atomic"
	"testing"
	"time"

	"github.com/hudl/fargo"
)

const planTestTimeout = 5 * time.Second

func TestPlanWatchReturnsWhenUpdatesChannelIsClosed(t *testing.T) {
	var handlerCalls atomic.Int32
	plan, updates, watchDone := startPlanWatch(func(*fargo.Application) error {
		handlerCalls.Add(1)
		return nil
	})

	close(updates)
	waitForPlanWatch(t, watchDone)
	plan.Stop()

	if got := handlerCalls.Load(); got != 0 {
		t.Fatalf("expected handler not to be called, got %d calls", got)
	}
}

func TestPlanWatchSkipsNilApplication(t *testing.T) {
	handled := make(chan *fargo.Application, 2)
	plan, updates, watchDone := startPlanWatch(func(application *fargo.Application) error {
		handled <- application
		return nil
	})

	updates <- fargo.AppUpdate{}
	expected := &fargo.Application{Name: "test-service"}
	updates <- fargo.AppUpdate{App: expected}
	close(updates)
	waitForPlanWatch(t, watchDone)
	plan.Stop()

	select {
	case actual := <-handled:
		if actual != expected {
			t.Fatalf("expected handler to receive %p, got %p", expected, actual)
		}
	default:
		t.Fatal("expected handler to receive the valid application")
	}

	select {
	case unexpected := <-handled:
		t.Fatalf("expected handler to be called once, got an extra application: %#v", unexpected)
	default:
	}
}

func TestPlanStopIsConcurrentAndIdempotent(t *testing.T) {
	plan, updates, watchDone := startPlanWatch(func(*fargo.Application) error {
		return nil
	})

	const callers = 32
	start := make(chan struct{})
	var callersDone sync.WaitGroup
	callersDone.Add(callers)
	for range callers {
		go func() {
			defer callersDone.Done()
			<-start
			plan.Stop()
		}()
	}

	close(start)
	stopDone := make(chan struct{})
	go func() {
		callersDone.Wait()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(planTestTimeout):
		t.Fatal("concurrent Stop calls did not return")
	}
	waitForPlanWatch(t, watchDone)

	// A later Stop call must also be safe and return immediately.
	plan.Stop()
	close(updates)
}

func TestScheduleAppUpdatesClosesUpdatesAfterStop(t *testing.T) {
	client := &eurekaHttpClient{EurekaHttpConfig: EurekaHttpConfig{PollInterval: 3600}}
	stop := make(chan struct{})
	updates := client.ScheduleAppUpdates("test-service", stop)

	close(stop)
	select {
	case _, ok := <-updates:
		if ok {
			t.Fatal("expected updates channel to be closed")
		}
	case <-time.After(planTestTimeout):
		t.Fatal("ScheduleAppUpdates did not stop and close its updates channel")
	}
}

func startPlanWatch(handler Handler) (*Plan, chan fargo.AppUpdate, <-chan struct{}) {
	plan := &Plan{
		stop:    make(chan struct{}),
		handler: handler,
	}
	updates := make(chan fargo.AppUpdate)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		plan.watch(updates)
	}()
	return plan, updates, watchDone
}

func waitForPlanWatch(t *testing.T, watchDone <-chan struct{}) {
	t.Helper()
	select {
	case <-watchDone:
	case <-time.After(planTestTimeout):
		t.Fatal("Plan watch goroutine did not return")
	}
}
