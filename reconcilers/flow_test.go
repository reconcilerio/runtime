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
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	diecorev1 "reconciler.io/dies/apis/core/v1"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	"reconciler.io/runtime/validation"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestIfThen(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		SpecDie(func(d *dies.TestResourceSpecDie) {
			d.Fields(map[string]string{})
		})

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"then": {
			Metadata: map[string]interface{}{
				"Condition": true,
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("then", "called")
				}).
				DieReleasePtr(),
			ExpectedResult: reconcile.Result{RequeueAfter: 1},
		},
		"then error": {
			Metadata: map[string]interface{}{
				"Condition": true,
				"ThenError": fmt.Errorf("then error"),
				"ElseError": fmt.Errorf("else error"),
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("then", "called")
				}).
				DieReleasePtr(),
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if result != (reconcile.Result{RequeueAfter: 1}) {
					t.Errorf("unexpected result: %v", result)
				}
				if err.Error() != "then error" {
					t.Errorf("unexpected error: %s", err)
				}
			},
		},
		"else": {
			Metadata: map[string]interface{}{
				"Condition": false,
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("else", "called")
				}).
				DieReleasePtr(),
			ExpectedResult: reconcile.Result{RequeueAfter: 2},
		},
		"else error": {
			Metadata: map[string]interface{}{
				"Condition": false,
				"ThenError": fmt.Errorf("then error"),
				"ElseError": fmt.Errorf("else error"),
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("else", "called")
				}).
				DieReleasePtr(),
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if result != (reconcile.Result{RequeueAfter: 2}) {
					t.Errorf("unexpected result: %v", result)
				}
				if err.Error() != "else error" {
					t.Errorf("unexpected error: %s", err)
				}
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		return &reconcilers.IfThen[*resources.TestResource]{
			If: func(ctx context.Context, resource *resources.TestResource) bool {
				return rtc.Metadata["Condition"].(bool)
			},
			Then: &reconcilers.SyncReconciler[*resources.TestResource]{
				SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
					resource.Spec.Fields["then"] = "called"
					var thenErr error
					if err, ok := rtc.Metadata["ThenError"]; ok {
						thenErr = err.(error)
					}
					return reconcilers.Result{RequeueAfter: 1}, thenErr
				},
			},
			Else: &reconcilers.SyncReconciler[*resources.TestResource]{
				SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
					resource.Spec.Fields["else"] = "called"
					var elseErr error
					if err, ok := rtc.Metadata["ElseError"]; ok {
						elseErr = err.(error)
					}
					return reconcilers.Result{RequeueAfter: 2}, elseErr
				},
			},
		}
	})
}

