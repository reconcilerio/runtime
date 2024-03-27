/*
Copyright 2024 the original author or authors.

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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/apis"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestAdvice(t *testing.T) {
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
		"before can replace the context": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.Advice[*resources.TestResource]{
						Before: func(ctx context.Context, resource *resources.TestResource) (context.Context, reconcile.Result, error) {
							ctx = context.WithValue(ctx, "message", "hello world")
							return ctx, reconcile.Result{}, nil
						},
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								if resource.Status.Fields == nil {
									resource.Status.Fields = map[string]string{}
								}
								resource.Status.Fields["message"] = ctx.Value("message").(string)
								return nil
							},
						},
					}
				},
			},
			ExpectResource: resource.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("message", "hello world")
				}).
				DieReleasePtr(),
		},
		"before can augment the result": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.Advice[*resources.TestResource]{
						Before: func(ctx context.Context, resource *resources.TestResource) (context.Context, reconcile.Result, error) {
							return nil, reconcile.Result{Requeue: true}, nil
						},
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								if resource.Status.Fields == nil {
									resource.Status.Fields = map[string]string{}
								}
								resource.Status.Fields["message"] = "reconciler called"
								return nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcile.Result{
				Requeue: true,
			},
			ExpectResource: resource.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("message", "reconciler called")
				}).
				DieReleasePtr(),
		},
		"before errors return immediately": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.Advice[*resources.TestResource]{
						Before: func(ctx context.Context, resource *resources.TestResource) (context.Context, reconcile.Result, error) {
							return nil, reconcile.Result{}, errors.New("test")
						},
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								t.Errorf("unreachable")
								return nil
							},
						},
					}
				},
			},
			ShouldErr: true,
		},
		"around calls the reconciler by default": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					// this reconciler would fail validation, but more directly expresses the desired behavior
					return &reconcilers.Advice[*resources.TestResource]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								c := reconcilers.RetrieveConfigOrDie(ctx)
								c.Recorder.Event(resource, corev1.EventTypeNormal, "Called", "reconciler called")
								return nil
							},
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Called", "reconciler called"),
			},
		},
		"around can skip the reconciler": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.Advice[*resources.TestResource]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								t.Errorf("unreachable")
								return nil
							},
						},
						Around: func(ctx context.Context, resource *resources.TestResource, reconciler reconcilers.SubReconciler[*resources.TestResource]) (reconcile.Result, error) {
							return reconcilers.Result{}, nil
						},
					}
				},
			},
		},
		"around can call into the reconciler multiple times": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.Advice[*resources.TestResource]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								c := reconcilers.RetrieveConfigOrDie(ctx)
								c.Recorder.Event(resource, corev1.EventTypeNormal, "Called", "reconciler called")
								return nil
							},
						},
						Around: func(ctx context.Context, resource *resources.TestResource, reconciler reconcilers.SubReconciler[*resources.TestResource]) (reconcile.Result, error) {
							result := reconcilers.Result{}
							for i := 0; i < 2; i++ {
								if r, err := reconciler.Reconcile(ctx, resource); true {
									result = reconcilers.AggregateResults(result, r)
								} else if err != nil {
									return result, err
								}
							}
							return result, nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Called", "reconciler called"),
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Called", "reconciler called"),
			},
		},
		"after can rewrite the result": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.Advice[*resources.TestResource]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								return errors.New("test")
							},
						},
						After: func(ctx context.Context, resource *resources.TestResource, result reconcile.Result, err error) (reconcile.Result, error) {
							if err == nil {
								t.Errorf("expected error")
							}
							return reconcile.Result{Requeue: true}, nil
						},
					}
				},
			},
			ExpectedResult: reconcile.Result{
				Requeue: true,
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		return rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource])(t, c)
	})
}
