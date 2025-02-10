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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	diecorev1 "reconciler.io/dies/apis/core/v1"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/apis"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCastResource(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

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
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.TemplateDie(func(d *diecorev1.PodTemplateSpecDie) {
						d.SpecDie(func(d *diecorev1.PodSpecDie) {
							d.ContainerDie("test-container", func(d *diecorev1.ContainerDie) {})
						})
					})
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.CastResource[*resources.TestResource, *appsv1.Deployment]{
						Reconciler: &reconcilers.SyncReconciler[*appsv1.Deployment]{
							Sync: func(ctx context.Context, resource *appsv1.Deployment) error {
								reconcilers.RetrieveConfigOrDie(ctx).
									Recorder.Event(resource, corev1.EventTypeNormal, "Test",
									resource.Spec.Template.Spec.Containers[0].Name)
								return nil
							},
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Test", "test-container"),
			},
		},
		"cast mutation": {
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.TemplateDie(func(d *diecorev1.PodTemplateSpecDie) {
						d.MetadataDie(func(d *diemetav1.ObjectMetaDie) {
							d.Name("mutation")
						})
					})
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.CastResource[*resources.TestResource, *appsv1.Deployment]{
						Reconciler: &reconcilers.SyncReconciler[*appsv1.Deployment]{
							Sync: func(ctx context.Context, resource *appsv1.Deployment) error {
								// mutation that exists on the original resource and will be reflected
								resource.Spec.Template.Name = "mutation"
								// mutation that does not exists on the original resource and will be dropped
								resource.Spec.Paused = true
								return nil
							},
						},
					}
				},
			},
		},
		"return subreconciler result": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.CastResource[*resources.TestResource, *appsv1.Deployment]{
						Reconciler: &reconcilers.SyncReconciler[*appsv1.Deployment]{
							SyncWithResult: func(ctx context.Context, resource *appsv1.Deployment) (reconcilers.Result, error) {
								return reconcilers.Result{Requeue: true}, nil
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{Requeue: true},
		},
		"return subreconciler err, preserves result and status update": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.CastResource[*resources.TestResource, *appsv1.Deployment]{
						Reconciler: &reconcilers.SyncReconciler[*appsv1.Deployment]{
							SyncWithResult: func(ctx context.Context, resource *appsv1.Deployment) (reconcilers.Result, error) {
								resource.Status.Conditions[0] = appsv1.DeploymentCondition{
									Type:    apis.ConditionReady,
									Status:  corev1.ConditionFalse,
									Reason:  "Failed",
									Message: "expected error",
								}
								return reconcilers.Result{Requeue: true}, fmt.Errorf("subreconciler error")
							},
						},
					}
				},
			},
			ExpectedResult: reconcilers.Result{Requeue: true},
			ExpectResource: resource.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.ConditionsDie(
						diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionFalse).Reason("Failed").Message("expected error"),
					)
				}).
				DieReleasePtr(),
			ShouldErr: true,
		},
		"marshal error": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.ErrOnMarshal(true)
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.CastResource[*resources.TestResource, *resources.TestResource]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								c.Recorder.Event(resource, corev1.EventTypeNormal, "Test", resource.Name)
								return nil
							},
						},
					}
				},
			},
			ShouldErr: true,
		},
		"unmarshal error": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.ErrOnUnmarshal(true)
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.CastResource[*resources.TestResource, *resources.TestResource]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								c.Recorder.Event(resource, corev1.EventTypeNormal, "Test", resource.Name)
								return nil
							},
						},
					}
				},
			},
			ShouldErr: true,
		},
		"cast mutation patch error": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.CastResource[*resources.TestResource, *resources.TestResource]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, r *resources.TestResource) error {
								r.Spec.ErrOnMarshal = true
								return fmt.Errorf("subreconciler error")
							},
						},
					}
				},
			},
			ShouldErr: true,
		},
		"cast mutation patch apply error": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.CastResource[*resources.TestResource, *resources.TestResource]{
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, r *resources.TestResource) error {
								r.Spec.ErrOnUnmarshal = true
								return fmt.Errorf("subreconciler error")
							},
						},
					}
				},
			},
			ExpectResource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.ErrOnUnmarshal(true)
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.ConditionsDie() // The unmarshal error would result in losing the initializing Ready condition during applying the patch
				}).
				DieReleasePtr(),
			ShouldErr: true,
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		return rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource])(t, c)
	})
}

func TestCastResource_Validate(t *testing.T) {
	tests := []struct {
		name           string
		resource       client.Object
		reconciler     *reconcilers.CastResource[*corev1.ConfigMap, *corev1.Secret]
		validateNested bool
		shouldErr      string
	}{
		{
			name:       "empty",
			resource:   &corev1.ConfigMap{},
			reconciler: &reconcilers.CastResource[*corev1.ConfigMap, *corev1.Secret]{},
			shouldErr:  `CastResource "" must define Reconciler`,
		},
		{
			name:     "valid",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.CastResource[*corev1.ConfigMap, *corev1.Secret]{
				Reconciler: &reconcilers.SyncReconciler[*corev1.Secret]{
					Sync: func(ctx context.Context, resource *corev1.Secret) error {
						return nil
					},
				},
			},
		},
		{
			name:     "missing reconciler",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.CastResource[*corev1.ConfigMap, *corev1.Secret]{
				Name:       "missing reconciler",
				Reconciler: nil,
			},
			shouldErr: `CastResource "missing reconciler" must define Reconciler`,
		},
		{
			name:     "valid reconciler",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.CastResource[*corev1.ConfigMap, *corev1.Secret]{
				Reconciler: &reconcilers.SyncReconciler[*corev1.Secret]{
					Sync: func(ctx context.Context, resource *corev1.Secret) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name:     "invalid reconciler",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.CastResource[*corev1.ConfigMap, *corev1.Secret]{
				Reconciler: &reconcilers.SyncReconciler[*corev1.Secret]{
					// Sync: func(ctx context.Context, resource *corev1.Secret) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr:      `CastResource "" must have a valid Reconciler: SyncReconciler "" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			ctx := reconcilers.StashResourceType(context.TODO(), c.resource)
			if c.validateNested {
				ctx = reconcilers.WithNestedValidation(ctx)
			}
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
		})
	}
}
