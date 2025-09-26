/*
Copyright 2019 the original author or authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testing

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"reconciler.io/runtime/reconcilers"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcilerTestCase_Run(t *testing.T) {
	type testCase struct {
		name      string
		verify    func(*testing.T, *ReconcilerTestCase, *testCase)
		verifyRan bool
	}
	tests := []*testCase{
		{
			name: "Now defaults to time.Now()",
			verify: func(t *testing.T, rtc *ReconcilerTestCase, tc *testCase) {
				tc.verifyRan = true
				if rtc.Now.IsZero() {
					t.Error("expected test case to be initialized with time.Now()")
				}

			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rtc := &ReconcilerTestCase{}
			if tc.verify != nil {
				rtc.Prepare = func(t *testing.T, ctx context.Context, rtc *ReconcilerTestCase) (context.Context, error) {
					tc.verify(t, rtc, tc)
					return ctx, nil
				}
			}
			rtc.Run(t, runtime.NewScheme(), func(t *testing.T, rtc *ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
				return reconcile.Func(func(ctx context.Context, o reconcile.Request) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				})
			})

			if !tc.verifyRan {
				t.Fatal("expected verify to have run")
			}
		})
	}
}
