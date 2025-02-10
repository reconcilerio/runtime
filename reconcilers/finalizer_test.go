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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	"reconciler.io/runtime/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestWithFinalizer(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testFinalizer := "test-finalizer"

	now := &metav1.Time{Time: time.Now().Truncate(time.Second)}

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		})

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"in sync": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Sync", ""),
			},
		},
		"add finalizer": {
			Resource: resource.DieReleasePtr(),
			ExpectResource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers(testFinalizer)
					d.ResourceVersion("1000")
				}).
				DieReleasePtr(),
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "FinalizerPatched",
					`Patched finalizer %q`, testFinalizer),
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Sync", ""),
			},
			ExpectPatches: []rtesting.PatchRef{
				{
					Group:     "testing.reconciler.runtime",
					Kind:      "TestResource",
					Namespace: testNamespace,
					Name:      testName,
					PatchType: types.MergePatchType,
					Patch:     []byte(`{"metadata":{"finalizers":["test-finalizer"],"resourceVersion":"999"}}`),
				},
			},
		},
		"error adding finalizer": {
			Resource: resource.DieReleasePtr(),
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("patch", "TestResource"),
			},
			ShouldErr: true,
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeWarning, "FinalizerPatchFailed",
					`Failed to patch finalizer %q: inducing failure for patch TestResource`, testFinalizer),
			},
			ExpectPatches: []rtesting.PatchRef{
				{
					Group:     "testing.reconciler.runtime",
					Kind:      "TestResource",
					Namespace: testNamespace,
					Name:      testName,
					PatchType: types.MergePatchType,
					Patch:     []byte(`{"metadata":{"finalizers":["test-finalizer"],"resourceVersion":"999"}}`),
				},
			},
		},
		"keep finalizer until ready": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			ExpectResource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Finalize", ""),
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "ReadyToClearFinalizer", "not ready"),
			},
			Metadata: map[string]interface{}{
				"ReadyToClearFinalizer": func(ctx context.Context, resource *resources.TestResource) bool {
					c := reconcilers.RetrieveConfigOrDie(ctx)
					c.Recorder.Event(resource, corev1.EventTypeNormal, "ReadyToClearFinalizer", "not ready")
					return false
				},
			},
		},
		"clear finalizer": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			ExpectResource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(now)
					d.ResourceVersion("1000")
				}).
				DieReleasePtr(),
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Finalize", ""),
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
					Patch:     []byte(`{"metadata":{"finalizers":null,"resourceVersion":"999"}}`),
				},
			},
		},
		"error clearing finalizer": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("patch", "TestResource"),
			},
			ShouldErr: true,
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Finalize", ""),
				rtesting.NewEvent(resource, scheme, corev1.EventTypeWarning, "FinalizerPatchFailed",
					`Failed to patch finalizer %q: inducing failure for patch TestResource`, testFinalizer),
			},
			ExpectPatches: []rtesting.PatchRef{
				{
					Group:     "testing.reconciler.runtime",
					Kind:      "TestResource",
					Namespace: testNamespace,
					Name:      testName,
					PatchType: types.MergePatchType,
					Patch:     []byte(`{"metadata":{"finalizers":null,"resourceVersion":"999"}}`),
				},
			},
		},
		"keep finalizer on error": {
			Resource: resource.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.DeletionTimestamp(now)
					d.Finalizers(testFinalizer)
				}).
				DieReleasePtr(),
			ShouldErr: true,
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "Finalize", ""),
			},
			Metadata: map[string]interface{}{
				"FinalizerError": fmt.Errorf("finalize error"),
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		var syncErr, finalizeErr error
		if err, ok := rtc.Metadata["SyncError"]; ok {
			syncErr = err.(error)
		}
		if err, ok := rtc.Metadata["FinalizerError"]; ok {
			finalizeErr = err.(error)
		}
		var readyToClearFinalizer func(context.Context, *resources.TestResource) bool
		if ready, ok := rtc.Metadata["ReadyToClearFinalizer"]; ok {
			readyToClearFinalizer = ready.(func(context.Context, *resources.TestResource) bool)
		}

		return &reconcilers.WithFinalizer[*resources.TestResource]{
			Finalizer:             testFinalizer,
			ReadyToClearFinalizer: readyToClearFinalizer,
			Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
				Sync: func(ctx context.Context, resource *resources.TestResource) error {
					c.Recorder.Event(resource, corev1.EventTypeNormal, "Sync", "")
					return syncErr
				},
				Finalize: func(ctx context.Context, resource *resources.TestResource) error {
					c.Recorder.Event(resource, corev1.EventTypeNormal, "Finalize", "")
					return finalizeErr
				},
			},
		}
	})
}

func TestWithFinalizer_Validate(t *testing.T) {
	tests := []struct {
		name           string
		resource       client.Object
		reconciler     *reconcilers.WithFinalizer[*corev1.ConfigMap]
		validateNested bool
		shouldErr      string
	}{
		{
			name:       "empty",
			resource:   &corev1.ConfigMap{},
			reconciler: &reconcilers.WithFinalizer[*corev1.ConfigMap]{},
			shouldErr:  `WithFinalizer "" must define Finalizer`,
		},
		{
			name:     "valid",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithFinalizer[*corev1.ConfigMap]{
				Reconciler: &reconcilers.Sequence[*corev1.ConfigMap]{},
				Finalizer:  "my-finalizer",
			},
		},
		{
			name:     "missing finalizer",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithFinalizer[*corev1.ConfigMap]{
				Name:       "missing finalizer",
				Reconciler: &reconcilers.Sequence[*corev1.ConfigMap]{},
			},
			shouldErr: `WithFinalizer "missing finalizer" must define Finalizer`,
		},
		{
			name:     "missing reconciler",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithFinalizer[*corev1.ConfigMap]{
				Name:      "missing reconciler",
				Finalizer: "my-finalizer",
			},
			shouldErr: `WithFinalizer "missing reconciler" must define Reconciler`,
		},
		{
			name:     "valid reconciler",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithFinalizer[*corev1.ConfigMap]{
				Finalizer: "my-finalizer",
				Reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
					Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name:     "invalid reconciler",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithFinalizer[*corev1.ConfigMap]{
				Finalizer:  "my-finalizer",
				Reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
					// Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr:      `WithFinalizer "" must have a valid Reconciler: SyncReconciler "" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.TODO()
			if c.validateNested {
				ctx = validation.WithRecursive(ctx)
			}
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
		})
	}
}