func TestIfThen_Validate(t *testing.T) {
	tests := []struct {
		name           string
		reconciler     *reconcilers.IfThen[*resources.TestResource]
		validateNested bool
		shouldErr      string
		expectedLogs   []string
	}{
		{
			name: "valid",
			reconciler: &reconcilers.IfThen[*resources.TestResource]{
				If: func(ctx context.Context, resource *resources.TestResource) bool {
					return false
				},
				Then: reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name: "missing if",
			reconciler: &reconcilers.IfThen[*resources.TestResource]{
				Name: "missing if",
				Then: reconcilers.Sequence[*resources.TestResource]{},
			},
			shouldErr: `IfThen "missing if" must implement If`,
		},
		{
			name: "missing then",
			reconciler: &reconcilers.IfThen[*resources.TestResource]{
				Name: "missing then",
				If: func(ctx context.Context, resource *resources.TestResource) bool {
					return false
				},
			},
			shouldErr: `IfThen "missing then" must implement Then`,
		},
		{
			name: "with else",
			reconciler: &reconcilers.IfThen[*resources.TestResource]{
				If: func(ctx context.Context, resource *resources.TestResource) bool {
					return false
				},
				Then: reconcilers.Sequence[*resources.TestResource]{},
				Else: reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name: "valid then",
			reconciler: &reconcilers.IfThen[*resources.TestResource]{
				If: func(ctx context.Context, resource *resources.TestResource) bool {
					return false
				},
				Then: &reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name: "invalid then",
			reconciler: &reconcilers.IfThen[*resources.TestResource]{
				If: func(ctx context.Context, resource *resources.TestResource) bool {
					return false
				},
				Then: &reconcilers.SyncReconciler[*resources.TestResource]{
					// Sync: func(ctx context.Context, resource *resources.TestResource) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr:      `IfThen "IfThen" must have a valid Then: SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			sink := &bufferedSink{}
			ctx := logr.NewContext(context.TODO(), logr.New(sink))
			if c.validateNested {
				ctx = validation.WithRecursive(ctx)
			}
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
			if diff := cmp.Diff(c.expectedLogs, sink.Lines); diff != "" {
				t.Errorf("%s: unexpected logs (-expected, +actual): %s", c.name, diff)
			}
		})
	}
}

func TestWhile(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		SpecDie(func(d *dies.TestResourceSpecDie) {
			d.Fields(map[string]string{})
		})

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"return immediately": {
			Metadata: map[string]interface{}{
				"Iterations": 0,
			},
			Resource:       resource.DieReleasePtr(),
			ExpectResource: resource.DieReleasePtr(),
			ExpectedResult: reconcile.Result{RequeueAfter: 0},
		},
		"return after 10 iterations": {
			Metadata: map[string]interface{}{
				"Iterations": 10,
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("iterations", "10")
				}).
				DieReleasePtr(),
			ExpectedResult: reconcile.Result{RequeueAfter: 990},
		},
		"halt after default max iterations": {
			Metadata: map[string]interface{}{
				"Iterations": 1000,
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("iterations", "100")
				}).
				DieReleasePtr(),
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if result != (reconcile.Result{RequeueAfter: 900}) {
					t.Errorf("unexpected result: %v", result)
				}
				if err.Error() != "exceeded max iterations: 100" {
					t.Errorf("unexpected error: %s", err)
				}
			},
		},
		"return before custom max iterations": {
			Metadata: map[string]interface{}{
				"MaxIterations": 10,
				"Iterations":    5,
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("iterations", "5")
				}).
				DieReleasePtr(),
			ExpectedResult: reconcile.Result{RequeueAfter: 995},
		},
		"halt after custom max iterations": {
			Metadata: map[string]interface{}{
				"MaxIterations": 10,
				"Iterations":    1000,
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("iterations", "10")
				}).
				DieReleasePtr(),
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if result != (reconcile.Result{RequeueAfter: 990}) {
					t.Errorf("unexpected result: %v", result)
				}
				if err.Error() != "exceeded max iterations: 10" {
					t.Errorf("unexpected error: %s", err)
				}
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		var desiredIterations int
		if i, ok := rtc.Metadata["Iterations"]; ok {
			desiredIterations = i.(int)
		}

		r := &reconcilers.While[*resources.TestResource]{
			Condition: func(ctx context.Context, resource *resources.TestResource) bool {
				return desiredIterations != reconcilers.RetrieveIteration(ctx)
			},
			Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
				SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcile.Result, error) {
					i := reconcilers.RetrieveIteration(ctx) + 1
					resource.Spec.Fields["iterations"] = fmt.Sprintf("%d", i)
					return reconcile.Result{RequeueAfter: time.Duration(1000 - i)}, nil
				},
			},
		}
		if i, ok := rtc.Metadata["MaxIterations"]; ok {
			r.MaxIterations = ptr.To(i.(int))
		}

		return r
	})
}

