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

package reconcilers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"reconciler.io/runtime/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ SubReconciler[client.Object] = (*WithFinalizer[client.Object])(nil)

// WithFinalizer ensures the resource being reconciled has the desired finalizer set so that state
// can be cleaned up upon the resource being deleted. The finalizer is added to the resource, if not
// already set, before calling the nested reconciler. When the resource is terminating, the
// finalizer is cleared after returning from the nested reconciler without error and
// ReadyToClearFinalizer returns true.
type WithFinalizer[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `WithFinalizer`.  Ideally unique, but
	// not required to be so.
	//
	// +optional
	Name string

	// Finalizer to set on the reconciled resource. The value must be unique to this specific
	// reconciler instance and not shared. Reusing a value may result in orphaned state when
	// the reconciled resource is deleted.
	//
	// Using a finalizer is encouraged when state needs to be manually cleaned up before a resource
	// is fully deleted. This commonly include state allocated outside of the current cluster.
	Finalizer string

	// ReadyToClearFinalizer must return true before the finalizer is cleared from the resource.
	// Only called when the resource is terminating.
	//
	// Defaults to always return true.
	//
	// +optional
	ReadyToClearFinalizer func(ctx context.Context, resource Type) bool

	// Reconciler is called for each reconciler request with the reconciled
	// resource being reconciled. Typically a Sequence is used to compose
	// multiple SubReconcilers.
	Reconciler SubReconciler[Type]

	initOnce sync.Once
}

func (r *WithFinalizer[T]) init() {
	r.initOnce.Do(func() {
		if r.Name == "" {
			r.Name = "WithFinalizer"
		}
		if r.ReadyToClearFinalizer == nil {
			r.ReadyToClearFinalizer = func(ctx context.Context, resource T) bool {
				return true
			}
		}
	})
}

func (r *WithFinalizer[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if err := r.Validate(ctx); err != nil {
		return err
	}
	return r.Reconciler.SetupWithManager(ctx, mgr, bldr)
}

func (r *WithFinalizer[T]) Validate(ctx context.Context) error {
	r.init()

	// validate Finalizer value
	if r.Finalizer == "" {
		return fmt.Errorf("WithFinalizer %q must define Finalizer", r.Name)
	}

	// validate Reconciler value
	if r.Reconciler == nil {
		return fmt.Errorf("WithFinalizer %q must define Reconciler", r.Name)
	}
	if validation.IsRecursive(ctx) {
		if v, ok := r.Reconciler.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("WithFinalizer %q must have a valid Reconciler: %w", r.Name, err)
			}
		}
	}

	return nil
}

func (r *WithFinalizer[T]) Reconcile(ctx context.Context, resource T) (Result, error) {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if resource.GetDeletionTimestamp() == nil {
		if err := AddFinalizer(ctx, resource, r.Finalizer); err != nil {
			return Result{}, err
		}
	}
	result, err := r.Reconciler.Reconcile(ctx, resource)
	if err != nil {
		return result, err
	}
	if resource.GetDeletionTimestamp() != nil && r.ReadyToClearFinalizer(ctx, resource) {
		if err := ClearFinalizer(ctx, resource, r.Finalizer); err != nil {
			return Result{}, err
		}
	}
	return result, err
}

// AddFinalizer ensures the desired finalizer exists on the reconciled resource. The client that
// loaded the reconciled resource is used to patch it with the finalizer if not already set.
func AddFinalizer(ctx context.Context, resource client.Object, finalizer string) error {
	return ensureFinalizer(ctx, resource, finalizer, true)
}

// ClearFinalizer ensures the desired finalizer does not exist on the reconciled resource. The
// client that loaded the reconciled resource is used to patch it with the finalizer if set.
func ClearFinalizer(ctx context.Context, resource client.Object, finalizer string) error {
	return ensureFinalizer(ctx, resource, finalizer, false)
}

func ensureFinalizer(ctx context.Context, current client.Object, finalizer string, add bool) error {
	if finalizer == "" || controllerutil.ContainsFinalizer(current, finalizer) == add {
		// nothing to do
		return nil
	}

	config := RetrieveOriginalConfigOrDie(ctx)
	log := logr.FromContextOrDiscard(ctx)

	desired := current.DeepCopyObject().(client.Object)
	if add {
		log.Info("adding finalizer", "finalizer", finalizer)
		controllerutil.AddFinalizer(desired, finalizer)
	} else {
		log.Info("removing finalizer", "finalizer", finalizer)
		controllerutil.RemoveFinalizer(desired, finalizer)
	}

	patch := client.MergeFromWithOptions(current, client.MergeFromWithOptimisticLock{})
	if err := config.Patch(ctx, desired, patch); err != nil {
		if !errors.Is(err, ErrQuiet) {
			log.Error(err, "unable to patch finalizers", "finalizer", finalizer)
			config.Recorder.Eventf(current, corev1.EventTypeWarning, "FinalizerPatchFailed",
				"Failed to patch finalizer %q: %s", finalizer, err)
		}
		return err
	}
	config.Recorder.Eventf(current, corev1.EventTypeNormal, "FinalizerPatched",
		"Patched finalizer %q", finalizer)

	// update current object with values from the api server after patching
	current.SetFinalizers(desired.GetFinalizers())
	current.SetResourceVersion(desired.GetResourceVersion())
	current.SetGeneration(desired.GetGeneration())

	return nil
}
