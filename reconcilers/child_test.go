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
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	diecorev1 "reconciler.io/dies/apis/core/v1"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/apis"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestChildReconciler(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testFinalizer := "test.finalizer"

	now := metav1.NewTime(time.Now().Truncate(time.Second))

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
	resourceReady := resource.
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionTrue).Reason("Ready"),
			)
		})

	configMapCreate := diecorev1.ConfigMapBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
			d.ControlledBy(resource, scheme)
		}).
		AddData("foo", "bar")
	configMapGiven := configMapCreate.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
		})

	defaultChildReconciler := func(c reconcilers.Config) *reconcilers.ChildReconciler[*resources.TestResource, *corev1.ConfigMap, *corev1.ConfigMapList] {
		return &reconcilers.ChildReconciler[*resources.TestResource, *corev1.ConfigMap, *corev1.ConfigMapList]{
			DesiredChild: func(ctx context.Context, parent *resources.TestResource) (*corev1.ConfigMap, error) {
				if len(parent.Spec.Fields) == 0 {
					return nil, nil
				}

				return &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: parent.Namespace,
						Name:      parent.Name,
					},
					Data: reconcilers.MergeMaps(parent.Spec.Fields),
				}, nil
			},
			ChildObjectManager: &rtesting.StubObjectManager[*corev1.ConfigMap]{},
			ReflectChildStatusOnParent: func(ctx context.Context, parent *resources.TestResource, child *corev1.ConfigMap, err error) {
				if err != nil {
					switch {
					case apierrs.IsAlreadyExists(err):
						name := err.(apierrs.APIStatus).Status().Details.Name
						parent.Status.MarkNotReady(ctx, "NameConflict", "%q already exists", name)
					case apierrs.IsInvalid(err):
						name := err.(apierrs.APIStatus).Status().Details.Name
						parent.Status.MarkNotReady(ctx, "InvalidChild", "%q was rejected by the api server", name)
					}
					return
				}
				if child == nil {
					parent.Status.Fields = nil
					parent.Status.MarkReady(ctx)
					return
				}
				parent.Status.Fields = reconcilers.MergeMaps(child.Data)
				parent.Status.MarkReady(ctx)
			},
		}
	}

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"preserve no child": {
			Resource: resourceReady.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
		},
		"child is in sync": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
		},
		"child is in sync, in a different namespace": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Namespace("other-ns")
					}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildReconciler(c)
					r.ListOptions = func(ctx context.Context, parent *resources.TestResource) []client.ListOption {
						return []client.ListOption{
							client.InNamespace("other-ns"),
						}
					}
					return r
				},
			},
		},
		"create child": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			ExpectCreates: []client.Object{
				configMapCreate,
			},
		},
		"create child with custom owner reference": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildReconciler(c)
					desiredChild := r.DesiredChild
					r.DesiredChild = func(ctx context.Context, resource *resources.TestResource) (*corev1.ConfigMap, error) {
						child, err := desiredChild(ctx, resource)
						if child != nil {
							child.OwnerReferences = []metav1.OwnerReference{
								{
									APIVersion: resources.GroupVersion.String(),
									Kind:       "TestResource",
									Name:       resource.GetName(),
									UID:        resource.GetUID(),
									Controller: ptr.To(true),
									// the default controller ref is set to block
									BlockOwnerDeletion: ptr.To(false),
								},
							}
						}
						return child, err
					}
					return r
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			ExpectCreates: []client.Object{
				configMapCreate.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.OwnerReferences(
							metav1.OwnerReference{
								APIVersion:         resources.GroupVersion.String(),
								Kind:               "TestResource",
								Name:               resource.GetName(),
								UID:                resource.GetUID(),
								Controller:         ptr.To(true),
								BlockOwnerDeletion: ptr.To(false),
							},
						)
					}),
			},
		},
		"update child": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
					d.AddField("new", "field")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
					d.AddField("new", "field")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
					d.AddField("new", "field")
				}).
				DieReleasePtr(),
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("new", "field"),
			},
		},
		"delete child": {
			Resource: resourceReady.DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapGiven, scheme),
			},
		},
		"ignore extraneous children": {
			Resource: resourceReady.DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildReconciler(c)
					r.OurChild = func(parent *resources.TestResource, child *corev1.ConfigMap) bool {
						return false
					}
					return r
				},
			},
		},
		"delete duplicate children": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("extra-child-1")
					}),
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("extra-child-2")
					}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			ExpectDeletes: []rtesting.DeleteRef{
				{Group: "", Kind: "ConfigMap", Namespace: testNamespace, Name: "extra-child-1"},
				{Group: "", Kind: "ConfigMap", Namespace: testNamespace, Name: "extra-child-2"},
			},
			ExpectCreates: []client.Object{
				configMapCreate,
			},
		},
		"delete child during finalization": {
			Resource: resourceReady.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(&now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.OwnerReferences()
					}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildReconciler(c)
					r.SkipOwnerReference = true
					r.OurChild = func(parent *resources.TestResource, child *corev1.ConfigMap) bool { return true }
					r.ListOptions = func(ctx context.Context, resource *resources.TestResource) []client.ListOption {
						return []client.ListOption{
							client.InNamespace(testNamespace),
						}
					}
					return r
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapGiven, scheme),
			},
		},
		"invalid child": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap", rtesting.InduceFailureOpts{
					Error: apierrs.NewInvalid(schema.GroupKind{}, testName, field.ErrorList{
						field.Invalid(field.NewPath("metadata", "name"), testName, ""),
					}),
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.ConditionsDie(
						diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionFalse).
							Reason("InvalidChild").Message(`"test-resource" was rejected by the api server`),
					)
				}).
				DieReleasePtr(),
			ExpectCreates: []client.Object{
				configMapCreate,
			},
		},
		"invalid child - return error rather than reflect": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap", rtesting.InduceFailureOpts{
					Error: apierrs.NewInvalid(schema.GroupKind{}, testName, field.ErrorList{
						field.Invalid(field.NewPath("metadata", "name"), testName, ""),
					}),
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildReconciler(c)
					// suppress all error reflection
					r.ReflectedChildErrorReasons = []metav1.StatusReason{}
					r.ReflectChildStatusOnParent = func(ctx context.Context, parent *resources.TestResource, child *corev1.ConfigMap, err error) {
						t.Fatalf("ReflectChildStatusOnParent should not be called")
					}
					return r
				},
			},
			ShouldErr: true,
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			ExpectCreates: []client.Object{
				configMapCreate,
			},
		},
		"child name collision": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			APIGivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.OwnerReferences()
					}),
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap", rtesting.InduceFailureOpts{
					Error: apierrs.NewAlreadyExists(schema.GroupResource{}, testName),
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.ConditionsDie(
						diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionFalse).
							Reason("NameConflict").Message(`"test-resource" already exists`),
					)
				}).
				DieReleasePtr(),
			ExpectCreates: []client.Object{
				configMapCreate,
			},
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(configMapGiven, resource, scheme),
			},
		},
		"child name collision, stale informer cache": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			APIGivenObjects: []client.Object{
				configMapGiven,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap", rtesting.InduceFailureOpts{
					Error: apierrs.NewAlreadyExists(schema.GroupResource{}, testName),
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectCreates: []client.Object{
				configMapCreate,
			},
			ShouldErr: true,
		},
		"status only reconcile": {
			Resource: resource.DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			ExpectResource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildReconciler(c)
					r.DesiredChild = func(ctx context.Context, parent *resources.TestResource) (*corev1.ConfigMap, error) {
						return nil, reconcilers.OnlyReconcileChildStatus
					}
					return r
				},
			},
		},
		"error listing children": {
			Resource: resourceReady.DieReleasePtr(),
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("list", "ConfigMapList"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ShouldErr: true,
		},
		"error creating child": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectCreates: []client.Object{
				configMapCreate,
			},
			ShouldErr: true,
		},
		"error updating child": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
					d.AddField("new", "field")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("update", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("new", "field"),
			},
			ShouldErr: true,
		},
		"error deleting child": {
			Resource: resourceReady.DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("delete", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapGiven, scheme),
			},
			ShouldErr: true,
		},
		"error deleting duplicate children": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("extra-child-1")
					}),
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("extra-child-2")
					}),
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("delete", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildReconciler(c)
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				{Group: "", Kind: "ConfigMap", Namespace: testNamespace, Name: "extra-child-1"},
			},
			ShouldErr: true,
		},
		"error creating desired child": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildReconciler(c)
					r.DesiredChild = func(ctx context.Context, parent *resources.TestResource) (*corev1.ConfigMap, error) {
						return nil, fmt.Errorf("test error")
					}
					return r
				},
			},
			ShouldErr: true,
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		return rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource])(t, c)
	})
}

