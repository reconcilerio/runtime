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
	"sort"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

func TestChildSetReconciler(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testFinalizer := "test.finalizer"

	idKey := fmt.Sprintf("%s/child-id", resources.GroupVersion.Group)

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

	configMapDesired := diecorev1.ConfigMapBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		AddData("foo", "bar")
	configMapCreate := configMapDesired.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.ControlledBy(resource, scheme)
		})
	configMapGiven := configMapCreate.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.CreationTimestamp(now)
			d.UID(types.UID("3b298fdb-b0b6-4603-9708-939e05daf183"))
		})

	configMapBlueDesired := configMapCreate.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Name(testName + "-blue")
			d.AddAnnotation(idKey, "blue")
		})
	configMapBlueCreate := configMapBlueDesired.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.ControlledBy(resource, scheme)
		})
	configMapBlueGiven := configMapBlueCreate.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.CreationTimestamp(now)
			d.UID(types.UID("a0e91ff9-bf42-4bc7-9253-2a6581b07e4d"))
		})

	configMapGreenDesired := configMapCreate.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Name(testName + "-green")
			d.AddAnnotation(idKey, "green")
		})
	configMapGreenCreate := configMapGreenDesired.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.ControlledBy(resource, scheme)
		})
	configMapGreenGiven := configMapGreenCreate.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.CreationTimestamp(metav1.NewTime(now.Add(-1 * time.Hour)))
			d.UID(types.UID("62af4b9a-767a-4f32-b62c-e4bccbfa8ef0"))
		})

	defaultChildSetReconciler := func(c reconcilers.Config) *reconcilers.ChildSetReconciler[*resources.TestResource, *corev1.ConfigMap, *corev1.ConfigMapList] {
		return &reconcilers.ChildSetReconciler[*resources.TestResource, *corev1.ConfigMap, *corev1.ConfigMapList]{
			DesiredChildren: func(ctx context.Context, parent *resources.TestResource) ([]*corev1.ConfigMap, error) {
				return []*corev1.ConfigMap{}, nil
			},
			IdentifyChild: func(child *corev1.ConfigMap) string {
				annotations := child.GetAnnotations()
				if annotations == nil {
					return ""
				}
				return annotations[idKey]
			},
			ChildObjectManager: &rtesting.StubObjectManager[*corev1.ConfigMap]{},
			ReflectChildrenStatusOnParent: func(ctx context.Context, parent *resources.TestResource, result reconcilers.ChildSetResult[*corev1.ConfigMap]) {
				if err := result.AggregateError(); err != nil {
					if apierrs.IsAlreadyExists(err) {
						name := err.(apierrs.APIStatus).Status().Details.Name
						parent.Status.MarkNotReady(ctx, "NameConflict", "%q already exists", name)
					}
					return
				}

				parent.Status.Fields = map[string]string{}
				for _, childResult := range result.Children {
					child := childResult.Child
					id := childResult.Id
					if child == nil {
						continue
					}
					for k, v := range child.Data {
						parent.Status.Fields[fmt.Sprintf("%s.%s", id, k)] = v
					}
				}
				if len(parent.Status.Fields) == 0 {
					parent.Status.Fields = nil
				}
				parent.Status.MarkReady(ctx)
			},
		}
	}

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"in sync no children": {
			Resource: resourceReady.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return defaultChildSetReconciler(c)
				},
			},
		},
		"in sync with children": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.CalledAtMostTimes("list", "ConfigMapList", 1),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
							configMapGreenDesired.DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
		},
		"preserve existing children": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						children := reconcilers.RetrieveKnownChildren[*corev1.ConfigMap](ctx)
						return children, nil
					}
					return r
				},
			},
		},
		"garbage collect all but oldest child": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						children := reconcilers.RetrieveKnownChildren[*corev1.ConfigMap](ctx)
						sort.Slice(children, func(i, j int) bool {
							iDate := children[i].CreationTimestamp
							jDate := children[j].CreationTimestamp
							return iDate.Before(&jDate)
						})
						return children[0:1], nil
					}
					return r
				},
			},
			ExpectResource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapBlueGiven, scheme),
			},
		},
		"ignores resources that are not ours": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
				// not our resource
				diecorev1.ConfigMapBlank.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Namespace(testNamespace)
						d.Name(testName)
					}).
					DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
							configMapGreenDesired.DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
		},
		"create green child, preserving blue child": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
							configMapGreenDesired.DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
			ExpectResource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			ExpectCreates: []client.Object{
				configMapGreenCreate.DieReleasePtr(),
			},
		},
		"delete green child, preserving blue child": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
			ExpectResource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
				}).
				DieReleasePtr(),
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapGreenGiven.DieReleasePtr(), scheme),
			},
		},
		"update children": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.
								AddData("foo", "updated-blue").
								DieReleasePtr(),
							configMapGreenDesired.
								AddData("foo", "updated-green").
								DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
			ExpectResource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "updated-blue")
					d.AddField("green.foo", "updated-green")
				}).
				DieReleasePtr(),
			ExpectUpdates: []client.Object{
				configMapBlueGiven.
					AddData("foo", "updated-blue").
					DieReleasePtr(),
				configMapGreenGiven.
					AddData("foo", "updated-green").
					DieReleasePtr(),
			},
		},
		"errors for desired children with empty id": {
			Resource: resourceReady.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.
								MetadataDie(func(d *diemetav1.ObjectMetaDie) {
									// clear existing id annotation
									d.DieStamp(func(r *metav1.ObjectMeta) {
										delete(r.Annotations, idKey)
									})
								}).
								DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
			ShouldErr: true,
		},
		"errors for desired children with duplicate ids": {
			Resource: resourceReady.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
							configMapGreenDesired.
								MetadataDie(func(d *diemetav1.ObjectMetaDie) {
									d.AddAnnotation(idKey, "blue")
								}).
								DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
			ShouldErr: true,
		},
		"deletes actual children with duplicate ids": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.AddAnnotation(idKey, "blue")
					}).
					DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapBlueGiven.DieReleasePtr(), scheme),
				rtesting.NewDeleteRefFromObject(configMapGreenGiven.DieReleasePtr(), scheme),
			},
			ExpectCreates: []client.Object{
				configMapBlueCreate.DieReleasePtr(),
			},
		},
		"deletes actual child resource missing id": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
							configMapGreenDesired.DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapGiven.DieReleasePtr(), scheme),
			},
		},
		"defines a finalizer when requested": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.Finalizer = testFinalizer
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
							configMapGreenDesired.DieReleasePtr(),
						}, nil
					}
					r.OurChild = func(resource *resources.TestResource, child *corev1.ConfigMap) bool {
						return true
					}
					r.ListOptions = func(ctx context.Context, resource *resources.TestResource) []client.ListOption {
						return []client.ListOption{
							client.InNamespace(testNamespace),
						}
					}
					return r
				},
			},
			ExpectResource: resourceReady.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.ResourceVersion("1000")
					d.Finalizers(testFinalizer)
				}).
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "FinalizerPatched",
					`Patched finalizer %q`, testFinalizer),
			},
			ExpectPatches: []rtesting.PatchRef{
				{
					Group:     "testing.reconciler.runtime",
					Kind:      "TestResource",
					Namespace: testNamespace,
					Name:      testName,
					PatchType: types.MergePatchType,
					Patch:     []byte(`{"metadata":{"finalizers":["test.finalizer"],"resourceVersion":"999"}}`),
				},
			},
		},
		"forwards error listing children": {
			Resource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
							configMapGreenDesired.DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("list", "ConfigMapList"),
			},
			ShouldErr: true,
		},
		"forwards error from child reconciler": {
			Resource: resourceReady.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return []*corev1.ConfigMap{
							configMapBlueDesired.DieReleasePtr(),
							configMapGreenDesired.DieReleasePtr(),
						}, nil
					}
					return r
				},
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap"),
			},
			ShouldErr: true,
			ExpectCreates: []client.Object{
				configMapBlueCreate.DieReleasePtr(),
			},
		},
		"skip resource manager operations when OnlyReconcileChildStatus is returned": {
			Resource: resource.DieReleasePtr(),
			GivenObjects: []client.Object{
				configMapBlueGiven.DieReleasePtr(),
				configMapGreenGiven.DieReleasePtr(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return nil, reconcilers.OnlyReconcileChildStatus
					}
					return r
				},
			},
			ExpectResource: resourceReady.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
		},
		"skip resource manager operations when OnlyReconcileChildStatus is returned, even when there are no actual or desired children": {
			Resource: resource.
				StatusDie(func(d *dies.TestResourceStatusDie) {
					d.AddField("blue.foo", "bar")
					d.AddField("green.foo", "bar")
				}).
				DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return nil, reconcilers.OnlyReconcileChildStatus
					}
					return r
				},
			},
			ExpectResource: resourceReady.DieReleasePtr(),
		},
		"errors when desired children returns an error": {
			Resource: resourceReady.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					r := defaultChildSetReconciler(c)
					r.DesiredChildren = func(ctx context.Context, resource *resources.TestResource) ([]*corev1.ConfigMap, error) {
						return nil, fmt.Errorf("test")
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

func TestChildSetReconciler_Validate(t *testing.T) {
	tests := []struct {
		name       string
		parent     *corev1.ConfigMap
		reconciler *reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]
		shouldErr  string
	}{
		{
			name:       "empty",
			parent:     &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{},
			shouldErr:  `ChildSetReconciler "PodChildSetReconciler" must implement DesiredChildren`,
		},
		{
			name:   "valid",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
		},
		{
			name:   "valid, ReflectChildrenStatusOnParentWithError",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParentWithError: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) error {
					return nil
				},
				IdentifyChild: func(child *corev1.Pod) string { return "" },
			},
		},
		{
			name:   "ChildType missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name: "ChildType missing",
				// ChildType:                  &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
		},
		{
			name:   "ChildListType missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:      "ChildListType missing",
				ChildType: &corev1.Pod{},
				// ChildListType:              &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
		},
		{
			name:   "DesiredChildren missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:          "DesiredChildren missing",
				ChildType:     &corev1.Pod{},
				ChildListType: &corev1.PodList{},
				// DesiredChildren:            func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
			shouldErr: `ChildSetReconciler "DesiredChildren missing" must implement DesiredChildren`,
		},
		{
			name:   "ReflectChildrenStatusOnParent missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:            "ReflectChildrenStatusOnParent missing",
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				// ReflectChildrenStatusOnParent: func(parent *corev1.ConfigMap, child *corev1.Pod, err error) {},
				IdentifyChild: func(child *corev1.Pod) string { return "" },
			},
			shouldErr: `ChildSetReconciler "ReflectChildrenStatusOnParent missing" must implement ReflectChildrenStatusOnParent or ReflectChildrenStatusOnParentWithError`,
		},
		{
			name:   "IdentifyChild missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:            "IdentifyChild missing",
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				// IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
			shouldErr: `ChildSetReconciler "IdentifyChild missing" must implement IdentifyChild`,
		},
		{
			name:   "ListOptions",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				ListOptions:                   func(ctx context.Context, parent *corev1.ConfigMap) []client.ListOption { return []client.ListOption{} },
				IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
		},
		{
			name:   "ListOptions missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:            "ListOptions missing",
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				SkipOwnerReference:            true,
				// ListOptions:                   func(ctx context.Context, parent *corev1.ConfigMap) []client.ListOption { return []client.ListOption{} },
				OurChild:      func(resource *corev1.ConfigMap, child *corev1.Pod) bool { return true },
				IdentifyChild: func(child *corev1.Pod) string { return "" },
			},
			shouldErr: `ChildSetReconciler "ListOptions missing" must implement ListOptions since owner references are not used`,
		},
		{
			name:   "Finalizer without OurChild",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:            "Finalizer without OurChild",
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				Finalizer:                     "my-finalizer",
				IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
			shouldErr: `ChildSetReconciler "Finalizer without OurChild" must implement OurChild since owner references are not used`,
		},
		{
			name:   "SkipOwnerReference without OurChild",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:            "SkipOwnerReference without OurChild",
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				SkipOwnerReference:            true,
				IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
			shouldErr: `ChildSetReconciler "SkipOwnerReference without OurChild" must implement OurChild since owner references are not used`,
		},
		{
			name:   "OurChild",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				ChildObjectManager: &reconcilers.UpdatingObjectManager[*corev1.Pod]{
					MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				},
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				OurChild:                      func(parent *corev1.ConfigMap, child *corev1.Pod) bool { return false },
				IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
		},
		{
			name:   "ChildObjectManager missing",
			parent: &corev1.ConfigMap{},
			reconciler: &reconcilers.ChildSetReconciler[*corev1.ConfigMap, *corev1.Pod, *corev1.PodList]{
				Name:            "ChildObjectManager missing",
				ChildType:       &corev1.Pod{},
				ChildListType:   &corev1.PodList{},
				DesiredChildren: func(ctx context.Context, parent *corev1.ConfigMap) ([]*corev1.Pod, error) { return nil, nil },
				// ChildObjectManager: &UpdatingObjectManager[*corev1.Pod]{
				// 	MergeBeforeUpdate: func(current, desired *corev1.Pod) {},
				// },
				ReflectChildrenStatusOnParent: func(ctx context.Context, parent *corev1.ConfigMap, result reconcilers.ChildSetResult[*corev1.Pod]) {},
				IdentifyChild:                 func(child *corev1.Pod) string { return "" },
			},
			shouldErr: `ChildSetReconciler "ChildObjectManager missing" must implement ChildObjectManager`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			ctx := reconcilers.StashResourceType(context.TODO(), c.parent)
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err.Error(), c.shouldErr)
			}
		})
	}
}
