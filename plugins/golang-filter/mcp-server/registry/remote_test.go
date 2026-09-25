// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package registry

import "testing"

func TestSelectOneInstanceCanSelectEveryBackend(t *testing.T) {
	instances := []Instance{
		{Host: "backend-0", Port: 8080},
		{Host: "backend-1", Port: 8080},
	}
	ctx := &RpcContext{Instances: &instances}

	// With the old Intn(len(instances)-1) bound and two backends, backend-1
	// cannot be selected at all. Repeating the fixed input makes that regression
	// deterministic while making a false failure after the fix negligible.
	selected := make(map[string]bool, len(instances))
	for i := 0; i < 100; i++ {
		instance, err := selectOneInstance(ctx)
		if err != nil {
			t.Fatalf("selectOneInstance() error = %v", err)
		}
		selected[instance.Host] = true
	}

	for _, instance := range instances {
		if !selected[instance.Host] {
			t.Errorf("backend %q was never selected", instance.Host)
		}
	}
}