func TestChildReconciler_Unstructured(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		APIVersion("testing.reconciler.runtime/v1").
		Kind("TestResource").
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})
	resourceReady := resource.
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionTrue).Reason("Ready"),
			)
		})

	configMapCreate := diecorev1.ConfigMapBlank.
		APIVersion("v1").
		Kind("ConfigMap").
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
			d.ControlledBy(resource, scheme)
		}).
		AddData("foo", "bar")
	configMapGiven := configMapCreate.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
		})

	defaultChildReconciler := func(c reconcilers.Config) *reconcilers.ChildReconciler[*unstructured.Unstructured, *unstructured.Unstructured, *unstructured.UnstructuredList] {
		return &reconcilers.ChildReconciler[*unstructured.Unstructured, *unstructured.Unstructured, *unstructured.UnstructuredList]{
			ChildType: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
				},
			},
			ChildListType: &unstructured.UnstructuredList{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMapList",
				},
			},
			DesiredChild: func(ctx context.Context, parent *unstructured.Unstructured) (*unstructured.Unstructured, error) {
				fields, ok, _ := unstructured.NestedMap(parent.Object, "spec", "fields")
				if !ok || len(fields) == 0 {
					return nil, nil
				}

				child := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"namespace": parent.GetNamespace(),
							"name":      parent.GetName(),
						},
						"data": map[string]interface{}{},
					},
				}
				for k, v := range fields {
					unstructured.SetNestedField(child.Object, v, "data", k)
				}

				return child, nil
			},
			ChildObjectManager: &rtesting.StubObjectManager[*unstructured.Unstructured]{},
			ReflectChildStatusOnParent: func(ctx context.Context, parent *unstructured.Unstructured, child *unstructured.Unstructured, err error) {
				if err != nil {
					if apierrs.IsAlreadyExists(err) {
						name := err.(apierrs.APIStatus).Status().Details.Name
						readyCond := map[string]interface{}{
							"type":    "Ready",
							"status":  "False",
							"reason":  "NameConflict",
							"message": fmt.Sprintf("%q already exists", name),
						}
						unstructured.SetNestedSlice(parent.Object, []interface{}{readyCond}, "status", "conditions")
					}
					return
				}
				if child == nil {
					unstructured.RemoveNestedField(parent.Object, "status", "fields")
					readyCond := map[string]interface{}{
						"type":    "Ready",
						"status":  "True",
						"reason":  "Ready",
						"message": "",
					}
					unstructured.SetNestedSlice(parent.Object, []interface{}{readyCond}, "status", "conditions")
					return
				}
				unstructured.SetNestedMap(parent.Object, map[string]interface{}{}, "status", "fields")
				for k, v := range child.Object["data"].(map[string]interface{}) {
					unstructured.SetNestedField(parent.Object, v, "status", "fields", k)
				}
				readyCond := map[string]interface{}{
					"type":    "Ready",
					"status":  "True",
					"reason":  "Ready",
					"message": "",
				}
				unstructured.SetNestedSlice(parent.Object, []interface{}{readyCond}, "status", "conditions")
			},
		}
	}

	rts := rtesting.SubReconcilerTests[*unstructured.Unstructured]{
		"preserve no child": {
			Resource: resourceReady.DieReleaseUnstructured(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
		},
		"child is in sync": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
		},
		"child is in sync, in a different namespace": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Namespace("other-ns")
					}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					r := defaultChildReconciler(c)
					r.ListOptions = func(ctx context.Context, parent *unstructured.Unstructured) []client.ListOption {
						return []client.ListOption{
							client.InNamespace("other-ns"),
						}
					}
					return r
				},
			},
		},
		"create child": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			ExpectCreates: []client.Object{
				configMapCreate.DieReleaseUnstructured(),
			},
		},
		"update child": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
					d.AddField("new", "field")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
					d.AddField("new", "field")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
					d.AddField("new", "field")
				}).
				DieReleaseUnstructured(),
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("new", "field").
					DieReleaseUnstructured(),
			},
		},
		"delete child": {
			Resource: resourceReady.DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapGiven, scheme),
			},
		},
		"ignore extraneous children": {
			Resource: resourceReady.DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					r := defaultChildReconciler(c)
					r.OurChild = func(parent *unstructured.Unstructured, child *unstructured.Unstructured) bool {
						return false
					}
					return r
				},
			},
		},
		"delete duplicate children": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("extra-child-1")
					}),
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("extra-child-2")
					}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			ExpectDeletes: []rtesting.DeleteRef{
				{Group: "", Kind: "ConfigMap", Namespace: testNamespace, Name: "extra-child-1"},
				{Group: "", Kind: "ConfigMap", Namespace: testNamespace, Name: "extra-child-2"},
			},
			ExpectCreates: []client.Object{
				configMapCreate.DieReleaseUnstructured(),
			},
		},
		"child name collision": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			APIGivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.OwnerReferences()
					}).DieReleaseUnstructured(),
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap", rtesting.InduceFailureOpts{
					Error: apierrs.NewAlreadyExists(schema.GroupResource{}, testName),
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectResource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.ConditionsDie(
						diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionFalse).
							Reason("NameConflict").Message(`"test-resource" already exists`),
					)
				}).
				DieReleaseUnstructured(),
			ExpectCreates: []client.Object{
				configMapCreate.DieReleaseUnstructured(),
			},
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(configMapGiven, resource, scheme),
			},
		},
		"child name collision, stale informer cache": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			APIGivenObjects: []client.Object{
				configMapGiven,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap", rtesting.InduceFailureOpts{
					Error: apierrs.NewAlreadyExists(schema.GroupResource{}, testName),
				}),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectCreates: []client.Object{
				configMapCreate.DieReleaseUnstructured(),
			},
			ShouldErr: true,
		},
		"status only reconcile": {
			Resource: resource.DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			ExpectResource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					r := defaultChildReconciler(c)
					r.DesiredChild = func(ctx context.Context, parent *unstructured.Unstructured) (*unstructured.Unstructured, error) {
						return nil, reconcilers.OnlyReconcileChildStatus
					}
					return r
				},
			},
		},
		"error listing children": {
			Resource: resourceReady.DieReleaseUnstructured(),
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("list", "ConfigMapList"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ShouldErr: true,
		},
		"error creating child": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectCreates: []client.Object{
				configMapCreate.DieReleaseUnstructured(),
			},
			ShouldErr: true,
		},
		"error updating child": {
			Resource: resourceReady.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
					d.AddField("new", "field")
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("update", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("new", "field").
					DieReleaseUnstructured(),
			},
			ShouldErr: true,
		},
		"error deleting child": {
			Resource: resourceReady.DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("delete", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapGiven, scheme),
			},
			ShouldErr: true,
		},
		"error deleting duplicate children": {
			Resource: resource.
				SpecDie(func(d *dies.TestResourceSpecDie) {
					d.AddField("foo", "bar")
				}).
				DieReleaseUnstructured(),
			GivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("extra-child-1")
					}),
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("extra-child-2")
					}),
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("delete", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					return defaultChildReconciler(c)
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				{Group: "", Kind: "ConfigMap", Namespace: testNamespace, Name: "extra-child-1"},
			},
			ShouldErr: true,
		},
		"error creating desired child": {
			Resource: resource.DieReleaseUnstructured(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
					r := defaultChildReconciler(c)
					r.DesiredChild = func(ctx context.Context, parent *unstructured.Unstructured) (*unstructured.Unstructured, error) {
						return nil, fmt.Errorf("test error")
					}
					return r
				},
			},
			ShouldErr: true,
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*unstructured.Unstructured], c reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured] {
		return rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*unstructured.Unstructured])(t, c)
	})
}

