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

func TestAlways(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

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
		"sub reconciler erred, keeps processing": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return fmt.Errorf("reconciler error")
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								if resource.Status.Fields == nil {
									resource.Status.Fields = map[string]string{}
								}
								resource.Status.Fields["still-running"] = "true"
								return nil
							},
						},
					}
				},
			},

			ShouldErr: true,
			ExpectResource: resource.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("still-running", "true")
				}).
				DieReleasePtr(),
		},
		"non-quiet errors remain non-quiet": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return fmt.Errorf("not quiet")
							},
						}, &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return nil
							},
						},
					}
				},
			},
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if errors.Is(err, reconcilers.ErrQuiet) {
					t.Errorf("expected returned error to not be ErrQuiet")
				}
			},
		},
		"mixed quiet and non-quiet errors are not quiet": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return fmt.Errorf("not quiet")
							},
						}, &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return reconcilers.ErrHaltSubReconcilers
							},
						},
					}
				},
			},
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if errors.Is(err, reconcilers.ErrQuiet) {
					t.Errorf("expected returned error to not be ErrQuiet")
				}
			},
		},
		"quiet errors remain quiet": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{}, reconcilers.ErrHaltSubReconcilers
							},
						}, &reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 1 * time.Minute}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{RequeueAfter: 1 * time.Minute},
			ShouldErr:      true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if !errors.Is(err, reconcilers.ErrQuiet) {
					t.Errorf("expected returned error to be ErrQuiet")
				}
			},
		},
		"preserves result, Requeue": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
							return reconcilers.Result{Requeue: true}, nil
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{Requeue: true},
		},
		"preserves result, RequeueAfter": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 1 * time.Minute}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{RequeueAfter: 1 * time.Minute},
		},
		"ignores result on err": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{Requeue: true}, fmt.Errorf("test error")
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{},
			ShouldErr:      true,
		},
		"Requeue + empty => Requeue": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{Requeue: true}, nil
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{Requeue: true},
		},
		"empty + Requeue => Requeue": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{}, nil
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{Requeue: true}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{Requeue: true},
		},
		"RequeueAfter + empty => RequeueAfter": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 1 * time.Minute}, nil
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{RequeueAfter: 1 * time.Minute},
		},
		"empty + RequeueAfter => RequeueAfter": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{}, nil
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 1 * time.Minute}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{RequeueAfter: 1 * time.Minute},
		},
		"RequeueAfter + Requeue => RequeueAfter": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 1 * time.Minute}, nil
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{Requeue: true}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{RequeueAfter: 1 * time.Minute},
		},
		"Requeue + RequeueAfter => RequeueAfter": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{Requeue: true}, nil
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 1 * time.Minute}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{RequeueAfter: 1 * time.Minute},
		},
		"RequeueAfter(1m) + RequeueAfter(2m) => RequeueAfter(1m)": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 1 * time.Minute}, nil
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 2 * time.Minute}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{RequeueAfter: 1 * time.Minute},
		},
		"RequeueAfter(2m) + RequeueAfter(1m) => RequeueAfter(1m)": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Always[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 2 * time.Minute}, nil
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								return reconcilers.Result{RequeueAfter: 1 * time.Minute}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{RequeueAfter: 1 * time.Minute},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		return rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource])(t, c)
	})
}

func TestAlways_Validate(t *testing.T) {
	tests := []struct {
		name       string
		reconciler *reconcilers.Always[*resources.TestResource]
		shouldErr  string
	}{
		{
			name:       "valid empty sequence",
			reconciler: &reconcilers.Always[*resources.TestResource]{},
		},
		{
			name: "valid sequence",
			reconciler: &reconcilers.Always[*resources.TestResource]{
				&reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
				&reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
			},
		},
		{
			name: "invalid sequence",
			reconciler: &reconcilers.Always[*resources.TestResource]{
				&reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
				&reconcilers.SyncReconciler[*resources.TestResource]{
					// Sync: func(ctx context.Context, resource *resources.TestResource) error {
					// 	return nil
					// },
				},
			},
			shouldErr: `Always must have a valid Always[1]: SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			ctx := validation.WithRecursive(context.TODO())
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
		})
	}
}
