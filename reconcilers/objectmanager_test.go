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
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

func TestUpdatingObjectManager(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testFinalizer := "test-finalizer"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	now := metav1.Time{Time: time.Now().Truncate(time.Second)}

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

	desiredConfigMap := diecorev1.ConfigMapBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Name(testName)
			d.Namespace(testNamespace)
		}).
		AddData("hello", "world")
	givenConfigMap := desiredConfigMap.MetadataDie(func(d *diemetav1.ObjectMetaDie) {
		d.CreationTimestamp(now)
	})

	makeUpdatingObjectManager := func(modifiers ...func(*reconcilers.UpdatingObjectManager[*corev1.ConfigMap])) *reconcilers.UpdatingObjectManager[*corev1.ConfigMap] {
		om := &reconcilers.UpdatingObjectManager[*corev1.ConfigMap]{
			MergeBeforeUpdate: func(current, desired *corev1.ConfigMap) {
				current.Labels = desired.Labels
				current.Data = desired.Data
			},
		}
		for i := range modifiers {
			modifiers[i](om)
		}
		return om
	}
	withFinalizer := func(finalizer string) func(*reconcilers.UpdatingObjectManager[*corev1.ConfigMap]) {
		return func(om *reconcilers.UpdatingObjectManager[*corev1.ConfigMap]) {
			om.Finalizer = finalizer
		}
	}
	withTrackDesired := func(trackDesired bool) func(*reconcilers.UpdatingObjectManager[*corev1.ConfigMap]) {
		return func(om *reconcilers.UpdatingObjectManager[*corev1.ConfigMap]) {
			om.TrackDesired = trackDesired
		}
	}
	withHarmonizeImmutableFields := func(harmonizeImmutableFields func(*corev1.ConfigMap, *corev1.ConfigMap)) func(*reconcilers.UpdatingObjectManager[*corev1.ConfigMap]) {
		return func(om *reconcilers.UpdatingObjectManager[*corev1.ConfigMap]) {
			om.HarmonizeImmutableFields = harmonizeImmutableFields
		}
	}

	actualStashKey := rtesting.ObjectManagerReconcilerTestHarnessActualStasher[*corev1.ConfigMap]().Key()
	desiredStashKey := rtesting.ObjectManagerReconcilerTestHarnessDesiredStasher[*corev1.ConfigMap]().Key()
	resultStashKey := rtesting.ObjectManagerReconcilerTestHarnessResultStasher[*corev1.ConfigMap]().Key()

	rts := rtesting.SubReconcilerTests[client.Object]{
		"in sync": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey:  givenConfigMap.DieReleasePtr(),
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: givenConfigMap.DieReleasePtr(),
			},
		},
		"missing and desired": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey:  nil,
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Created", `Created ConfigMap %q`, testName),
			},
			ExpectCreates: []client.Object{
				desiredConfigMap,
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: desiredConfigMap.DieReleasePtr(),
			},
		},
		"missing and desired, blank actual": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey:  diecorev1.ConfigMapBlank.DieDefaultTypeMetadata().DieReleasePtr(),
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Created", `Created ConfigMap %q`, testName),
			},
			ExpectCreates: []client.Object{
				desiredConfigMap,
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: desiredConfigMap.DieReleasePtr(),
			},
		},
		"missing and desired, errored": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey:  nil,
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap"),
			},
			ShouldErr: true,
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeWarning, "CreationFailed", `Failed to create ConfigMap %q: inducing failure for create ConfigMap`, testName),
			},
			ExpectCreates: []client.Object{
				desiredConfigMap,
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: nil,
			},
		},
		"correct drift": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey: givenConfigMap.
					AddData("foo", "bar").
					DieReleasePtr(),
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Updated", `Updated ConfigMap %q`, testName),
			},
			ExpectUpdates: []client.Object{
				desiredConfigMap,
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: desiredConfigMap.DieReleasePtr(),
			},
		},
		"correct drift, errored": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey: givenConfigMap.
					AddData("foo", "bar").
					DieReleasePtr(),
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("update", "ConfigMap"),
			},
			ShouldErr: true,
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeWarning, "UpdateFailed", `Failed to update ConfigMap %q: inducing failure for update ConfigMap`, testName),
			},
			ExpectUpdates: []client.Object{
				desiredConfigMap,
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: nil,
			},
		},
		"cleanup": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey: givenConfigMap.
					AddData("foo", "bar").
					DieReleasePtr(),
				desiredStashKey: nil,
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Deleted", `Deleted ConfigMap %q`, testName),
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(givenConfigMap, scheme),
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: nil,
			},
		},
		"cleanup, errored": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey: givenConfigMap.
					AddData("foo", "bar").
					DieReleasePtr(),
				desiredStashKey: nil,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("delete", "ConfigMap"),
			},
			ShouldErr: true,
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeWarning, "DeleteFailed", `Failed to delete ConfigMap %q: inducing failure for delete ConfigMap`, testName),
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(givenConfigMap, scheme),
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: nil,
			},
		},
		"in sync with finalizer": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(
					withFinalizer(testFinalizer),
				),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey:  givenConfigMap.DieReleasePtr(),
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: givenConfigMap.DieReleasePtr(),
			},
		},
		"in sync missing finalizer": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(
					withFinalizer(testFinalizer),
				),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey:  givenConfigMap.DieReleasePtr(),
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			ExpectResource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers(testFinalizer)
					d.ResourceVersion("1000")
				}).
				DieReleasePtr(),
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "FinalizerPatched", `Patched finalizer %q`, testFinalizer),
			},
			ExpectPatches: []rtesting.PatchRef{
				{
					Group:       "testing.reconciler.runtime",
					Kind:        "TestResource",
					Namespace:   testNamespace,
					Name:        testName,
					SubResource: "",
					PatchType:   types.MergePatchType,
					Patch:       []byte(`{"metadata":{"finalizers":["test-finalizer"],"resourceVersion":"999"}}`),
				},
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: givenConfigMap.DieReleasePtr(),
			},
		},
		"cleanup finalizer": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(
					withFinalizer(testFinalizer),
				),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey:  nil,
				desiredStashKey: nil,
			},
			ExpectResource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers()
					d.ResourceVersion("1000")
				}).
				DieReleasePtr(),
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "FinalizerPatched", `Patched finalizer %q`, testFinalizer),
			},
			ExpectPatches: []rtesting.PatchRef{
				{
					Group:       "testing.reconciler.runtime",
					Kind:        "TestResource",
					Namespace:   testNamespace,
					Name:        testName,
					SubResource: "",
					PatchType:   types.MergePatchType,
					Patch:       []byte(`{"metadata":{"finalizers":null,"resourceVersion":"999"}}`),
				},
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: nil,
			},
		},
		"track given": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(
					withTrackDesired(true),
				),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey:  givenConfigMap.DieReleasePtr(),
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(givenConfigMap, resource, scheme),
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: givenConfigMap.DieReleasePtr(),
			},
		},
		"track generated names": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(
					withTrackDesired(true),
				),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey: nil,
				desiredStashKey: desiredConfigMap.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("")
						d.GenerateName(testName + "-")
					}).
					DieReleasePtr(),
			},
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(givenConfigMap.MetadataDie(func(d *diemetav1.ObjectMetaDie) { d.Name(testName + "-001") }), resource, scheme),
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Created", `Created ConfigMap %q`, testName+"-001"),
			},
			ExpectCreates: []client.Object{
				desiredConfigMap.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name("")
						d.GenerateName(testName + "-")
					}),
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: desiredConfigMap.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Name(testName + "-001")
						d.GenerateName(testName + "-")
					}).
					DieReleasePtr(),
			},
		},
		"ignore drift in immutable fields": rtesting.SubReconcilerTestCase[client.Object]{
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]any{
				"ObjectManager": makeUpdatingObjectManager(
					withHarmonizeImmutableFields(func(actual, desired *corev1.ConfigMap) {
						if actual.Immutable != nil && *actual.Immutable {
							// data is immutable, align desired with actual
							desired.Data = actual.Data
						}
					}),
				),
			},
			GivenStashedValues: map[reconcilers.StashKey]any{
				actualStashKey: givenConfigMap.
					Immutable(ptr.To[bool](true)).
					AddData("foo", "bar").
					DieReleasePtr(),
				desiredStashKey: desiredConfigMap.DieReleasePtr(),
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				resultStashKey: givenConfigMap.
					Immutable(ptr.To[bool](true)).
					AddData("foo", "bar").
					DieReleasePtr(),
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[client.Object], c reconcilers.Config) reconcilers.SubReconciler[client.Object] {
		return &rtesting.ObjectManagerReconcilerTestHarness[*corev1.ConfigMap]{
			ObjectManager: rtc.Metadata["ObjectManager"].(reconcilers.ObjectManager[*corev1.ConfigMap]),
		}
	})

}