func TestWhile_Validate(t *testing.T) {
	tests := []struct {
		name           string
		reconciler     *reconcilers.While[*resources.TestResource]
		shouldErr      string
		validateNested bool
		expectedLogs   []string
	}{
		{
			name: "valid",
			reconciler: &reconcilers.While[*resources.TestResource]{
				Condition: func(ctx context.Context, resource *resources.TestResource) bool {
					return false
				},
				Reconciler: reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name: "missing condition",
			reconciler: &reconcilers.While[*resources.TestResource]{
				Name:       "missing condition",
				Reconciler: reconcilers.Sequence[*resources.TestResource]{},
			},
			shouldErr: `While "missing condition" must implement Condition`,
		},
		{
			name: "missing reconciler",
			reconciler: &reconcilers.While[*resources.TestResource]{
				Name: "missing reconciler",
				Condition: func(ctx context.Context, resource *resources.TestResource) bool {
					return false
				},
			},
			shouldErr: `While "missing reconciler" must implement Reconciler`,
		},
		{
			name: "valid reconciler",
			reconciler: &reconcilers.While[*resources.TestResource]{
				Condition: func(ctx context.Context, resource *resources.TestResource) bool {
					return false
				},
				Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name: "invalid reconciler",
			reconciler: &reconcilers.While[*resources.TestResource]{
				Condition: func(ctx context.Context, resource *resources.TestResource) bool {
					return false
				},
				Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
					// Sync: func(ctx context.Context, resource *resources.TestResource) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr:      `While "While" must have a valid Reconciler: SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			sink := &bufferedSink{}
			ctx := logr.NewContext(context.TODO(), logr.New(sink))
			if c.validateNested {
				ctx = validation.WithRecursive(ctx)
			}
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
			if diff := cmp.Diff(c.expectedLogs, sink.Lines); diff != "" {
				t.Errorf("%s: unexpected logs (-expected, +actual): %s", c.name, diff)
			}
		})
	}
}

func TestForEach(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		SpecDie(func(d *dies.TestResourceSpecDie) {
			d.Fields(map[string]string{})
		})

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"empty items": {
			Resource:       resource.DieReleasePtr(),
			ExpectResource: resource.DieReleasePtr(),
		},
		"each container": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.TemplateDie(func(d *diecorev1.PodTemplateSpecDie) {
						d.SpecDie(func(d *diecorev1.PodSpecDie) {
							d.ContainerDie("hello", func(d *diecorev1.ContainerDie) {
								d.Image("world")
							})
							d.ContainerDie("foo", func(d *diecorev1.ContainerDie) {
								d.Image("bar")
							})
						})
					})
				}).
				DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.TemplateDie(func(d *diecorev1.PodTemplateSpecDie) {
						d.SpecDie(func(d *diecorev1.PodSpecDie) {
							d.ContainerDie("hello", func(d *diecorev1.ContainerDie) {
								d.Image("world")
							})
							d.ContainerDie("foo", func(d *diecorev1.ContainerDie) {
								d.Image("bar")
							})
						})
					})
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					// container.name -> container.image-index-length
					d.AddField("hello", "world-0-2")
					d.AddField("foo", "bar-1-2")
				}).
				DieReleasePtr(),
		},
		"terminate iteration on error": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.TemplateDie(func(d *diecorev1.PodTemplateSpecDie) {
						d.SpecDie(func(d *diecorev1.PodSpecDie) {
							d.ContainerDie("hello", func(d *diecorev1.ContainerDie) {
								d.Image("world")
							})
							d.ContainerDie("die", func(d *diecorev1.ContainerDie) {
								d.Image("die")
							})
							d.ContainerDie("foo", func(d *diecorev1.ContainerDie) {
								d.Image("bar")
							})
						})
					})
				}).
				DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.TemplateDie(func(d *diecorev1.PodTemplateSpecDie) {
						d.SpecDie(func(d *diecorev1.PodSpecDie) {
							d.ContainerDie("hello", func(d *diecorev1.ContainerDie) {
								d.Image("world")
							})
							d.ContainerDie("die", func(d *diecorev1.ContainerDie) {
								d.Image("die")
							})
							d.ContainerDie("foo", func(d *diecorev1.ContainerDie) {
								d.Image("bar")
							})
						})
					})
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					// container.name -> container.image-index-length
					d.AddField("hello", "world-0-3")
				}).
				DieReleasePtr(),
			ShouldErr: true,
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		r := &reconcilers.ForEach[*resources.TestResource, corev1.Container]{
			Items: func(ctx context.Context, resource *resources.TestResource) ([]corev1.Container, error) {
				return resource.Spec.Template.Spec.Containers, nil
			},
			Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
				Sync: func(ctx context.Context, resource *resources.TestResource) error {
					cursor := reconcilers.CursorStasher[corev1.Container]().RetrieveOrDie(ctx)

					if cursor.Item.Name == "die" {
						return fmt.Errorf("exit early")
					}

					if resource.Status.Fields == nil {
						resource.Status.Fields = map[string]string{}
					}
					resource.Status.Fields[cursor.Item.Name] = fmt.Sprintf("%s-%d-%d", cursor.Item.Image, cursor.Index, cursor.Length)

					return nil
				},
			},
		}

		return r
	})
}

