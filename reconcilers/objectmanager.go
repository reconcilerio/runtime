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
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	jsonmergepatch "github.com/evanphx/json-patch/v5"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	jsonpatch "gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"reconciler.io/runtime/duck"
	"reconciler.io/runtime/internal"
	"reconciler.io/runtime/validation"
)

type ObjectManager[Type client.Object] interface {
	SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error
	Manage(ctx context.Context, resource client.Object, actual, desired Type) (Type, error)
}

var _ ObjectManager[client.Object] = (*UpdatingObjectManager[client.Object])(nil)
var _ validation.Validator = (*UpdatingObjectManager[client.Object])(nil)

// UpdatingObjectManager compares the actual and desired resources to create/update/delete as desired.
type UpdatingObjectManager[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `{Type}UpdatingObjectManager`.  Ideally
	// unique, but not required to be so.
	//
	// +optional
	Name string

	// Type is the resource being created/updated/deleted by the reconciler. Required when the
	// generic type is not a struct, or is unstructured.
	//
	// +optional
	Type Type

	// Finalizer is set on the reconciled resource before a managed resource is created, and cleared
	// after a managed resource is deleted. The value must be unique to this specific manager
	// instance and not shared. Reusing a value may result in orphaned resources when the
	// reconciled resource is deleted.
	//
	// Using a finalizer is encouraged when the Kubernetes garbage collector is unable to delete
	// the child resource automatically, like when the reconciled resource and child are in different
	// namespaces, scopes or clusters.
	//
	// +optional
	Finalizer string

	// TrackDesired when true, the desired resource is tracked after creates, before
	// updates, and on delete errors.
	TrackDesired bool

	// HarmonizeImmutableFields allows fields that are immutable on the current
	// object to be copied to the desired object in order to avoid creating
	// updates which are guaranteed to fail.
	//
	// +optional
	HarmonizeImmutableFields func(current, desired Type)

	// MergeBeforeUpdate copies desired fields on to the current object before
	// calling update. Typically fields to copy are the Spec, Labels and
	// Annotations.
	MergeBeforeUpdate func(current, desired Type)

	// Sanitize is called with an object before logging the value. Any value may
	// be returned. A meaningful subset of the resource is typically returned,
	// like the Spec.
	//
	// +optional
	Sanitize func(child Type) interface{}

	// DangerouslyAllowDuckTypes allows the Type to be a duck typed resource. This is dangerous
	// because duck types typically represent a subset of the target resource and may cause data
	// loss if the resource's server representation contains fields that do not exist on the duck
	// typed object.
	//
	// Use of this setting should be limited to when the author is certain the duck type is able to
	// represent the resource with full fidelity, or when data loss for unrepresented fields is
	// acceptable.
	DangerouslyAllowDuckTypes bool

	// mutationCache holds patches received from updates to a resource made by
	// mutation webhooks. This cache is used to avoid unnecessary update calls
	// that would actually have no effect.
	mutationCache *cache.Expiring
	lazyInit      sync.Once
}

func (r *UpdatingObjectManager[T]) init() {
	r.lazyInit.Do(func() {
		if internal.IsNil(r.Type) {
			var nilT T
			r.Type = newEmpty(nilT).(T)
		}
		if r.Name == "" {
			r.Name = fmt.Sprintf("%sUpdatingObjectManager", typeName(r.Type))
		}
		r.mutationCache = cache.NewExpiring()
	})
}

// Deprecated call SetupWithManager instead
func (r *UpdatingObjectManager[T]) Setup(ctx context.Context) error {
	r.init()
	return r.Validate(ctx)
}

