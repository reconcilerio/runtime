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

package testing

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"reconciler.io/runtime/internal"
	"reconciler.io/runtime/reconcilers"
	"reconciler.io/runtime/stash"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ reconcilers.ObjectManager[client.Object] = (*StubObjectManager[client.Object])(nil)

// StubObjectManager is used for testing the orchestration of an ObjectManager's essential behavior
// without the full robustness of a real ObjectManager implementation.
//
// There are four possible outcomes:
// - create a desired resource, returning the created object
// - update an outdated resource, returning the updated object
// - delete an undesired resource, returning nil
// - do nothing when the resources are in sync, returning the actual object
//
// If any of the client calls error, the error is returned.
//
// There are no user configurable values. Merging of desired and actual objects is handled
// reflectively and may be unsafe for some resources.
type StubObjectManager[Type client.Object] struct{}

func (m *StubObjectManager[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	return nil
}

func (m *StubObjectManager[T]) Manage(ctx context.Context, resource client.Object, actual, desired T) (T, error) {
	var nilT T
	c := reconcilers.RetrieveConfigOrDie(ctx)

	if (internal.IsNil(actual) || actual.GetCreationTimestamp().Time.IsZero()) && internal.IsNil(desired) {
		// nothing to do
		return nilT, nil
	}
	if internal.IsNil(desired) {
		current := actual.DeepCopyObject().(T)
		if current.GetDeletionTimestamp() != nil {
			// object is already terminating
			return nilT, nil
		}
		if err := c.Delete(ctx, current); err != nil {
			return nilT, err
		}
		return nilT, nil
	}
	if internal.IsNil(actual) || actual.GetCreationTimestamp().Time.IsZero() {
		current := desired.DeepCopyObject().(T)
		if err := c.Create(ctx, current); err != nil {
			return nilT, err
		}
		return current, nil
	}
	current := actual.DeepCopyObject().(T)
	// merge desired into current
	m.unsafeMergeInto(current, desired)
	if !equality.Semantic.DeepEqual(actual, current) {
		if err := c.Update(ctx, current); err != nil {
			return nilT, err
		}
		return current, nil
	}

	return current, nil
}

func (m *StubObjectManager[T]) unsafeMergeInto(target, source T) {
	ut, err := runtime.DefaultUnstructuredConverter.ToUnstructured(target)
	if err != nil {
		panic(err)
	}
	us, err := runtime.DefaultUnstructuredConverter.ToUnstructured(source)
	if err != nil {
		panic(err)
	}

	// copy allowed top-level fields
	for k := range us {
		if k != "apiVersion" && k != "kind" && k != "metadata" && k != "status" {
			ut[k] = us[k]
		}
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(ut, target); err != nil {
		panic(err)
	}

	// copy allowed metadata fields
	target.SetAnnotations(source.GetAnnotations())
	target.SetLabels(source.GetLabels())
}

var _ reconcilers.SubReconciler[client.Object] = (*ObjectManagerReconcilerTestHarness[client.Object])(nil)

// ObjectManagerReconcilerTestHarness orchestrates an ObjectManager as a SubReconciler so that it
// can be easily tested within a SubReconcilerTestCase.
//
// The actual and desired objects must be defined in the stash via the
// ObjectManagerReconcilerTestHarnessActualStasher and
// ObjectManagerReconcilerTestHarnessDesiredStasher stashers. Failure to stash these values will
// panic the reconciler. The resulting object is stashed with
// ObjectManagerReconcilerTestHarnessResultStasher.
//
// The actual object, when defined, is automatically added as a given object.
type ObjectManagerReconcilerTestHarness[T client.Object] struct {
	ObjectManager reconcilers.ObjectManager[T]
}

func (h *ObjectManagerReconcilerTestHarness[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	return h.ObjectManager.SetupWithManager(ctx, mgr, bldr)
}

func (h *ObjectManagerReconcilerTestHarness[T]) Reconcile(ctx context.Context, resource client.Object) (reconcilers.Result, error) {
	actual := ObjectManagerReconcilerTestHarnessActualStasher[T]().RetrieveOrDie(ctx)
	desired := ObjectManagerReconcilerTestHarnessDesiredStasher[T]().RetrieveOrDie(ctx)
	if !internal.IsNil(actual) {
		c := reconcilers.RetrieveConfigOrDie(ctx)
		c.Client.(TestClient).AddGiven(actual)
	}

	result, err := h.ObjectManager.Manage(ctx, resource, actual, desired)
	ObjectManagerReconcilerTestHarnessResultStasher[T]().Store(ctx, result)
	return reconcilers.Result{}, err
}

func ObjectManagerReconcilerTestHarnessActualStasher[T client.Object]() stash.Stasher[T] {
	return stash.New[T]("reconciler.io/object-manager-reconciler-test-harness-actual")
}

func ObjectManagerReconcilerTestHarnessDesiredStasher[T client.Object]() stash.Stasher[T] {
	return stash.New[T]("reconciler.io/object-manager-reconciler-test-harness-desired")
}

func ObjectManagerReconcilerTestHarnessResultStasher[T client.Object]() stash.Stasher[T] {
	return stash.New[T]("reconciler.io/object-manager-reconciler-test-harness-result")
}