func TestPatch(t *testing.T) {
	tests := []struct {
		name           string
		base           client.Object
		update         client.Object
		rebase         client.Object
		expected       client.Object
		newShouldErr   bool
		applyShouldErr bool
	}{
		{
			name:     "identity",
			base:     &corev1.Pod{},
			update:   &corev1.Pod{},
			rebase:   &corev1.Pod{},
			expected: &corev1.Pod{},
		},
		{
			name: "rebase",
			base: &corev1.Pod{},
			update: &corev1.Pod{
				Spec: corev1.PodSpec{
					DNSPolicy: corev1.DNSClusterFirst,
				},
			},
			rebase: &corev1.Pod{
				Spec: corev1.PodSpec{
					DNSConfig: &corev1.PodDNSConfig{
						Nameservers: []string{"1.1.1.1"},
					},
				},
			},
			expected: &corev1.Pod{
				Spec: corev1.PodSpec{
					DNSConfig: &corev1.PodDNSConfig{
						Nameservers: []string{"1.1.1.1"},
					},
					DNSPolicy: corev1.DNSClusterFirst,
				},
			},
		},
		{
			name:         "bad base",
			base:         &boom{ShouldErr: true},
			update:       &boom{},
			newShouldErr: true,
		},
		{
			name:         "bad update",
			base:         &boom{},
			update:       &boom{ShouldErr: true},
			newShouldErr: true,
		},
		{
			name:           "bad rebase",
			base:           &boom{},
			update:         &boom{},
			rebase:         &boom{ShouldErr: true},
			applyShouldErr: true,
		},
		{
			name:   "generation mismatch",
			base:   &corev1.Pod{},
			update: &corev1.Pod{},
			rebase: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			expected:       &corev1.Pod{},
			applyShouldErr: true,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			patch, newErr := reconcilers.NewPatch(c.base, c.update)
			if actual, expected := newErr != nil, c.newShouldErr; actual != expected {
				t.Errorf("%s: unexpected new error, actually = %v, expected = %v", c.name, actual, expected)
			}
			if c.newShouldErr {
				return
			}

			applyErr := patch.Apply(c.rebase)
			if actual, expected := applyErr != nil, c.applyShouldErr; actual != expected {
				t.Errorf("%s: unexpected apply error, actually = %v, expected = %v", c.name, actual, expected)
			}
			if c.applyShouldErr {
				return
			}

			if diff := cmp.Diff(c.expected, c.rebase); diff != "" {
				t.Errorf("%s: unexpected value (-expected, +actual): %s", c.name, diff)
			}
		})
	}
}

type boom struct {
	metav1.ObjectMeta `json:"metadata"`
	ShouldErr         bool `json:"shouldErr"`
}

func (b *boom) MarshalJSON() ([]byte, error) {
	if b.ShouldErr {
		return nil, fmt.Errorf("object asked to err")
	}
	return json.Marshal(b.ObjectMeta)
}

func (b *boom) UnmarshalJSON(data []byte) error {
	if b.ShouldErr {
		return fmt.Errorf("object asked to err")
	}
	return json.Unmarshal(data, b.ObjectMeta)
}

func (b *boom) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (b *boom) DeepCopyObject() runtime.Object {
	return nil
}