func TestForEach_Validate(t *testing.T) {
	tests := []struct {
		name           string
		reconciler     *reconcilers.ForEach[*resources.TestResource, any]
		validateNested bool
		shouldErr      string
		expectedLogs   []string
	}{
		{
			name: "valid",
			reconciler: &reconcilers.ForEach[*resources.TestResource, any]{
				Items: func(ctx context.Context, resource *resources.TestResource) ([]any, error) {
					return nil, nil
				},
				Reconciler: reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name: "missing items",
			reconciler: &reconcilers.ForEach[*resources.TestResource, any]{
				// Items: func(ctx context.Context, resource *resources.TestResource) ([]any, error) {
				// 	return nil, nil
				// },
				Reconciler: reconcilers.Sequence[*resources.TestResource]{},
			},
			shouldErr: `ForEach "missing items" must implement Items`,
		},
		{
			name: "missing reconciler",
			reconciler: &reconcilers.ForEach[*resources.TestResource, any]{
				Items: func(ctx context.Context, resource *resources.TestResource) ([]any, error) {
					return nil, nil
				},
				// Reconciler: Sequence[*resources.TestResource]{},
			},
			shouldErr: `ForEach "missing reconciler" must implement Reconciler`,
		},
		{
			name: "valid reconciler",
			reconciler: &reconcilers.ForEach[*resources.TestResource, any]{
				Items: func(ctx context.Context, resource *resources.TestResource) ([]any, error) {
					return nil, nil
				},
				Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name: "invalid reconciler",
			reconciler: &reconcilers.ForEach[*resources.TestResource, any]{
				Items: func(ctx context.Context, resource *resources.TestResource) ([]any, error) {
					return nil, nil
				},
				Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
					// Sync: func(ctx context.Context, resource *resources.TestResource) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr:      `ForEach "invalid reconciler" must have a valid Reconciler: SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			sink := &bufferedSink{}
			ctx := logr.NewContext(context.TODO(), logr.New(sink))
			if c.validateNested {
				ctx = validation.WithRecursive(ctx)
			}
			r := c.reconciler
			r.Name = c.name
			err := r.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
			if diff := cmp.Diff(c.expectedLogs, sink.Lines); diff != "" {
				t.Errorf("%s: unexpected logs (-expected, +actual): %s", c.name, diff)
			}
		})
	}
}

func TestTryCatch(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		SpecDie(func(d *dies.TestResourceSpecDie) {
			d.Fields(map[string]string{})
		})

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"catch rethrow": {
			Metadata: map[string]interface{}{
				"TryError": fmt.Errorf("try"),
				"Catch": func(ctx context.Context, resource *resources.TestResource, result reconcile.Result, err error) (reconcile.Result, error) {
					resource.Spec.Fields["catch"] = "called"
					return result, err
				},
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("try", "called")
					d.AddField("catch", "called")
				}).
				DieReleasePtr(),
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if result != (reconcile.Result{RequeueAfter: 3}) {
					t.Errorf("unexpected result: %v", result)
				}
				if err.Error() != "try" {
					t.Errorf("unexpected error: %s", err)
				}
			},
		},
		"catch and suppress error": {
			Metadata: map[string]interface{}{
				"TryError": fmt.Errorf("try"),
				"Catch": func(ctx context.Context, resource *resources.TestResource, result reconcile.Result, err error) (reconcile.Result, error) {
					resource.Spec.Fields["catch"] = "called"
					return result, nil
				},
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("try", "called")
					d.AddField("catch", "called")
				}).
				DieReleasePtr(),
			ExpectedResult: reconcile.Result{RequeueAfter: 3},
		},
		"catch and inject error": {
			Metadata: map[string]interface{}{
				"Catch": func(ctx context.Context, resource *resources.TestResource, result reconcile.Result, err error) (reconcile.Result, error) {
					resource.Spec.Fields["catch"] = "called"
					return reconcile.Result{RequeueAfter: 2}, fmt.Errorf("catch")
				},
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("try", "called")
					d.AddField("catch", "called")
				}).
				DieReleasePtr(),
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if result != (reconcile.Result{RequeueAfter: 2}) {
					t.Errorf("unexpected result: %v", result)
				}
				if err.Error() != "catch" {
					t.Errorf("unexpected error: %s", err)
				}
			},
		},
		"finally": {
			Metadata: map[string]interface{}{
				"Catch": func(ctx context.Context, resource *resources.TestResource, result reconcile.Result, err error) (reconcile.Result, error) {
					resource.Spec.Fields["catch"] = "called"
					return result, err
				},
				"Finally": &reconcilers.SyncReconciler[*resources.TestResource]{
					SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcile.Result, error) {
						resource.Spec.Fields["finally"] = "called"
						return reconcile.Result{RequeueAfter: 1}, nil
					},
				},
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("try", "called")
					d.AddField("catch", "called")
					d.AddField("finally", "called")
				}).
				DieReleasePtr(),
			ExpectedResult: reconcile.Result{RequeueAfter: 1},
		},
		"finally with try error": {
			Metadata: map[string]interface{}{
				"TryError": fmt.Errorf("try"),
				"Catch": func(ctx context.Context, resource *resources.TestResource, result reconcile.Result, err error) (reconcile.Result, error) {
					resource.Spec.Fields["catch"] = "called"
					return result, err
				},
				"Finally": &reconcilers.SyncReconciler[*resources.TestResource]{
					SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcile.Result, error) {
						resource.Spec.Fields["finally"] = "called"
						return reconcile.Result{RequeueAfter: 1}, nil
					},
				},
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("try", "called")
					d.AddField("catch", "called")
					d.AddField("finally", "called")
				}).
				DieReleasePtr(),
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if result != (reconcile.Result{RequeueAfter: 1}) {
					t.Errorf("unexpected result: %v", result)
				}
				if err.Error() != "try" {
					t.Errorf("unexpected error: %s", err)
				}
			},
		},
		"finally error overrides try error": {
			Metadata: map[string]interface{}{
				"TryError": fmt.Errorf("try"),
				"Catch": func(ctx context.Context, resource *resources.TestResource, result reconcile.Result, err error) (reconcile.Result, error) {
					resource.Spec.Fields["catch"] = "called"
					return result, err
				},
				"Finally": &reconcilers.SyncReconciler[*resources.TestResource]{
					SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcile.Result, error) {
						resource.Spec.Fields["finally"] = "called"
						return reconcile.Result{RequeueAfter: 1}, fmt.Errorf("finally")
					},
				},
			},
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("try", "called")
					d.AddField("catch", "called")
					d.AddField("finally", "called")
				}).
				DieReleasePtr(),
			ShouldErr: true,
			Verify: func(t *testing.T, result reconcilers.Result, err error) {
				if result != (reconcile.Result{RequeueAfter: 1}) {
					t.Errorf("unexpected result: %v", result)
				}
				if err.Error() != "finally" {
					t.Errorf("unexpected error: %s", err)
				}
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		r := &reconcilers.TryCatch[*resources.TestResource]{
			Try: &reconcilers.SyncReconciler[*resources.TestResource]{
				SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
					resource.Spec.Fields["try"] = "called"
					var tryErr error
					if err, ok := rtc.Metadata["TryError"]; ok {
						tryErr = err.(error)
					}
					return reconcilers.Result{RequeueAfter: 3}, tryErr
				},
			},
		}
		if catch, ok := rtc.Metadata["Catch"]; ok {
			r.Catch = catch.(func(context.Context, *resources.TestResource, reconcile.Result, error) (reconcile.Result, error))
		}
		if finally, ok := rtc.Metadata["Finally"]; ok {
			r.Finally = finally.(reconcilers.SubReconciler[*resources.TestResource])
		}
		return r
	})
}