func (r *UpdatingObjectManager[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	if err := r.Setup(ctx); err != nil {
		return err
	}

	if r.TrackDesired {
		var ct client.Object = r.Type
		if duck.IsDuck(ct, mgr.GetScheme()) {
			gvk := ct.GetObjectKind().GroupVersionKind()
			ct = &unstructured.Unstructured{}
			ct.GetObjectKind().SetGroupVersionKind(gvk)
		}

		bldr.Watches(ct, EnqueueTracked(ctx))
	}

	return nil
}

func (r *UpdatingObjectManager[T]) Validate(ctx context.Context) error {
	c := RetrieveConfigOrDie(ctx)

	// require MergeBeforeUpdate
	if r.MergeBeforeUpdate == nil {
		return fmt.Errorf("UpdatingObjectManager %q must define MergeBeforeUpdate", r.Name)
	}

	// require DangerouslyAllowDuckTypes for duck types
	if !r.DangerouslyAllowDuckTypes && duck.IsDuck(r.Type, c.Scheme()) {
		return fmt.Errorf("UpdatingObjectManager %q must enable DangerouslyAllowDuckTypes to use a duck type", r.Name)
	}

	return nil
}

// Manage a specific resource to create/update/delete based on the actual and desired state. The
// resource is the reconciled resource and used to record events for mutations. The actual and
// desired objects represent the managed resource and must be compatible with the type field.
func (r *UpdatingObjectManager[T]) Manage(ctx context.Context, resource client.Object, actual, desired T) (T, error) {
	r.init()

	var nilT T

	log := logr.FromContextOrDiscard(ctx)
	pc := RetrieveOriginalConfigOrDie(ctx)
	c := RetrieveConfigOrDie(ctx)
	if r.DangerouslyAllowDuckTypes {
		c = c.WithDangerousDuckClientOperations()
		ctx = StashConfig(ctx, c)
	}

	if (internal.IsNil(actual) || actual.GetCreationTimestamp().Time.IsZero()) && internal.IsNil(desired) {
		if err := ClearFinalizer(ctx, resource, r.Finalizer); err != nil {
			return nilT, err
		}
		return nilT, nil
	}

	// delete resource if no longer needed
	if internal.IsNil(desired) {
		if !actual.GetCreationTimestamp().Time.IsZero() && actual.GetDeletionTimestamp() == nil {
			log.Info("deleting unwanted resource", "resource", namespaceName(actual))
			if err := c.Delete(ctx, actual); err != nil {
				if !errors.Is(err, ErrQuiet) {
					log.Error(err, "unable to delete unwanted resource", "resource", namespaceName(actual))
					pc.Recorder.Eventf(resource, corev1.EventTypeWarning, "DeleteFailed",
						"Failed to delete %s %q: %v", typeName(actual), actual.GetName(), err)
				}
				return nilT, err
			}
			pc.Recorder.Eventf(resource, corev1.EventTypeNormal, "Deleted",
				"Deleted %s %q", typeName(actual), actual.GetName())

		}
		return nilT, nil
	}

	if err := AddFinalizer(ctx, resource, r.Finalizer); err != nil {
		return nilT, err
	}

	// create resource if it doesn't exist
	if internal.IsNil(actual) || actual.GetCreationTimestamp().Time.IsZero() {
		log.Info("creating resource", "resource", r.sanitize(desired))
		if err := c.Create(ctx, desired); err != nil {
			if !errors.Is(err, ErrQuiet) {
				log.Error(err, "unable to create resource", "resource", namespaceName(desired))
				pc.Recorder.Eventf(resource, corev1.EventTypeWarning, "CreationFailed",
					"Failed to create %s %q: %v", typeName(desired), desired.GetName(), err)
			}
			return nilT, err
		}
		if r.TrackDesired {
			// normally tracks should occur before API operations, but when creating a resource with a
			// generated name, we need to know the actual resource name.

			if err := c.Tracker.TrackObject(desired, resource); err != nil {
				return nilT, err
			}
		}
		pc.Recorder.Eventf(resource, corev1.EventTypeNormal, "Created",
			"Created %s %q", typeName(desired), desired.GetName())
		return desired, nil
	}

	// overwrite fields that should not be mutated
	if r.HarmonizeImmutableFields != nil {
		r.HarmonizeImmutableFields(actual, desired)
	}

	// lookup and apply remote mutations
	desiredPatched := desired.DeepCopyObject().(T)
	if patch, ok := r.mutationCache.Get(actual.GetUID()); ok {
		// the only object added to the cache is *Patch
		err := patch.(*Patch).Apply(desiredPatched)
		if err != nil {
			// there's not much we can do, let the normal update proceed
			log.Info("unable to patch desired child from mutation cache, this error is usually benign", "error", err.Error())
		}
	}

	// update resource with desired changes
	current := actual.DeepCopyObject().(T)
	if r.TrackDesired {
		if err := c.Tracker.TrackObject(current, resource); err != nil {
			return nilT, err
		}
	}
	r.MergeBeforeUpdate(current, desiredPatched)
	if equality.Semantic.DeepEqual(current, actual) {
		// resource is unchanged
		log.Info("resource is in sync, no update required")
		return actual, nil
	}
	log.Info("updating resource", "diff", cmp.Diff(r.sanitize(actual), r.sanitize(current), IgnoreAllUnexported))
	if err := c.Update(ctx, current); err != nil {
		if !errors.Is(err, ErrQuiet) {
			log.Error(err, "unable to update resource", "resource", namespaceName(current))
			pc.Recorder.Eventf(resource, corev1.EventTypeWarning, "UpdateFailed",
				"Failed to update %s %q: %v", typeName(current), current.GetName(), err)
		}
		return nilT, err
	}

	// capture admission mutation patch
	base := current.DeepCopyObject().(T)
	r.MergeBeforeUpdate(base, desired)
	patch, err := NewPatch(base, current)
	if err != nil {
		if !errors.Is(err, ErrQuiet) {
			log.Info("unable to generate mutation patch", "snapshot", r.sanitize(desired), "base", r.sanitize(base), "error", err.Error())
		}
	} else {
		r.mutationCache.Set(current.GetUID(), patch, 1*time.Hour)
	}

	log.Info("updated resource")
	pc.Recorder.Eventf(resource, corev1.EventTypeNormal, "Updated",
		"Updated %s %q", typeName(current), current.GetName())

	return current, nil
}

func (r *UpdatingObjectManager[T]) sanitize(resource T) interface{} {
	if r.Sanitize == nil {
		return resource
	}
	if internal.IsNil(resource) {
		return nil
	}

	// avoid accidental mutations in Sanitize method
	resource = resource.DeepCopyObject().(T)
	return r.Sanitize(resource)
}

func NewPatch(base, update client.Object) (*Patch, error) {
	baseBytes, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	updateBytes, err := json.Marshal(update)
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.CreatePatch(baseBytes, updateBytes)
	if err != nil {
		return nil, err
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}

	return &Patch{
		generation: base.GetGeneration(),
		bytes:      patchBytes,
	}, nil
}

type Patch struct {
	generation int64
	bytes      []byte
}

var PatchGenerationMismatch = errors.New("patch generation did not match target")

func (p *Patch) Apply(rebase client.Object) error {
	if rebase.GetGeneration() != p.generation {
		return PatchGenerationMismatch
	}

	rebaseBytes, err := json.Marshal(rebase)
	if err != nil {
		return err
	}
	merge, err := jsonmergepatch.DecodePatch(p.bytes)
	if err != nil {
		return err
	}
	patchedBytes, err := merge.Apply(rebaseBytes)
	if err != nil {
		return err
	}
	// reset rebase to its empty value before unmarshaling into it
	replaceWithEmpty(rebase)
	return json.Unmarshal(patchedBytes, rebase)
}
