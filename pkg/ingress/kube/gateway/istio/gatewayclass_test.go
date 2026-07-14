// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package istio

import (
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/alibaba/higress/v2/pkg/config/constants"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/test/util/retry"
)

func TestClassController(t *testing.T) {
	setGatewayClassNameForTest(t, "")
	client := kube.NewFakeClient()
	cc := NewClassController(client)
	classes := clienttest.Wrap(t, cc.classes)
	stop := test.NewStop(t)
	client.RunAndWait(stop)
	go cc.Run(stop)
	createClass := func(name, controller string) {
		gc := &gateway.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: gateway.GatewayClassSpec{
				ControllerName: gateway.GatewayController(controller),
			},
		}
		classes.CreateOrUpdate(gc)
	}
	deleteClass := func(name string) {
		classes.Delete(name, "")
	}
	expectClass := func(name, controller string) {
		t.Helper()
		retry.UntilSuccessOrFail(t, func() error {
			gc := classes.Get(name, "")
			if controller == "" {
				if gc == nil { // Expect none, got none
					return nil
				}
				return fmt.Errorf("expected no class, got %v", gc.Spec.ControllerName)
			}
			if gc == nil {
				return fmt.Errorf("expected class %v, got none", controller)
			}
			if gateway.GatewayController(controller) != gc.Spec.ControllerName {
				return fmt.Errorf("expected class %v, got %v", controller, gc.Spec.ControllerName)
			}
			return nil
		}, retry.Timeout(time.Second*3))
	}

	// Class should be created initially
	expectClass(constants.DefaultGatewayClass, constants.ManagedGatewayController)

	// Once we delete it, it should be added back
	deleteClass(constants.DefaultGatewayClass)
	expectClass(constants.DefaultGatewayClass, constants.ManagedGatewayController)

	// Overwrite the class, controller should not reconcile it back
	createClass(constants.DefaultGatewayClass, "different-controller")
	expectClass(constants.DefaultGatewayClass, "different-controller")

	// Once we delete it, it should be added back
	deleteClass(constants.DefaultGatewayClass)
	expectClass(constants.DefaultGatewayClass, constants.ManagedGatewayController)

	// Create an unrelated GatewayClass, we should not do anything to it
	createClass("something-else", "different-controller")
	expectClass("something-else", "different-controller")
	deleteClass("something-else")
	expectClass("something-else", "")
}

func TestClassControllerWithCustomGatewayClass(t *testing.T) {
	if runInGatewayClassSubprocess(t) {
		return
	}
	gatewayClass := "higress-internal"
	setGatewayClassNameForTest(t, gatewayClass)
	client := kube.NewFakeClient()
	controllerName := string(gateway.GatewayController(constants.ManagedGatewayController + "-" + gatewayClass))
	cc := NewClassController(client)
	classes := clienttest.Wrap(t, cc.classes)
	stop := test.NewStop(t)
	client.RunAndWait(stop)
	go cc.Run(stop)

	expectClass := func(name, controller string) {
		t.Helper()
		retry.UntilSuccessOrFail(t, func() error {
			gc := classes.Get(name, "")
			if controller == "" {
				if gc == nil {
					return nil
				}
				return fmt.Errorf("expected no class, got %v", gc.Spec.ControllerName)
			}
			if gc == nil {
				return fmt.Errorf("expected class %v, got none", controller)
			}
			if gateway.GatewayController(controller) != gc.Spec.ControllerName {
				return fmt.Errorf("expected class %v, got %v", controller, gc.Spec.ControllerName)
			}
			return nil
		}, retry.Timeout(time.Second*3))
	}

	expectClass(gatewayClass, controllerName)
	expectClass(constants.DefaultGatewayClass, "")
}

func TestSetGatewayClassName(t *testing.T) {
	if runInGatewayClassSubprocess(t) {
		return
	}
	SetGatewayClassName("")
	if gatewayClassName != gateway.ObjectName(constants.DefaultGatewayClass) {
		t.Fatalf("expected default gateway class %q, got %q", constants.DefaultGatewayClass, gatewayClassName)
	}
	if managedGatewayController != gateway.GatewayController(constants.ManagedGatewayController) {
		t.Fatalf("expected default controller %q, got %q", constants.ManagedGatewayController, managedGatewayController)
	}

	customClass := "higress-internal"
	SetGatewayClassName(customClass)
	customController := gateway.GatewayController(constants.ManagedGatewayController + "-" + customClass)
	if gatewayClassName != gateway.ObjectName(customClass) {
		t.Fatalf("expected custom gateway class %q, got %q", customClass, gatewayClassName)
	}
	if managedGatewayController != customController {
		t.Fatalf("expected custom controller %q, got %q", customController, managedGatewayController)
	}
	if got := builtinClasses[gateway.ObjectName(customClass)]; got != customController {
		t.Fatalf("expected builtin class controller %q, got %q", customController, got)
	}
	if _, exists := builtinClasses[gateway.ObjectName(constants.DefaultGatewayClass)]; exists {
		t.Fatalf("custom config should not include default gateway class %q", constants.DefaultGatewayClass)
	}
}
