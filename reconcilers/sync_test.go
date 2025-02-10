/*
Copyright 2020 the original author or authors.

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

package reconcilers_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/apis"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestSyncReconciler(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testFinalizer := "test.finalizer"

	now := metav1.Now()

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"sync success": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return nil
						},
					}
				},
			},
		},
		"sync with result halted": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							return reconcilers.Result{Requeue: true}, reconcilers.ErrHaltSubReconcilers
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{Requeue: true},
			ShouldErr:      true,
		},
		"sync error": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return fmt.Errorf("syncreconciler error")
						},
					}
				},
			},
			ShouldErr: true,
		},
		"should not finalize non-deleted resources": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							return reconcilers.Result{RequeueAfter: 2 * time.Hour}, nil
						},
						FinalizeWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							t.Errorf("reconciler should not call finalize for non-deleted resources")
							return reconcilers.Result{RequeueAfter: 3 * time.Hour}, nil
						},
					}
				},
			},
			ExpectedResult: reconcile.Result{RequeueAfter: 2 * time.Hour},
		},
		"should finalize deleted resources": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(&now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							t.Errorf("reconciler should not call sync for deleted resources")
							return reconcilers.Result{RequeueAfter: 2 * time.Hour}, nil
						},
						FinalizeWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							return reconcilers.Result{RequeueAfter: 3 * time.Hour}, nil
						},
					}
				},
			},
			ExpectedResult: reconcile.Result{RequeueAfter: 3 * time.Hour},
		},
		"finalize can halt subreconcilers": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(&now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							t.Errorf("reconciler should not call sync for deleted resources")
							return reconcilers.Result{RequeueAfter: 2 * time.Hour}, nil
						},
						FinalizeWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							return reconcilers.Result{RequeueAfter: 3 * time.Hour}, reconcilers.ErrHaltSubReconcilers
						},
					}
				},
			},
			ExpectedResult: reconcile.Result{RequeueAfter: 3 * time.Hour},
			ShouldErr:      true,
		},
		"should finalize and sync deleted resources when asked to": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(&now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						SyncDuringFinalization: true,
						SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							return reconcilers.Result{RequeueAfter: 2 * time.Hour}, nil
						},
						FinalizeWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							return reconcilers.Result{RequeueAfter: 3 * time.Hour}, nil
						},
					}
				},
			},
			ExpectedResult: reconcile.Result{RequeueAfter: 2 * time.Hour},
		},
		"should finalize and sync deleted resources when asked to, shorter resync wins": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(&now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						SyncDuringFinalization: true,
						SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							return reconcilers.Result{RequeueAfter: 3 * time.Hour}, nil
						},
						FinalizeWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							return reconcilers.Result{RequeueAfter: 2 * time.Hour}, nil
						},
					}
				},
			},
			ExpectedResult: reconcile.Result{RequeueAfter: 2 * time.Hour},
		},
		"finalize is optional": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(&now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return nil
						},
					}
				},
			},
		},
		"finalize error": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(&now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return nil
						},
						Finalize: func(ctx context.Context, resource *resources.TestResource) error {
							return fmt.Errorf("syncreconciler finalize error")
						},
					}
				},
			},
			ShouldErr: true,
		},
		"context can be augmented in Prepare and accessed in Cleanup": {
			Resource: resource.DieReleasePtr(),
			Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.SubReconcilerTestCase[*resources.TestResource]) (context.Context, error) {
				key := "test-key"
				value := "test-value"
				ctx = context.WithValue(ctx, key, value)

				tc.Metadata["SubReconciler"] = func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if v := ctx.Value(key); v != value {
								t.Errorf("expected %s to be in context", key)
							}
							return nil
						},
					}
				}
				tc.CleanUp = func(t *testing.T, ctx context.Context, tc *rtesting.SubReconcilerTestCase[*resources.TestResource]) error {
					if v := ctx.Value(key); v != value {
						t.Errorf("expected %s to be in context", key)
					}
					return nil
				}

				return ctx, nil
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		return rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource])(t, c)
	})
}

func TestSyncReconcilerDuck(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	// _ = resources.AddToScheme(scheme)

	resource := dies.TestDuckBlank.
		APIVersion(resources.GroupVersion.String()).
		Kind("TestResource").
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		SpecDie(func(d *dies.TestDuckSpecDie) {
			d.AddField("mutation", "false")
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})

	rts := rtesting.SubReconcilerTests[*resources.TestDuck]{
		"sync no mutation": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							return nil
						},
					}
				},
			},
		},
		"sync with mutation": {
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestDuckSpecDie) {
					d.AddField("mutation", "true")
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							resource.Spec.Fields["mutation"] = "true"
							return nil
						},
					}
				},
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestDuck], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
		return rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck])(t, c)
	})
}

func TestSyncReconciler_Validate(t *testing.T) {
	tests := []struct {
		name       string
		resource   client.Object
		reconciler *reconcilers.SyncReconciler[*corev1.ConfigMap]
		shouldErr  string
	}{
		{
			name:       "empty",
			resource:   &corev1.ConfigMap{},
			reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{},
			shouldErr:  `SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
		{
			name:     "valid",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
				Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
					return nil
				},
			},
		},
		{
			name:     "valid SyncWithResult",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
				SyncWithResult: func(ctx context.Context, resource *corev1.ConfigMap) (reconcilers.Result, error) {
					return reconcilers.Result{}, nil
				},
			},
		},
		{
			name:     "valid",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
				Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
					return nil
				},
			},
		},
		{
			name:     "invalid Sync and SyncWithResult",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
				Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
					return nil
				},
				SyncWithResult: func(ctx context.Context, resource *corev1.ConfigMap) (reconcilers.Result, error) {
					return reconcilers.Result{}, nil
				},
			},
			shouldErr: `SyncReconciler "SyncReconciler" may not implement both Sync and SyncWithResult`,
		},
		{
			name:     "valid Finalize",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
				Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
					return nil
				},
				Finalize: func(ctx context.Context, resource *corev1.ConfigMap) error {
					return nil
				},
			},
		},
		{
			name:     "valid Finalize with result",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
				Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
					return nil
				},
				FinalizeWithResult: func(ctx context.Context, resource *corev1.ConfigMap) (reconcilers.Result, error) {
					return reconcilers.Result{}, nil
				},
			},
		},
		{
			name:     "invalid Finalize and FinalizeWithResult",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
				Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
					return nil
				},
				Finalize: func(ctx context.Context, resource *corev1.ConfigMap) error {
					return nil
				},
				FinalizeWithResult: func(ctx context.Context, resource *corev1.ConfigMap) (reconcilers.Result, error) {
					return reconcilers.Result{}, nil
				},
			},
			shouldErr: `SyncReconciler "SyncReconciler" may not implement both Finalize and FinalizeWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			ctx := reconcilers.StashResourceType(context.TODO(), c.resource)
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
		})
	}
}