func TestTryCatch_Validate(t *testing.T) {
	tests := []struct {
		name           string
		reconciler     *reconcilers.TryCatch[*resources.TestResource]
		validateNested bool
		shouldErr      string
		expectedLogs   []string
	}{
		{
			name: "valid",
			reconciler: &reconcilers.TryCatch[*resources.TestResource]{
				Try: reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name: "with catch",
			reconciler: &reconcilers.TryCatch[*resources.TestResource]{
				Try: reconcilers.Sequence[*resources.TestResource]{},
				Catch: func(ctx context.Context, resource *resources.TestResource, result reconcile.Result, err error) (reconcile.Result, error) {
					return result, err
				},
			},
		},
		{
			name: "with finally",
			reconciler: &reconcilers.TryCatch[*resources.TestResource]{
				Try:     reconcilers.Sequence[*resources.TestResource]{},
				Finally: reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name: "with catch and finally",
			reconciler: &reconcilers.TryCatch[*resources.TestResource]{
				Try: reconcilers.Sequence[*resources.TestResource]{},
				Catch: func(ctx context.Context, resource *resources.TestResource, result reconcile.Result, err error) (reconcile.Result, error) {
					return result, err
				},
				Finally: reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name: "missing try",
			reconciler: &reconcilers.TryCatch[*resources.TestResource]{
				Name: "missing try",
			},
			shouldErr: `TryCatch "missing try" must implement Try`,
		},
		{
			name: "valid try",
			reconciler: &reconcilers.TryCatch[*resources.TestResource]{
				Try: &reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name: "invalid try",
			reconciler: &reconcilers.TryCatch[*resources.TestResource]{
				Try: &reconcilers.SyncReconciler[*resources.TestResource]{
					// Sync: func(ctx context.Context, resource *resources.TestResource) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr:      `TryCatch "TryCatch" must have a valid Try: SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
		{
			name: "valid finally",
			reconciler: &reconcilers.TryCatch[*resources.TestResource]{
				Try: &reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
				Finally: &reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name: "invalid finally",
			reconciler: &reconcilers.TryCatch[*resources.TestResource]{
				Try: &reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
				Finally: &reconcilers.SyncReconciler[*resources.TestResource]{
					// Sync: func(ctx context.Context, resource *resources.TestResource) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr:      `TryCatch "TryCatch" must have a valid Finally: SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			sink := &bufferedSink{}
			ctx := logr.NewContext(context.TODO(), logr.New(sink))
			if c.validateNested {
				ctx = validation.WithRecursive(ctx)
			}
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
			if diff := cmp.Diff(c.expectedLogs, sink.Lines); diff != "" {
				t.Errorf("%s: unexpected logs (-expected, +actual): %s", c.name, diff)
			}
		})
	}
}

func TestOverrideSetup(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		SpecDie(func(d *dies.TestResourceSpecDie) {
			d.Fields(map[string]string{})
		})

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"calls reconcile": {
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("reconciler", "called")
				}).
				DieReleasePtr(),
			ExpectedResult: reconcile.Result{Requeue: true},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		return &reconcilers.OverrideSetup[*resources.TestResource]{
			Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
				Setup: func(ctx context.Context, mgr manager.Manager, bldr *builder.Builder) error {
					return fmt.Errorf("setup")
				},
				SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
					resource.Spec.Fields["reconciler"] = "called"
					return reconcilers.Result{Requeue: true}, nil
				},
			},
		}
	})
}

func TestOverrideSetup_Validate(t *testing.T) {
	tests := []struct {
		name           string
		reconciler     *reconcilers.OverrideSetup[*resources.TestResource]
		validateNested bool
		shouldErr      string
		expectedLogs   []string
	}{
		{
			name: "with reconciler",
			reconciler: &reconcilers.OverrideSetup[*resources.TestResource]{
				Reconciler: reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name: "with setup",
			reconciler: &reconcilers.OverrideSetup[*resources.TestResource]{
				Setup: func(ctx context.Context, mgr manager.Manager, bldr *builder.Builder) error {
					return nil
				},
			},
		},
		{
			name: "with reconciler and setup",
			reconciler: &reconcilers.OverrideSetup[*resources.TestResource]{
				Reconciler: reconcilers.Sequence[*resources.TestResource]{},
				Setup: func(ctx context.Context, mgr manager.Manager, bldr *builder.Builder) error {
					return nil
				},
			},
		},
		{
			name: "missing reconciler or setup",
			reconciler: &reconcilers.OverrideSetup[*resources.TestResource]{
				Name: "missing reconciler or setup",
			},
			shouldErr: `OverrideSetup "missing reconciler or setup" must implement at least one of Setup or Reconciler`,
		},
		{
			name: "valid reconciler",
			reconciler: &reconcilers.OverrideSetup[*resources.TestResource]{
				Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name: "invalid reconciler",
			reconciler: &reconcilers.OverrideSetup[*resources.TestResource]{
				Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
					// Sync: func(ctx context.Context, resource *resources.TestResource) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr: `OverrideSetup "SkipSetup" must have a valid Reconciler: SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			sink := &bufferedSink{}
			ctx := logr.NewContext(context.TODO(), logr.New(sink))
			if c.validateNested {
				ctx = validation.WithRecursive(ctx)
			}
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
			if diff := cmp.Diff(c.expectedLogs, sink.Lines); diff != "" {
				t.Errorf("%s: unexpected logs (-expected, +actual): %s", c.name, diff)
			}
		})
	}
}
