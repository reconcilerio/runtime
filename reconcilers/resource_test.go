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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/apis"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	rtime "reconciler.io/runtime/time"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestResourceReconciler_NoStatus(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource-no-status"
	testRequest := reconcilers.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceNoStatusBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
			d.AddAnnotation("blah", "blah")
		})

	rts := rtesting.ReconcilerTests{
		"resource exists": {
			Request: testRequest,
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceNoStatus] {
					return &reconcilers.SyncReconciler[*resources.TestResourceNoStatus]{
						Sync: func(ctx context.Context, resource *resources.TestResourceNoStatus) error {
							return nil
						},
					}
				},
			},
		},
	}
	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		return &reconcilers.ResourceReconciler[*resources.TestResourceNoStatus]{
			Reconciler: rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceNoStatus])(t, c),
			Config:     c,
		}
	})
}

func TestResourceReconciler_EmptyStatus(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource-empty-status"
	testRequest := reconcilers.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceEmptyStatusBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
			d.AddAnnotation("blah", "blah")
		})

	rts := rtesting.ReconcilerTests{
		"resource exists": {
			Request: testRequest,
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceEmptyStatus] {
					return &reconcilers.SyncReconciler[*resources.TestResourceEmptyStatus]{
						Sync: func(ctx context.Context, resource *resources.TestResourceEmptyStatus) error {
							return nil
						},
					}
				},
			},
		},
	}
	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		return &reconcilers.ResourceReconciler[*resources.TestResourceEmptyStatus]{
			Reconciler: rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceEmptyStatus])(t, c),
			Config:     c,
		}
	})
}

func TestResourceReconciler_NilableStatus(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testRequest := reconcilers.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceNilableStatusBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})

	rts := rtesting.ReconcilerTests{
		"nil status": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResourceNilableStatus{},
			},
			GivenObjects: []client.Object{
				resource.Status(nil),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceNilableStatus] {
					return &reconcilers.SyncReconciler[*resources.TestResourceNilableStatus]{
						Sync: func(ctx context.Context, resource *resources.TestResourceNilableStatus) error {
							if resource.Status != nil {
								t.Errorf("status expected to be nil")
							}
							return nil
						},
					}
				},
			},
		},
		"status conditions are initialized": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResourceNilableStatus{},
			},
			GivenObjects: []client.Object{
				resource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.ConditionsDie()
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceNilableStatus] {
					return &reconcilers.SyncReconciler[*resources.TestResourceNilableStatus]{
						Sync: func(ctx context.Context, resource *resources.TestResourceNilableStatus) error {
							expected := []metav1.Condition{
								{Type: apis.ConditionReady, Status: metav1.ConditionUnknown, Reason: "Initializing"},
							}
							if diff := cmp.Diff(expected, resource.Status.Conditions, rtesting.IgnoreLastTransitionTime); diff != "" {
								t.Errorf("Unexpected condition (-expected, +actual): %s", diff)
							}
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				resource,
			},
		},
		"reconciler mutated status": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResourceNilableStatus{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceNilableStatus] {
					return &reconcilers.SyncReconciler[*resources.TestResourceNilableStatus]{
						Sync: func(ctx context.Context, resource *resources.TestResourceNilableStatus) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["Reconciler"] = "ran"
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				resource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("Reconciler", "ran")
				}),
			},
		},
		"status update failed": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResourceNilableStatus{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("update", "TestResourceNilableStatus", rtesting.InduceFailureOpts{
					SubResource: "status",
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceNilableStatus] {
					return &reconcilers.SyncReconciler[*resources.TestResourceNilableStatus]{
						Sync: func(ctx context.Context, resource *resources.TestResourceNilableStatus) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["Reconciler"] = "ran"
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeWarning, "StatusUpdateFailed",
					`Failed to update status: inducing failure for update TestResourceNilableStatus`),
			},
			ExpectStatusUpdates: []client.Object{
				resource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("Reconciler", "ran")
				}),
			},
			ShouldErr: true,
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		return &reconcilers.ResourceReconciler[*resources.TestResourceNilableStatus]{
			Reconciler: rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceNilableStatus])(t, c),
			Config:     c,
		}
	})
}

