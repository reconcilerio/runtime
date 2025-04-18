/*
Copyright 2023 the original author or authors.

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
	"errors"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/apis"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	"reconciler.io/runtime/validation"
)

func TestSuppressTransientErrors(t *testing.T) {
	testNamespace := "test-namespace"

	now := metav1.Now()

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resourceBlue := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name("blue")
			d.Generation(1)
			d.UID("11111111-1111-1111-1111-111111111111")
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})
	resourceGreen := resourceBlue.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Name("green")
			d.UID("22222222-2222-2222-2222-222222222222")
		})

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"no error continues": {
			Resource: resourceBlue.DieReleasePtr(),
			Now:      now.Time,
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return nil
							},
						},
					}
				},
			},
		},
		"transient error suppressed": {
			Resource: resourceBlue.DieReleasePtr(),
			Now:      now.Time,
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return fmt.Errorf("an error")
							},
						},
					}
				},
			},
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
					t.Errorf("error expected to be ErrSkipStatusUpdate")
				}
			},
			ShouldErr: true,
		},
		"transient error suppressed until threshold": {
			Resource: resourceBlue.DieReleasePtr(),
			Now:      now.Time,
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return fmt.Errorf("an error")
							},
						},
					}
				},
			},
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
					t.Errorf("error expected to be ErrSkipStatusUpdate")
				}
			},
			ShouldErr: true,
			AdditionalReconciles: []rtesting.SubReconcilerTestCase[*resources.TestResource]{
				{
					Name:     "still below threshold",
					Resource: resourceBlue.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name:     "at threshold",
					Resource: resourceBlue.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to not be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name:     "above threshold",
					Resource: resourceBlue.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to not be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
			},
		},
		"transient error suppressed until threshold, per resource": {
			Resource: resourceBlue.DieReleasePtr(),
			Now:      now.Time,
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return fmt.Errorf("an error")
							},
						},
					}
				},
			},
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
					t.Errorf("error expected to be ErrSkipStatusUpdate")
				}
			},
			ShouldErr: true,
			AdditionalReconciles: []rtesting.SubReconcilerTestCase[*resources.TestResource]{
				{
					Name:     "start green",
					Resource: resourceGreen.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name:     "blue still below threshold",
					Resource: resourceBlue.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name:     "green still below threshold",
					Resource: resourceGreen.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name:     "blue at threshold",
					Resource: resourceBlue.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to not be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name:     "green at threshold",
					Resource: resourceGreen.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to not be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name:     "blue above threshold",
					Resource: resourceBlue.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to not be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name:     "green above threshold",
					Resource: resourceGreen.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to not be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
			},
		},
		"updated generation resets threshold": {
			Resource: resourceBlue.DieReleasePtr(),
			Now:      now.Time,
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return fmt.Errorf("an error")
							},
						},
					}
				},
			},
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
					t.Errorf("error expected to be ErrSkipStatusUpdate")
				}
			},
			ShouldErr: true,
			AdditionalReconciles: []rtesting.SubReconcilerTestCase[*resources.TestResource]{
				{
					Name:     "still below threshold",
					Resource: resourceBlue.DieReleasePtr(),
					Now:      now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name: "bumped generation, counter reset",
					Resource: resourceBlue.
						MetadataDie(func(d *diemetav1.ObjectMetaDie) {
							d.Generation(2)
						}).
						DieReleasePtr(),
					Now: now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name: "still below threshold",
					Resource: resourceBlue.
						MetadataDie(func(d *diemetav1.ObjectMetaDie) {
							d.Generation(2)
						}).
						DieReleasePtr(),
					Now: now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
				{
					Name: "bumped generation again, counter reset",
					Resource: resourceBlue.
						MetadataDie(func(d *diemetav1.ObjectMetaDie) {
							d.Generation(3)
						}).
						DieReleasePtr(),
					Now: now.Time,
					Verify: func(t *testing.T, result reconcilers.Result, err error) {
						if !errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
							t.Errorf("error expected to be ErrSkipStatusUpdate")
						}
					},
					ShouldErr: true,
				},
			},
		},
		"durable error not suppressed": {
			Resource: resourceBlue.DieReleasePtr(),
			Now:      now.Time,
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return reconcilers.ErrDurable
							},
						},
					}
				},
			},
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if errors.Is(err, reconcilers.ErrSkipStatusUpdate) {
					t.Errorf("error expected to not be ErrSkipStatusUpdate")
				}
			},
			ShouldErr: true,
		},
		"purge stale counters": {
			Resource: resourceBlue.DieReleasePtr(),
			Now:      now.Time.Add(-25 * time.Hour),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return reconcilers.ErrQuiet
							},
						},
					}
				},
			},
			ShouldErr: true,
			AdditionalReconciles: []rtesting.SubReconcilerTestCase[*resources.TestResource]{
				{
					Name:      "other resource",
					Resource:  resourceGreen.DieReleasePtr(),
					Now:       now.Time.Add(-25 * time.Hour),
					ShouldErr: true,
				},
				{
					Name:      "purge",
					Resource:  resourceGreen.DieReleasePtr(),
					Now:       now.Time,
					ShouldErr: true,
				},
			},
		},
		"purge stale counters fails": {
			Resource: resourceBlue.DieReleasePtr(),
			Now:      now.Time.Add(-25 * time.Hour),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return reconcilers.ErrQuiet
							},
						},
					}
				},
			},
			ShouldErr: true,
			AdditionalReconciles: []rtesting.SubReconcilerTestCase[*resources.TestResource]{
				{
					Name:      "other resource",
					Resource:  resourceGreen.DieReleasePtr(),
					Now:       now.Time.Add(-25 * time.Hour),
					ShouldErr: true,
				},
				{
					Name:     "purge",
					Resource: resourceGreen.DieReleasePtr(),
					Now:      now.Time,
					WithReactors: []rtesting.ReactionFunc{
						rtesting.InduceFailure("list", "TestResourceList"),
					},
					ShouldErr: true,
				},
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		return rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource])(t, c)
	})
}

func TestSuppressTransientErrors_Validate(t *testing.T) {
	tests := []struct {
		name       string
		parent     *resources.TestResource
		reconciler *reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]
		shouldErr  string
	}{
		{
			name:       "empty",
			parent:     &resources.TestResource{},
			reconciler: &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{},
			shouldErr:  `SuppressTransientErrors "SuppressTransientErrors" must implement Reconciler`,
		},
		{
			name:   "valid",
			parent: &resources.TestResource{},
			reconciler: &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
				Reconciler: &reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name:   "Reconciler missing",
			parent: &resources.TestResource{},
			reconciler: &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
				Name: "Reconciler missing",
				// Reconciler: &reconcilers.Sequence[*resources.TestResource]{},
			},
			shouldErr: `SuppressTransientErrors "Reconciler missing" must implement Reconciler`,
		},
		{
			name:   "Reconciler invalid",
			parent: &resources.TestResource{},
			reconciler: &reconcilers.SuppressTransientErrors[*resources.TestResource, *resources.TestResourceList]{
				Name:       "Reconciler invalid",
				Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{},
			},
			shouldErr: `SuppressTransientErrors "Reconciler invalid" must have a valid Reconciler: SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			ctx := reconcilers.StashResourceType(context.TODO(), c.parent)
			ctx = validation.WithRecursive(ctx)
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err.Error(), c.shouldErr)
			}
		})
	}
}
