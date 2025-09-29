/*
Copyright 2025 the original author or authors.

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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"reconciler.io/runtime/reconcilers"
	rtime "reconciler.io/runtime/time"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcilerTestCase_Now(t *testing.T) {
	start := time.Now()

	type testCase struct {
		name   string
		rtc    *ReconcilerTestCase
		verify func(*testing.T, context.Context)
	}
	tests := []*testCase{
		{
			name: "Now defaults to time.Now()",
			rtc:  &ReconcilerTestCase{},
			verify: func(t *testing.T, ctx context.Context) {
				now := rtime.RetrieveNow(ctx)
				// compare with a range to allow for time skew
				// test must complete within 1 hour of wall clock time
				if now.Before(start.Add(-1*time.Second)) || now.After(start.Add(time.Hour)) {
					t.Error("expected test case to be initialized with time.Now()")
				}
			},
		},
		{
			name: "Now is set explicitly",
			rtc: &ReconcilerTestCase{
				Now: time.UnixMilli(1000),
			},
			verify: func(t *testing.T, ctx context.Context) {
				now := rtime.RetrieveNow(ctx)
				if !now.Equal(time.UnixMilli(1000)) {
					t.Error("expected time to be initialized from the test case")
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.rtc.ExpectedResult = reconcile.Result{RequeueAfter: time.Second}
			tc.rtc.Run(t, runtime.NewScheme(), func(t *testing.T, rtc *ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
				return reconcile.Func(func(ctx context.Context, o reconcile.Request) (reconcile.Result, error) {
					tc.verify(t, ctx)
					return reconcile.Result{RequeueAfter: time.Second}, nil
				})
			})
		})
	}
}