func TestResourceReconciler_Unstructured(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testRequest := reconcilers.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		APIVersion(resources.GroupVersion.Identifier()).
		Kind("TestResource").
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
			d.Generation(1)
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})

	rts := rtesting.ReconcilerTests{
		"in sync status": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return &reconcilers.SyncReconciler[*unstructured.Unstructured]{
						Sync: func(ctx context.Context, resource *unstructured.Unstructured) error {
							return nil
						},
					}
				},
			},
		},
		"status update": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return &reconcilers.SyncReconciler[*unstructured.Unstructured]{
						Sync: func(ctx context.Context, resource *unstructured.Unstructured) error {
							resource.Object["status"].(map[string]interface{})["fields"] = map[string]interface{}{
								"Reconciler": "ran",
							}
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "StatusUpdated", `Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				resource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("Reconciler", "ran")
				}).DieReleaseUnstructured(),
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		return &reconcilers.ResourceReconciler[*unstructured.Unstructured]{
			Type: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": resources.GroupVersion.Identifier(),
					"kind":       "TestResource",
				},
			},
			Reconciler: rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured])(t, c),
			Config:     c,
		}
	})
}

func TestResourceReconciler_Duck(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testFinalizer := "test.finalizer"
	testRequest := reconcilers.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}

	now := metav1.NewTime(time.Now().UTC()).Rfc3339Copy()
	nowRfc3339 := now.Format(time.RFC3339)

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing").LastTransitionTime(now),
			)
		})
	deletedAt := metav1.NewTime(time.UnixMilli(2000))

	rts := rtesting.ReconcilerTests{
		"resource does not exist": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							t.Error("should not be called")
							return nil
						},
					}
				},
			},
		},
		"ignore deleted resource": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource.MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(&deletedAt)
					d.Finalizers(testFinalizer)
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							t.Error("should not be called")
							return nil
						},
					}
				},
			},
		},
		"error fetching resource": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("get", "TestResource"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							t.Error("should not be called")
							return nil
						},
					}
				},
			},
			ShouldErr: true,
		},
		"resource is defaulted": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							if expected, actual := "ran", resource.Spec.Fields["Defaulter"]; expected != actual {
								t.Errorf("unexpected default value, actually = %v, expected = %v", expected, actual)
							}
							return nil
						},
					}
				},
			},
		},
		"status conditions are initialized": {
			Now:     now.Time,
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.ConditionsDie()
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							expected := []metav1.Condition{
								{Type: apis.ConditionReady, Status: metav1.ConditionUnknown, Reason: "Initializing", LastTransitionTime: now},
							}
							if diff := cmp.Diff(expected, resource.Status.Conditions); diff != "" {
								t.Errorf("Unexpected condition (-expected, +actual): %s", diff)
							}
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "StatusPatched",
					`Patched status`),
			},
			ExpectStatusPatches: []rtesting.PatchRef{
				{
					Group:       "testing.reconciler.runtime",
					Kind:        "TestResource",
					Namespace:   resource.GetNamespace(),
					Name:        resource.GetName(),
					SubResource: "status",
					PatchType:   types.MergePatchType,
					Patch:       []byte(`{"spec":{"fields":{"Defaulter":"ran"}},"status":{"conditions":[{"lastTransitionTime":"` + nowRfc3339 + `","message":"","reason":"Initializing","status":"Unknown","type":"Ready"}]}}`),
				},
			},
		},
		"reconciler mutated status": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["Reconciler"] = "ran"
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "StatusPatched",
					`Patched status`),
			},
			ExpectStatusPatches: []rtesting.PatchRef{
				{
					Group:       "testing.reconciler.runtime",
					Kind:        "TestResource",
					Namespace:   resource.GetNamespace(),
					Name:        resource.GetName(),
					SubResource: "status",
					PatchType:   types.MergePatchType,
					Patch:       []byte(`{"spec":{"fields":{"Defaulter":"ran"}},"status":{"fields":{"Reconciler":"ran"}}}`),
				},
			},
		},
		"skip status updates": {
			Request: testRequest,
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SkipStatusUpdate": true,
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["Reconciler"] = "ran"
							return nil
						},
					}
				},
			},
		},
		"sub reconciler erred": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							return fmt.Errorf("reconciler error")
						},
					}
				},
			},
			ShouldErr: true,
		},
		"sub reconciler halted": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return reconcilers.Sequence[*resources.TestDuck]{
						&reconcilers.SyncReconciler[*resources.TestDuck]{
							Sync: func(ctx context.Context, resource *resources.TestDuck) error {
								resource.Status.Fields = map[string]string{
									"want": "this to run",
								}
								return reconcilers.ErrHaltSubReconcilers
							},
						},
						&reconcilers.SyncReconciler[*resources.TestDuck]{
							Sync: func(ctx context.Context, resource *resources.TestDuck) error {
								resource.Status.Fields = map[string]string{
									"don't want": "this to run",
								}
								return fmt.Errorf("reconciler error")
							},
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "StatusPatched",
					`Patched status`),
			},
			ExpectStatusPatches: []rtesting.PatchRef{
				{
					Group:       "testing.reconciler.runtime",
					Kind:        "TestResource",
					Namespace:   resource.GetNamespace(),
					Name:        resource.GetName(),
					SubResource: "status",
					PatchType:   types.MergePatchType,
					Patch:       []byte(`{"spec":{"fields":{"Defaulter":"ran"}},"status":{"fields":{"want":"this to run"}}}`),
				},
			},
		},
		"sub reconciler halted with result": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return reconcilers.Sequence[*resources.TestDuck]{
						&reconcilers.SyncReconciler[*resources.TestDuck]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestDuck) (reconcilers.Result, error) {
								resource.Status.Fields = map[string]string{
									"want": "this to run",
								}
								return reconcilers.Result{Requeue: true}, reconcilers.ErrHaltSubReconcilers
							},
						},
						&reconcilers.SyncReconciler[*resources.TestDuck]{
							Sync: func(ctx context.Context, resource *resources.TestDuck) error {
								resource.Status.Fields = map[string]string{
									"don't want": "this to run",
								}
								return fmt.Errorf("reconciler error")
							},
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "StatusPatched",
					`Patched status`),
			},
			ExpectStatusPatches: []rtesting.PatchRef{
				{
					Group:       "testing.reconciler.runtime",
					Kind:        "TestResource",
					Namespace:   resource.GetNamespace(),
					Name:        resource.GetName(),
					SubResource: "status",
					PatchType:   types.MergePatchType,
					Patch:       []byte(`{"spec":{"fields":{"Defaulter":"ran"}},"status":{"fields":{"want":"this to run"}}}`),
				},
			},
			ExpectedResult: reconcilers.Result{Requeue: true},
		},
		"status patch failed": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("patch", "TestResource", rtesting.InduceFailureOpts{
					SubResource: "status",
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["Reconciler"] = "ran"
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeWarning, "StatusPatchFailed",
					`Failed to patch status: inducing failure for patch TestResource`),
			},
			ExpectStatusPatches: []rtesting.PatchRef{
				{
					Group:       "testing.reconciler.runtime",
					Kind:        "TestResource",
					Namespace:   resource.GetNamespace(),
					Name:        resource.GetName(),
					SubResource: "status",
					PatchType:   types.MergePatchType,
					Patch:       []byte(`{"spec":{"fields":{"Defaulter":"ran"}},"status":{"fields":{"Reconciler":"ran"}}}`),
				},
			},
			ShouldErr: true,
		},
		"context is stashable": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							var key reconcilers.StashKey = "foo"
							// StashValue will panic if the context is not setup correctly
							reconcilers.StashValue(ctx, key, "bar")
							return nil
						},
					}
				},
			},
		},
		"context has config": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							if config := reconcilers.RetrieveConfigOrDie(ctx); config != c {
								t.Errorf("expected config in context, found %#v", config)
							}
							if resourceConfig := reconcilers.RetrieveOriginalConfigOrDie(ctx); resourceConfig != c {
								t.Errorf("expected original config in context, found %#v", resourceConfig)
							}
							return nil
						},
					}
				},
			},
		},
		"context has resource type": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							if resourceType, ok := reconcilers.RetrieveOriginalResourceType(ctx).(*resources.TestDuck); !ok {
								t.Errorf("expected original resource type not in context, found %#v", resourceType)
							}
							if resourceType, ok := reconcilers.RetrieveResourceType(ctx).(*resources.TestDuck); !ok {
								t.Errorf("expected resource type not in context, found %#v", resourceType)
							}
							return nil
						},
					}
				},
			},
		},
		"context can be augmented in Prepare and accessed in Cleanup": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) (context.Context, error) {
				key := "test-key"
				value := "test-value"
				ctx = context.WithValue(ctx, key, value)

				tc.Metadata["SubReconciler"] = func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck] {
					return &reconcilers.SyncReconciler[*resources.TestDuck]{
						Sync: func(ctx context.Context, resource *resources.TestDuck) error {
							if v := ctx.Value(key); v != value {
								t.Errorf("expected %s to be in context", key)
							}
							return nil
						},
					}
				}
				tc.CleanUp = func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) error {
					if v := ctx.Value(key); v != value {
						t.Errorf("expected %s to be in context", key)
					}
					return nil
				}

				return ctx, nil
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		skipStatusUpdate := false
		if skip, ok := rtc.Metadata["SkipStatusUpdate"].(bool); ok {
			skipStatusUpdate = skip
		}
		return &reconcilers.ResourceReconciler[*resources.TestDuck]{
			Type: &resources.TestDuck{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "testing.reconciler.runtime/v1",
					Kind:       "TestResource",
				},
			},
			Reconciler:       rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestDuck])(t, c),
			SkipStatusUpdate: skipStatusUpdate,
			Config:           c,
		}
	})
}

func TestResourceReconciler(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testFinalizer := "test.finalizer"
	testRequest := reconcilers.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	createResource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		})
	givenResource := createResource.
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})
	deletedAt := metav1.NewTime(time.UnixMilli(2000))

	rts := rtesting.ReconcilerTests{
		"resource does not exist": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							t.Error("should not be called")
							return nil
						},
					}
				},
			},
		},
		"ignore deleted resource": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource.MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(&deletedAt)
					d.Finalizers(testFinalizer)
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							t.Error("should not be called")
							return nil
						},
					}
				},
			},
		},
		"error fetching resource": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("get", "TestResource"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							t.Error("should not be called")
							return nil
						},
					}
				},
			},
			ShouldErr: true,
		},
		"resource is defaulted": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if expected, actual := "ran", resource.Spec.Fields["Defaulter"]; expected != actual {
								t.Errorf("unexpected default value, actually = %v, expected = %v", expected, actual)
							}
							return nil
						},
					}
				},
			},
		},
		"status conditions are initialized": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.ConditionsDie()
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							expected := []metav1.Condition{
								{Type: apis.ConditionReady, Status: metav1.ConditionUnknown, Reason: "Initializing"},
							}
							if diff := cmp.Diff(expected, resource.Status.Conditions, rtesting.IgnoreLastTransitionTime); diff != "" {
								t.Errorf("Unexpected condition (-expected, +actual): %s", diff)
							}
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(givenResource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				givenResource,
			},
		},
		"reconciler mutated status": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["Reconciler"] = "ran"
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(givenResource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				givenResource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("Reconciler", "ran")
				}),
			},
		},
		"skip status updates": {
			Request: testRequest,
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SkipStatusUpdate": true,
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["Reconciler"] = "ran"
							return nil
						},
					}
				},
			},
		},
		"sub reconciler erred": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return fmt.Errorf("reconciler error")
						},
					}
				},
			},
			ShouldErr: true,
		},
		"sub reconciler halted": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Sequence[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								resource.Status.Fields = map[string]string{
									"want": "this to run",
								}
								return reconcilers.ErrHaltSubReconcilers
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								resource.Status.Fields = map[string]string{
									"don't want": "this to run",
								}
								return fmt.Errorf("reconciler error")
							},
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(givenResource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				givenResource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("want", "this to run")
				}),
			},
		},
		"sub reconciler halted with result": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Sequence[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							SyncWithResult: func(ctx context.Context, resource *resources.TestResource) (reconcilers.Result, error) {
								resource.Status.Fields = map[string]string{
									"want": "this to run",
								}
								return reconcilers.Result{Requeue: true}, reconcilers.ErrHaltSubReconcilers
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, resource *resources.TestResource) error {
								resource.Status.Fields = map[string]string{
									"don't want": "this to run",
								}
								return fmt.Errorf("reconciler error")
							},
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(givenResource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				givenResource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("want", "this to run")
				}),
			},
			ExpectedResult: reconcilers.Result{Requeue: true},
		},
		"status update failed": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("update", "TestResource", rtesting.InduceFailureOpts{
					SubResource: "status",
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["Reconciler"] = "ran"
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(givenResource, scheme, corev1.EventTypeWarning, "StatusUpdateFailed",
					`Failed to update status: inducing failure for update TestResource`),
			},
			ExpectStatusUpdates: []client.Object{
				givenResource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("Reconciler", "ran")
				}),
			},
			ShouldErr: true,
		},
		"context is stashable": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							var key reconcilers.StashKey = "foo"
							// StashValue will panic if the context is not setup correctly
							reconcilers.StashValue(ctx, key, "bar")
							return nil
						},
					}
				},
			},
		},
		"context has config": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if config := reconcilers.RetrieveConfigOrDie(ctx); config != c {
								t.Errorf("expected config in context, found %#v", config)
							}
							if resourceConfig := reconcilers.RetrieveOriginalConfigOrDie(ctx); resourceConfig != c {
								t.Errorf("expected original config in context, found %#v", resourceConfig)
							}
							return nil
						},
					}
				},
			},
		},
		"context has resource type": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if resourceType, ok := reconcilers.RetrieveOriginalResourceType(ctx).(*resources.TestResource); !ok {
								t.Errorf("expected original resource type not in context, found %#v", resourceType)
							}
							if resourceType, ok := reconcilers.RetrieveResourceType(ctx).(*resources.TestResource); !ok {
								t.Errorf("expected resource type not in context, found %#v", resourceType)
							}
							return nil
						},
					}
				},
			},
		},
		"context can be augmented in Prepare and accessed in Cleanup": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) (context.Context, error) {
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
				tc.CleanUp = func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) error {
					if v := ctx.Value(key); v != value {
						t.Errorf("expected %s to be in context", key)
					}
					return nil
				}

				return ctx, nil
			},
		},
		"before reconcile is called before reconcile and after the context is populated": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			Metadata: map[string]interface{}{
				"BeforeReconcile": func(ctx context.Context, req reconcilers.Request) (context.Context, reconcilers.Result, error) {
					c := reconcilers.RetrieveConfigOrDie(ctx)
					// create the object manually rather than as a given
					r := createResource.
						MetadataDie(func(d *diemetav1.ObjectMetaDie) {
							d.CreationTimestamp(metav1.NewTime(rtime.RetrieveNow(ctx)))
						}).
						DieReleasePtr()
					err := c.Create(ctx, r)
					return nil, reconcile.Result{}, err
				},
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["Reconciler"] = "ran"
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(givenResource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectCreates: []client.Object{
				createResource,
			},
			ExpectStatusUpdates: []client.Object{
				givenResource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("Reconciler", "ran")
				}),
			},
		},
		"before reconcile can influence the result": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"BeforeReconcile": func(ctx context.Context, req reconcilers.Request) (context.Context, reconcilers.Result, error) {
					return nil, reconcile.Result{Requeue: true}, nil
				},
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return nil
						},
					}
				},
			},
			ExpectedResult: reconcile.Result{
				Requeue: true,
			},
		},
		"before reconcile can replace the context": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"BeforeReconcile": func(ctx context.Context, req reconcilers.Request) (context.Context, reconcilers.Result, error) {
					ctx = context.WithValue(ctx, "message", "hello world")
					return ctx, reconcile.Result{}, nil
				},
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.Status.Fields["message"] = ctx.Value("message").(string)
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(givenResource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				givenResource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("message", "hello world")
				}),
			},
		},
		"before reconcile errors shortcut execution": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			Metadata: map[string]interface{}{
				"BeforeReconcile": func(ctx context.Context, req reconcilers.Request) (context.Context, reconcilers.Result, error) {
					return nil, reconcile.Result{}, errors.New("test")
				},
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							t.Error("should not be called")
							return nil
						},
					}
				},
			},
			ShouldErr: true,
		},
		"after reconcile can overwrite the result": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResource{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return errors.New("test")
						},
					}
				},
				"AfterReconcile": func(ctx context.Context, req reconcile.Request, res reconcile.Result, err error) (reconcile.Result, error) {
					// suppress error
					return reconcile.Result{}, nil
				},
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		skipStatusUpdate := false
		if skip, ok := rtc.Metadata["SkipStatusUpdate"].(bool); ok {
			skipStatusUpdate = skip
		}
		var beforeReconcile func(context.Context, reconcilers.Request) (context.Context, reconcilers.Result, error)
		if before, ok := rtc.Metadata["BeforeReconcile"].(func(context.Context, reconcilers.Request) (context.Context, reconcilers.Result, error)); ok {
			beforeReconcile = before
		}
		var afterReconcile func(context.Context, reconcilers.Request, reconcilers.Result, error) (reconcilers.Result, error)
		if after, ok := rtc.Metadata["AfterReconcile"].(func(context.Context, reconcilers.Request, reconcilers.Result, error) (reconcilers.Result, error)); ok {
			afterReconcile = after
		}
		return &reconcilers.ResourceReconciler[*resources.TestResource]{
			Reconciler:       rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource])(t, c),
			SkipStatusUpdate: skipStatusUpdate,
			BeforeReconcile:  beforeReconcile,
			AfterReconcile:   afterReconcile,
			Config:           c,
		}
	})
}

func TestResourceReconciler_UnexportedFields(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testRequest := reconcilers.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceUnexportedFieldsBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		StatusDie(func(d *dies.TestResourceUnexportedFieldsStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})

	rts := rtesting.ReconcilerTests{
		"mutated exported and unexported status": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResourceUnexportedFields{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceUnexportedFields] {
					return &reconcilers.SyncReconciler[*resources.TestResourceUnexportedFields]{
						Sync: func(ctx context.Context, resource *resources.TestResourceUnexportedFields) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.ReflectUnexportedFieldsToStatus()
							resource.Status.Fields["Reconciler"] = "ran"
							resource.Status.AddUnexportedField("Reconciler", "ran")
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				resource.StatusDie(func(d *dies.TestResourceUnexportedFieldsStatusDie) {
					d.AddField("Reconciler", "ran")
					d.AddUnexportedField("Reconciler", "ran")
				}),
			},
		},
		"mutated unexported status": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResourceUnexportedFields{},
			},
			GivenObjects: []client.Object{
				resource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceUnexportedFields] {
					return &reconcilers.SyncReconciler[*resources.TestResourceUnexportedFields]{
						Sync: func(ctx context.Context, resource *resources.TestResourceUnexportedFields) error {
							if resource.Status.Fields == nil {
								resource.Status.Fields = map[string]string{}
							}
							resource.ReflectUnexportedFieldsToStatus()
							resource.Status.AddUnexportedField("Reconciler", "ran")
							return nil
						},
					}
				},
			},
		},
		"no mutated status": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResourceUnexportedFields{},
			},
			GivenObjects: []client.Object{
				resource.StatusDie(func(d *dies.TestResourceUnexportedFieldsStatusDie) {
					d.AddUnexportedField("Test", "ran")
					d.AddUnexportedField("Reconciler", "ran")
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceUnexportedFields] {
					return &reconcilers.SyncReconciler[*resources.TestResourceUnexportedFields]{
						Sync: func(ctx context.Context, resource *resources.TestResourceUnexportedFields) error {
							resource.Status.AddUnexportedField("Reconciler", "ran")
							return nil
						},
					}
				},
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		return &reconcilers.ResourceReconciler[*resources.TestResourceUnexportedFields]{
			Reconciler: rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceUnexportedFields])(t, c),
			Config:     c,
		}
	})
}

func TestResourceReconciler_LegacyDefault(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testRequest := reconcilers.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	createResource := dies.TestResourceWithLegacyDefaultBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		})
	givenResource := createResource.
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})

	rts := rtesting.ReconcilerTests{
		"resource is defaulted": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResourceWithLegacyDefault{},
			},
			GivenObjects: []client.Object{
				givenResource,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceWithLegacyDefault] {
					return &reconcilers.SyncReconciler[*resources.TestResourceWithLegacyDefault]{
						Sync: func(ctx context.Context, resource *resources.TestResourceWithLegacyDefault) error {
							if expected, actual := "ran", resource.Spec.Fields["Defaulter"]; expected != actual {
								t.Errorf("unexpected default value, actually = %v, expected = %v", expected, actual)
							}
							return nil
						},
					}
				},
			},
		},
		"status conditions are initialized": {
			Request: testRequest,
			StatusSubResourceTypes: []client.Object{
				&resources.TestResourceWithLegacyDefault{},
			},
			GivenObjects: []client.Object{
				givenResource.StatusDie(func(d *dies.TestResourceStatusDie) {
					d.ConditionsDie()
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceWithLegacyDefault] {
					return &reconcilers.SyncReconciler[*resources.TestResourceWithLegacyDefault]{
						Sync: func(ctx context.Context, resource *resources.TestResourceWithLegacyDefault) error {
							expected := []metav1.Condition{
								{Type: apis.ConditionReady, Status: metav1.ConditionUnknown, Reason: "Initializing"},
							}
							if diff := cmp.Diff(expected, resource.Status.Conditions, rtesting.IgnoreLastTransitionTime); diff != "" {
								t.Errorf("Unexpected condition (-expected, +actual): %s", diff)
							}
							return nil
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(givenResource, scheme, corev1.EventTypeNormal, "StatusUpdated",
					`Updated status`),
			},
			ExpectStatusUpdates: []client.Object{
				givenResource,
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		skipStatusUpdate := false
		if skip, ok := rtc.Metadata["SkipStatusUpdate"].(bool); ok {
			skipStatusUpdate = skip
		}
		var beforeReconcile func(context.Context, reconcilers.Request) (context.Context, reconcilers.Result, error)
		if before, ok := rtc.Metadata["BeforeReconcile"].(func(context.Context, reconcilers.Request) (context.Context, reconcilers.Result, error)); ok {
			beforeReconcile = before
		}
		var afterReconcile func(context.Context, reconcilers.Request, reconcilers.Result, error) (reconcilers.Result, error)
		if after, ok := rtc.Metadata["AfterReconcile"].(func(context.Context, reconcilers.Request, reconcilers.Result, error) (reconcilers.Result, error)); ok {
			afterReconcile = after
		}
		return &reconcilers.ResourceReconciler[*resources.TestResourceWithLegacyDefault]{
			Reconciler:       rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResourceWithLegacyDefault])(t, c),
			SkipStatusUpdate: skipStatusUpdate,
			BeforeReconcile:  beforeReconcile,
			AfterReconcile:   afterReconcile,
			Config:           c,
		}
	})
}
