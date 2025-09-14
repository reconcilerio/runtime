/*
Copyright 2022 the original author or authors.

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
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	applyconfigurationsappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestExpectConfig(t *testing.T) {
	ns := "my-namespace"
	r1 := &resources.TestResource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "resource-1",
		},
	}
	r1patch := &resources.TestResource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "resource-1",
		},
		Status: resources.TestResourceStatus{
			Fields: map[string]string{
				"foo": "bar",
			},
		},
	}
	r2 := &resources.TestResource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "resource-2",
		},
	}

	ac1 := applyconfigurationsappsv1.Deployment("resource", ns).
		WithSpec(applyconfigurationsappsv1.DeploymentSpec().WithReplicas(1))
	ac2 := applyconfigurationsappsv1.Deployment("resource", ns).
		WithSpec(applyconfigurationsappsv1.DeploymentSpec().WithReplicas(2))

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	tests := map[string]struct {
		config           ExpectConfig
		operation        func(t *testing.T, ctx context.Context, c reconcilers.Config)
		failedAssertions []string
	}{
		"no mutations": {
			config:           ExpectConfig{},
			operation:        func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{},
		},

		"get given object": {
			config: ExpectConfig{
				GivenObjects: []client.Object{
					r1.DeepCopy(),
					r2.DeepCopy(),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				r := &resources.TestResource{}
				err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: "resource-1"}, r)
				if err != nil {
					t.Errorf("unexpected get error: %s", err)
				}
				if r.Namespace != ns || r.Name != "resource-1" {
					t.Errorf("got unexpected object")
				}
			},
			failedAssertions: []string{},
		},
		"client reactor": {
			config: ExpectConfig{
				GivenObjects: []client.Object{
					r1.DeepCopy(),
					r2.DeepCopy(),
				},
				WithReactors: []ReactionFunc{
					InduceFailure("get", "TestResource"),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				r := &resources.TestResource{}
				err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: "resource-1"}, r)
				if err == nil {
					t.Errorf("expected get error")
				}
			},
			failedAssertions: []string{},
		},
		"list given object": {
			config: ExpectConfig{
				GivenObjects: []client.Object{
					r1.DeepCopy(),
					r2.DeepCopy(),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				r := &resources.TestResourceList{}
				err := c.List(ctx, r)
				if err != nil {
					t.Errorf("unexpected get error: %s", err)
				}
				if len(r.Items) != 2 {
					t.Errorf("listed unexpected objects")
				}
			},
			failedAssertions: []string{},
		},

		"get api given object": {
			config: ExpectConfig{
				APIGivenObjects: []client.Object{
					r1.DeepCopy(),
					r2.DeepCopy(),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				r := &resources.TestResource{}
				err := c.APIReader.Get(ctx, client.ObjectKey{Namespace: ns, Name: "resource-1"}, r)
				if err != nil {
					t.Errorf("unexpected get error: %s", err)
				}
				if r.Namespace != ns || r.Name != "resource-1" {
					t.Errorf("got unexpected object")
				}
			},
			failedAssertions: []string{},
		},
		"list api given object": {
			config: ExpectConfig{
				APIGivenObjects: []client.Object{
					r1.DeepCopy(),
					r2.DeepCopy(),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				r := &resources.TestResourceList{}
				err := c.APIReader.List(ctx, r)
				if err != nil {
					t.Errorf("unexpected get error: %s", err)
				}
				if len(r.Items) != 2 {
					t.Errorf("listed unexpected objects")
				}
			},
			failedAssertions: []string{},
		},

		"given apiresources": {
			config: ExpectConfig{
				GivenAPIResources: []*metav1.APIResourceList{
					{
						TypeMeta:     metav1.TypeMeta{APIVersion: "testing.reconciler.runtime/v1"},
						GroupVersion: "testing.reconciler.runtime/v1",
						APIResources: []metav1.APIResource{
							{
								Name:         "testresources",
								SingularName: "testresource",
								Namespaced:   true,
								Group:        "testing.reconciler.runtime",
								Version:      "v1",
								Kind:         "TestResource",
							},
						},
					},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				gvk, err := c.RESTMapper().KindFor(schema.GroupVersionResource{Group: "testing.reconciler.runtime", Version: "v1", Resource: "testresources"})
				if err != nil {
					t.Fatalf("unexpected error from KindFor: %s", err)
				}
				if expected, actual := "TestResource", gvk.Kind; expected != actual {
					t.Errorf("unexpected kind: expected %q, actual %q", expected, actual)
				}

				resources, err := c.Discovery.ServerResourcesForGroupVersion("testing.reconciler.runtime/v1")
				if err != nil {
					t.Fatalf("unexpected error from ServerResourcesForGroupVersion: %s", err)
				}
				if expected, actual := 1, len(resources.APIResources); expected != actual {
					t.Errorf("unexpected APIResources length: expected %q, actual %q", expected, actual)
				}
				if expected, actual := "TestResource", resources.APIResources[0].Kind; expected != actual {
					t.Errorf("unexpected kind: expected %q, actual %q", expected, actual)
				}
			},
		},

		"given track": {
			config: ExpectConfig{
				GivenTracks: []TrackRequest{
					NewTrackRequest(r2, r1, scheme),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				actual, _ := c.Tracker.GetObservers(r2)
				expected := []types.NamespacedName{
					{Namespace: r1.Namespace, Name: r1.Name},
				}
				if diff := cmp.Diff(expected, actual); diff != "" {
					t.Errorf("unexpected value (-expected, +actual): %s", diff)
				}
			},
			failedAssertions: []string{},
		},
		"expected track": {
			config: ExpectConfig{
				ExpectTracks: []TrackRequest{
					NewTrackRequest(r2, r1, scheme),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Tracker.TrackObject(r2, r1)
			},
			failedAssertions: []string{},
		},
		"unexpected track": {
			config: ExpectConfig{
				ExpectTracks: []TrackRequest{
					NewTrackRequest(r1, r2, scheme),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Tracker.TrackObject(r2, r1)
			},
			failedAssertions: []string{
				`ExpectTracks[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra track": {
			config: ExpectConfig{
				ExpectTracks: []TrackRequest{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Tracker.TrackObject(r2, r1)
			},
			failedAssertions: []string{
				`Unexpected Track observed for config "test": {my-namespace/resource-1 { /} {testing.reconciler.runtime TestResource my-namespace resource-2 <nil>}}`,
			},
		},
		"missing track": {
			config: ExpectConfig{
				ExpectTracks: []TrackRequest{
					NewTrackRequest(r2, r1, scheme),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectTracks[0] not observed for config "test": {my-namespace/resource-1 { /} {testing.reconciler.runtime TestResource my-namespace resource-2 <nil>}}`,
			},
		},

		"expected event": {
			config: ExpectConfig{
				ExpectEvents: []Event{
					NewEvent(r1, scheme, corev1.EventTypeNormal, "TheReason", "the message"),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Recorder.Eventf(r1, corev1.EventTypeNormal, "TheReason", "the message")
			},
			failedAssertions: []string{},
		},
		"unexpected event": {
			config: ExpectConfig{
				ExpectEvents: []Event{
					NewEvent(r1, scheme, corev1.EventTypeNormal, "TheReason", "the message"),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Recorder.Eventf(r2, corev1.EventTypeNormal, "TheReason", "the message")
			},
			failedAssertions: []string{
				`ExpectEvents[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra event": {
			config: ExpectConfig{
				ExpectEvents: []Event{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Recorder.Eventf(r1, corev1.EventTypeNormal, "TheReason", "the message")
			},
			failedAssertions: []string{
				`Unexpected Event observed for config "test": `,
			},
		},
		"missing event": {
			config: ExpectConfig{
				ExpectEvents: []Event{
					NewEvent(r1, scheme, corev1.EventTypeNormal, "TheReason", "the message"),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectEvents[0] not observed for config "test": `,
			},
		},

		"expected create": {
			config: ExpectConfig{
				ExpectCreates: []client.Object{
					r1,
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Create(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{},
		},
		"unexpected create": {
			config: ExpectConfig{
				ExpectCreates: []client.Object{
					r2,
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Create(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{
				`ExpectCreates[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra create": {
			config: ExpectConfig{
				ExpectCreates: []client.Object{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Create(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{
				`Unexpected Create observed for config "test": `,
			},
		},
		"missing create": {
			config: ExpectConfig{
				ExpectCreates: []client.Object{
					r1,
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectCreates[0] not observed for config "test": `,
			},
		},
		"generate name": {
			config: ExpectConfig{
				ExpectCreates: []client.Object{
					&resources.TestResource{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:    ns,
							GenerateName: "resource-",
						},
					},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				r := &resources.TestResource{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:    ns,
						GenerateName: "resource-",
					},
				}

				c.Create(ctx, r)
			},
		},
		"generate name unstructured": {
			config: ExpectConfig{
				ExpectCreates: []client.Object{
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "testing.reconciler.runtime/v1",
							"kind":       "TestResource",
							"metadata": map[string]interface{}{
								"namespace":    ns,
								"generateName": "resource-",
							},
						},
					},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				u := &unstructured.Unstructured{}
				u.SetAPIVersion("testing.reconciler.runtime/v1")
				u.SetKind("TestResource")
				u.SetNamespace(ns)
				u.SetGenerateName("resource-")

				c.Create(ctx, u)
			},
		},

		"expected apply": {
			config: ExpectConfig{
				ExpectApplies: []ApplyRef{
					{
						Group:              "apps",
						Kind:               "Deployment",
						Namespace:          ns,
						Name:               "resource",
						ApplyConfiguration: ac1,
					},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Apply(ctx, ac1)
			},
			failedAssertions: []string{},
		},
		"unexpected apply": {
			config: ExpectConfig{
				ExpectApplies: []ApplyRef{
					{
						Group:              "apps",
						Kind:               "Deployment",
						Namespace:          ns,
						Name:               "resource",
						ApplyConfiguration: ac2,
					},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Apply(ctx, ac1)
			},
			failedAssertions: []string{
				`ExpectApplies[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra apply": {
			config: ExpectConfig{
				ExpectApplies: []ApplyRef{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Apply(ctx, ac1)
			},
			failedAssertions: []string{
				`Unexpected Apply observed for config "test": `,
			},
		},
		"missing apply": {
			config: ExpectConfig{
				ExpectApplies: []ApplyRef{
					{
						Group:              "apps",
						Kind:               "Deployment",
						Namespace:          ns,
						Name:               "resource",
						ApplyConfiguration: ac1,
					},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectApplies[0] not observed for config "test": `,
			},
		},

		"expected update": {
			config: ExpectConfig{
				ExpectUpdates: []client.Object{
					r1,
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Update(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{},
		},
		"unexpected update": {
			config: ExpectConfig{
				ExpectUpdates: []client.Object{
					r2,
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Update(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{
				`ExpectUpdates[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra update": {
			config: ExpectConfig{
				ExpectUpdates: []client.Object{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Update(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{
				`Unexpected Update observed for config "test": `,
			},
		},
		"missing update": {
			config: ExpectConfig{
				ExpectUpdates: []client.Object{
					r1,
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectUpdates[0] not observed for config "test": `,
			},
		},

		"expected patch": {
			config: ExpectConfig{
				ExpectPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", PatchType: types.MergePatchType, Patch: []byte(`{"status":{"fields":{"foo":"bar"}}}`)},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
			},
			failedAssertions: []string{},
		},
		"unexpected patch": {
			config: ExpectConfig{
				ExpectPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", PatchType: types.MergePatchType, Patch: []byte(`{}`)},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
			},
			failedAssertions: []string{
				`ExpectPatches[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra patch": {
			config: ExpectConfig{
				ExpectPatches: []PatchRef{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
			},
			failedAssertions: []string{
				`Unexpected Patch observed for config "test": `,
			},
		},
		"missing patch": {
			config: ExpectConfig{
				ExpectPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", PatchType: types.MergePatchType, Patch: []byte(`{"status":{"fields":{"foo":"bar"}}}`)},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectPatches[0] not observed for config "test": `,
			},
		},

		"expected delete": {
			config: ExpectConfig{
				ExpectDeletes: []DeleteRef{
					NewDeleteRefFromObject(r1, scheme),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Delete(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{},
		},
		"unexpected delete": {
			config: ExpectConfig{
				ExpectDeletes: []DeleteRef{
					NewDeleteRefFromObject(r2, scheme),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Delete(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{
				`ExpectDeletes[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra delete": {
			config: ExpectConfig{
				ExpectDeletes: []DeleteRef{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Delete(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{
				`Unexpected Delete observed for config "test": `,
			},
		},
		"missing delete": {
			config: ExpectConfig{
				ExpectDeletes: []DeleteRef{
					NewDeleteRefFromObject(r1, scheme),
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectDeletes[0] not observed for config "test": `,
			},
		},

		"expected delete collection": {
			config: ExpectConfig{
				ExpectDeleteCollections: []DeleteCollectionRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource"},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.DeleteAllOf(ctx, &resources.TestResource{})
			},
			failedAssertions: []string{},
		},
		"expected delete collection with label selector": {
			config: ExpectConfig{
				ExpectDeleteCollections: []DeleteCollectionRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Labels: labels.SelectorFromSet(labels.Set{"foo": "bar"})},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.DeleteAllOf(ctx, &resources.TestResource{}, client.MatchingLabels{"foo": "bar"})
			},
			failedAssertions: []string{},
		},
		"expected delete collection with field selector": {
			config: ExpectConfig{
				ExpectDeleteCollections: []DeleteCollectionRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Fields: fields.SelectorFromSet(fields.Set{".metadata.name": "bar"})},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.DeleteAllOf(ctx, &resources.TestResource{}, client.MatchingFields{".metadata.name": "bar"})
			},
			failedAssertions: []string{},
		},
		"unexpected delete collection": {
			config: ExpectConfig{
				ExpectDeleteCollections: []DeleteCollectionRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource"},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.DeleteAllOf(ctx, &resources.TestResourceNoStatus{})
			},
			failedAssertions: []string{
				`ExpectDeleteCollections[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra delete collection": {
			config: ExpectConfig{
				ExpectDeleteCollections: []DeleteCollectionRef{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.DeleteAllOf(ctx, &resources.TestResource{})
			},
			failedAssertions: []string{
				`Unexpected DeleteCollection observed for config "test": `,
			},
		},
		"missing delete collection": {
			config: ExpectConfig{
				ExpectDeleteCollections: []DeleteCollectionRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource"},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectDeleteCollections[0] not observed for config "test": `,
			},
		},

		"expected status update": {
			config: ExpectConfig{
				ExpectStatusUpdates: []client.Object{
					r1,
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Status().Update(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{},
		},
		"unexpected status update": {
			config: ExpectConfig{
				ExpectStatusUpdates: []client.Object{
					r1patch,
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Status().Update(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{
				`ExpectStatusUpdates[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra status update": {
			config: ExpectConfig{
				ExpectStatusUpdates: []client.Object{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Status().Update(ctx, r1.DeepCopy())
			},
			failedAssertions: []string{
				`Unexpected StatusUpdate observed for config "test": `,
			},
		},
		"missing status update": {
			config: ExpectConfig{
				ExpectStatusUpdates: []client.Object{
					r1,
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectStatusUpdates[0] not observed for config "test": `,
			},
		},

		"expected status patch": {
			config: ExpectConfig{
				ExpectStatusPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", SubResource: "status", PatchType: types.MergePatchType, Patch: []byte(`{"status":{"fields":{"foo":"bar"}}}`)},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Status().Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
			},
			failedAssertions: []string{},
		},
		"unexpected status patch": {
			config: ExpectConfig{
				ExpectStatusPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", SubResource: "status", PatchType: types.MergePatchType, Patch: []byte(`{}`)},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Status().Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
			},
			failedAssertions: []string{
				`ExpectStatusPatches[0] differs for config "test" (-expected, +actual):`,
			},
		},
		"extra status patch": {
			config: ExpectConfig{
				ExpectStatusPatches: []PatchRef{},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Status().Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
			},
			failedAssertions: []string{
				`Unexpected StatusPatch observed for config "test": `,
			},
		},
		"missing status patch": {
			config: ExpectConfig{
				ExpectStatusPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", SubResource: "status", PatchType: types.MergePatchType, Patch: []byte(`{"status":{"fields":{"foo":"bar"}}}`)},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {},
			failedAssertions: []string{
				`ExpectStatusPatches[0] not observed for config "test": `,
			},
		},

		"custom diff - always different": {
			config: ExpectConfig{
				Differ: &staticDiffer{
					diff: "always different",
				},
				ExpectTracks: []TrackRequest{
					NewTrackRequest(r2, r1, scheme),
				},
				ExpectEvents: []Event{
					NewEvent(r1, scheme, corev1.EventTypeNormal, "TheReason", "the message"),
				},
				ExpectCreates: []client.Object{
					r1,
				},
				ExpectUpdates: []client.Object{
					r1,
				},
				ExpectPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", PatchType: types.MergePatchType, Patch: []byte(`{"status":{"fields":{"foo":"bar"}}}`)},
				},
				ExpectDeletes: []DeleteRef{
					NewDeleteRefFromObject(r1, scheme),
				},
				ExpectDeleteCollections: []DeleteCollectionRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource"},
				},
				ExpectStatusUpdates: []client.Object{
					r1,
				},
				ExpectStatusPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", SubResource: "status", PatchType: types.MergePatchType, Patch: []byte(`{"status":{"fields":{"foo":"bar"}}}`)},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Tracker.TrackObject(r2, r1)
				c.Recorder.Eventf(r1, corev1.EventTypeNormal, "TheReason", "the message")
				c.Create(ctx, r1.DeepCopy())
				c.Update(ctx, r1.DeepCopy())
				c.Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
				c.Delete(ctx, r1.DeepCopy())
				c.DeleteAllOf(ctx, &resources.TestResourceNoStatus{})
				c.Status().Update(ctx, r1.DeepCopy())
				c.Status().Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
			},
			failedAssertions: []string{
				"ExpectCreates[0] differs for config \"test\" (-expected, +actual):\nalways different\n",
				"ExpectUpdates[0] differs for config \"test\" (-expected, +actual):\nalways different\n",
				"ExpectPatches[0] differs for config \"test\" (-expected, +actual):\nalways different\n",
				"ExpectDeletes[0] differs for config \"test\" (-expected, +actual):\nalways different\n",
				"ExpectDeleteCollections[0] differs for config \"test\" (-expected, +actual):\nalways different\n",
				"ExpectStatusUpdates[0] differs for config \"test\" (-expected, +actual):\nalways different\n",
				"ExpectStatusPatches[0] differs for config \"test\" (-expected, +actual):\nalways different\n",
				"ExpectEvents[0] differs for config \"test\" (-expected, +actual):\nalways different\n",
				"ExpectTracks[0] differs for config \"test\" (-expected, +actual):\nalways different\n",
			},
		},
		"custom diff - never different": {
			config: ExpectConfig{
				Differ: &staticDiffer{
					diff: "",
				},
				ExpectTracks: []TrackRequest{
					NewTrackRequest(r2, r1, scheme),
				},
				ExpectEvents: []Event{
					NewEvent(r1, scheme, corev1.EventTypeNormal, "TheReason", "the message"),
				},
				ExpectCreates: []client.Object{
					r1,
				},
				ExpectUpdates: []client.Object{
					r1,
				},
				ExpectPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", PatchType: types.MergePatchType, Patch: []byte(`{"status":{"fields":{"foo":"bar"}}}`)},
				},
				ExpectDeletes: []DeleteRef{
					NewDeleteRefFromObject(r1, scheme),
				},
				ExpectDeleteCollections: []DeleteCollectionRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource"},
				},
				ExpectStatusUpdates: []client.Object{
					r1,
				},
				ExpectStatusPatches: []PatchRef{
					{Group: "testing.reconciler.runtime", Kind: "TestResource", Namespace: ns, Name: "resource-1", SubResource: "status", PatchType: types.MergePatchType, Patch: []byte(`{"status":{"fields":{"foo":"bar"}}}`)},
				},
			},
			operation: func(t *testing.T, ctx context.Context, c reconcilers.Config) {
				c.Tracker.TrackObject(r2, r1)
				c.Recorder.Eventf(r1, corev1.EventTypeNormal, "TheReason", "the message")
				c.Create(ctx, r1.DeepCopy())
				c.Update(ctx, r1.DeepCopy())
				c.Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
				c.Delete(ctx, r1.DeepCopy())
				c.DeleteAllOf(ctx, &resources.TestResourceNoStatus{})
				c.Status().Update(ctx, r1.DeepCopy())
				c.Status().Patch(ctx, r1patch.DeepCopy(), client.MergeFrom(r1))
			},
			failedAssertions: []string{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := tc.config
			c.Name = "test"
			c.Scheme = scheme
			ctx := context.Background()
			tc.operation(t, ctx, c.Config())
			c.AssertExpectations(nil)

			if expected, actual := len(tc.failedAssertions), len(c.observedErrors); expected != actual {
				t.Errorf("unexpected config assertions, wanted %d, got %d: %#v", expected, actual, c.observedErrors)
			}
			for i := range tc.failedAssertions {
				expected, actual := tc.failedAssertions[i], c.observedErrors[i]
				if !strings.HasPrefix(actual, expected) {
					t.Errorf("unexpected config assertions: expected prefix %q, actual %q", expected, actual)
				}
			}
		})
	}
}

func TestIgnoreLastTransitionTime(t *testing.T) {
	a := diemetav1.ConditionBlank.
		Type("Ready").
		Status(metav1.ConditionTrue).
		Reason("AllGood").
		LastTransitionTime(metav1.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC))
	b := a.LastTransitionTime(metav1.Date(2022, 10, 10, 0, 0, 0, 0, time.UTC))

	objA := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace("default")
			d.Name("my-resource")
			d.CreationTimestamp(metav1.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC))
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(a)
		})
	objB := objA.
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(b)
		})

	tests := map[string]struct {
		a       interface{}
		b       interface{}
		hasDiff bool
	}{
		"nil": {
			a: nil,
			b: nil,
		},
		"metav1 condition": {
			a: a.DieRelease(),
			b: b.DieRelease(),
		},
		"object": {
			a: objA.DieReleasePtr(),
			b: objB.DieReleasePtr(),
		},
		"unstructured": {
			a: objA.DieReleaseUnstructured(),
			b: objB.DieReleaseUnstructured(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			diff := cmp.Diff(tc.a, tc.b, IgnoreLastTransitionTime)
			actual := diff != ""
			expected := tc.hasDiff
			if actual != expected {
				t.Errorf("unexpected diff: %s", diff)
			}
		})
	}
}

func TestIgnoreTypeMeta(t *testing.T) {
	a := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace("default")
			d.Name("my-resource")
			d.CreationTimestamp(metav1.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC))
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.
					Type("Ready").
					Status(metav1.ConditionTrue).
					Reason("AllGood").
					LastTransitionTime(metav1.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)),
			)
		})
	b := a.
		APIVersion(resources.GroupVersion.String()).
		Kind("TestResource")

	tests := map[string]struct {
		a       interface{}
		b       interface{}
		hasDiff bool
	}{
		"nil": {
			a: nil,
			b: nil,
		},
		"object": {
			a: a.DieReleasePtr(),
			b: b.DieReleasePtr(),
		},
		"unstructured": {
			a:       a.DieReleaseUnstructured(),
			b:       b.DieReleaseUnstructured(),
			hasDiff: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			diff := cmp.Diff(tc.a, tc.b, IgnoreTypeMeta)
			actual := diff != ""
			expected := tc.hasDiff
			if actual != expected {
				t.Errorf("unexpected diff: %s", diff)
			}
		})
	}
}

func TestIgnoreCreationTimestamp(t *testing.T) {
	a := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace("default")
			d.Name("my-resource")
			d.CreationTimestamp(metav1.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC))
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.
					Type("Ready").
					Status(metav1.ConditionTrue).
					Reason("AllGood").
					LastTransitionTime(metav1.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)),
			)
		})
	b := a.MetadataDie(func(d *diemetav1.ObjectMetaDie) {
		d.CreationTimestamp(metav1.Date(2022, 10, 10, 0, 0, 0, 0, time.UTC))
	})

	tests := map[string]struct {
		a       interface{}
		b       interface{}
		hasDiff bool
	}{
		"nil": {
			a: nil,
			b: nil,
		},
		"object": {
			a: a.DieReleasePtr(),
			b: b.DieReleasePtr(),
		},
		"unstructured": {
			a: a.DieReleaseUnstructured(),
			b: b.DieReleaseUnstructured(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			diff := cmp.Diff(tc.a, tc.b, IgnoreCreationTimestamp)
			actual := diff != ""
			expected := tc.hasDiff
			if actual != expected {
				t.Errorf("unexpected diff: %s", diff)
			}
		})
	}
}

func TestIgnoreResourceVersion(t *testing.T) {
	a := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace("default")
			d.Name("my-resource")
			d.ResourceVersion("999")
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.
					Type("Ready").
					Status(metav1.ConditionTrue).
					Reason("AllGood").
					LastTransitionTime(metav1.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)),
			)
		})
	b := a.MetadataDie(func(d *diemetav1.ObjectMetaDie) {
		d.ResourceVersion("1000")
	})

	tests := map[string]struct {
		a       interface{}
		b       interface{}
		hasDiff bool
	}{
		"nil": {
			a: nil,
			b: nil,
		},
		"object": {
			a: a.DieReleasePtr(),
			b: b.DieReleasePtr(),
		},
		"unstructured": {
			a: a.DieReleaseUnstructured(),
			b: b.DieReleaseUnstructured(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			diff := cmp.Diff(tc.a, tc.b, IgnoreResourceVersion)
			actual := diff != ""
			expected := tc.hasDiff
			if actual != expected {
				t.Errorf("unexpected diff: %s", diff)
			}
		})
	}
}

func TestNormalizeApplyConfiguration(t *testing.T) {
	ac1 := applyconfigurationsappsv1.Deployment("resource", "test-ns").
		WithSpec(applyconfigurationsappsv1.DeploymentSpec().WithReplicas(1))
	ac2 := applyconfigurationsappsv1.Deployment("resource", "test-ns").
		WithSpec(applyconfigurationsappsv1.DeploymentSpec().WithReplicas(2))

	tests := map[string]struct {
		a       runtime.ApplyConfiguration
		b       runtime.ApplyConfiguration
		hasDiff bool
	}{
		"nil": {
			a: nil,
			b: nil,
		},
		"same": {
			a: ac1,
			b: ac1,
		},
		"delta": {
			a:       ac1,
			b:       ac2,
			hasDiff: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			diff := cmp.Diff(tc.a, tc.b, NormalizeApplyConfiguration)
			actual := diff != ""
			expected := tc.hasDiff
			if actual != expected {
				t.Errorf("unexpected diff: %s", diff)
			}
		})
	}
}