func TestChildReconciler_Validate(t *testing.T) {
	tests := []struct {
		name       string
		parent     *corev1.ConfigMap
		reconciler *reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]
		shouldErr  string
	}{
		{
			name:       "empty",
			parent:     &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{},
			shouldErr:  `ChildReconciler "" must implement DesiredChild`,
		},
		{
			name:   "valid",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				ChildType:     &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				DesiredChild:  func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
			},
		},
		{
			name:   "ChildType missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name: "ChildType missing",
				// ChildType:                  &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				DesiredChild:  func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
			},
		},
		{
			name:   "ChildListType missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:      "ChildListType missing",
				ChildType: &corev1.Pod{},
				// ChildListType:              &corev1.PodList{},
				DesiredChild: func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
			},
		},
		{
			name:   "DesiredChild missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:          "DesiredChild missing",
				ChildType:     &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				// DesiredChild:               func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
			},
			shouldErr: `ChildReconciler "DesiredChild missing" must implement DesiredChild`,
		},
		{
			name:   "ReflectChildStatusOnParent missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:          "ReflectChildStatusOnParent missing",
				ChildType:     &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				DesiredChild:  func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				// ReflectChildStatusOnParent: func(parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
			},
			shouldErr: `ChildReconciler "ReflectChildStatusOnParent missing" must implement ReflectChildStatusOnParent`,
		},
		{
			name:   "ListOptions",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				ChildType:     &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				DesiredChild:  func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
				ListOptions:                func(ctx context.Context, parent *corev1.ConfigMap) []client.ListOption { return []client.ListOption{} },
			},
		},
		{
			name:   "ListOptions missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:          "ListOptions missing",
				ChildType:     &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				DesiredChild:  func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
				SkipOwnerReference:         true,
				// ListOptions:                func(ctx context.Context, parent *corev1.ConfigMap) []client.ListOption { return []client.ListOption{} },
				OurChild: func(resource *corev1.ConfigMap, child *corev1.Pod) bool { return true },
			},
			shouldErr: `ChildReconciler "ListOptions missing" must implement ListOptions since owner references are not used`,
		},
		{
			name:   "SkipOwnerReference without OurChild",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:          "SkipOwnerReference without OurChild",
				ChildType:     &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				DesiredChild:  func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
				SkipOwnerReference:         true,
			},
			shouldErr: `ChildReconciler "SkipOwnerReference without OurChild" must implement OurChild since owner references are not used`,
		},
		{
			name:   "OurChild",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				ChildType:     &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				DesiredChild:  func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
				OurChild:                   func(parent *corev1.ConfigMap, child *corev1.Pod) bool { return false },
			},
		},
		{
			name:   "ChildObjectManager missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:          "ChildObjectManager missing",
				ChildType:     &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				DesiredChild:  func(ctx context.Context, parent *corev1.ConfigMap) (*corev1.Pod, error) { return nil, nil },
				// ChildObjectManager: &UpdatingObjectManager[*corev1.Pod]{
				// 	MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				// },
				ReflectChildStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
			},
			shouldErr: `ChildReconciler "ChildObjectManager missing" must implement ChildObjectManager`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			ctx := reconcilers.StashResourceType(context.TODO(), c.parent)
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				var errString string
				if err != nil {
					errString = err.Error()
				}
				t.Errorf("validate() error = %q, shouldErr %q", errString, c.shouldErr)
			}
		})
	}
}
