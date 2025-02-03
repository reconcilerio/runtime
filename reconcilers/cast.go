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
	"fmt"
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ SubReconciler[client.Object] = (*CastResource[client.Object, client.Object])(nil)

// CastResource casts the ResourceReconciler's type by projecting the resource data
// onto a new struct. Casting the reconciled resource is useful to create cross
// cutting reconcilers that can operate on common portion of multiple  resources,
// commonly referred to as a duck type.
//
// If the CastType generic is an interface rather than a struct, the resource is
// passed directly rather than converted.
type CastResource[Type, CastType client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `{Type}CastResource`.  Ideally unique, but
	// not required to be so.
	//
	// +optional
	Name string

	// Reconciler is called for each reconciler request with the reconciled resource. Typically a
	// Sequence is used to compose multiple SubReconcilers.
	Reconciler SubReconciler[CastType]

	noop     bool
	lazyInit sync.Once
}

func (r *CastResource[T, CT]) init() {
	r.lazyInit.Do(func() {
		var nilCT CT
		if reflect.ValueOf(nilCT).Kind() == reflect.Invalid {
			// not a real cast, just converting generic types
			r.noop = true
			return
		}
		emptyCT := newEmpty(nilCT)
		if r.Name == "" {
			r.Name = fmt.Sprintf("%sCastResource", typeName(emptyCT))
		}
	})
}

func (r *CastResource[T, CT]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	if !r.noop {
		var nilCT CT
		emptyCT := newEmpty(nilCT).(CT)

		log := logr.FromContextOrDiscard(ctx).
			WithName(r.Name).
			WithValues("castResourceType", typeName(emptyCT))
		ctx = logr.NewContext(ctx, log)

		if err := r.Validate(ctx); err != nil {
			return err
		}
	}

	return r.Reconciler.SetupWithManager(ctx, mgr, bldr)
}

func (r *CastResource[T, CT]) Validate(ctx context.Context) error {
	// validate Reconciler value
	if r.Reconciler == nil {
		return fmt.Errorf("CastResource %q must define Reconciler", r.Name)
	}
	if hasNestedValidation(ctx) {
		if v, ok := r.Reconciler.(Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("CastResource %q must have a valid Reconciler: %s", r.Name, err)
			}
		}
	}

	return nil
}

func (r *CastResource[T, CT]) Reconcile(ctx context.Context, resource T) (Result, error) {
	r.init()

	if r.noop {
		// cast the type rather than convert the object
		return r.Reconciler.Reconcile(ctx, client.Object(resource).(CT))
	}

	var nilCT CT
	emptyCT := newEmpty(nilCT).(CT)

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name).
		WithValues("castResourceType", typeName(emptyCT))
	ctx = logr.NewContext(ctx, log)

	ctx, castResource, err := r.cast(ctx, resource)
	if err != nil {
		return Result{}, err
	}
	castOriginal := castResource.DeepCopyObject().(client.Object)
	result, err := r.Reconciler.Reconcile(ctx, castResource)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	if !equality.Semantic.DeepEqual(castResource, castOriginal) {
		// patch the reconciled resource with the updated duck values
		patch, err := NewPatch(castOriginal, castResource)
		if err != nil {
			errs = append(errs, err)
			return result, errors.NewAggregate(errs)
		}
		err = patch.Apply(resource)
		if err != nil {
			errs = append(errs, err)
			return result, errors.NewAggregate(errs)
		}

	}
	return result, errors.NewAggregate(errs)
}

func (r *CastResource[T, CT]) cast(ctx context.Context, resource T) (context.Context, CT, error) {
	var nilCT CT

	data, err := json.Marshal(resource)
	if err != nil {
		return nil, nilCT, err
	}
	castResource := newEmpty(nilCT).(CT)
	err = json.Unmarshal(data, castResource)
	if err != nil {
		return nil, nilCT, err
	}
	if kind := castResource.GetObjectKind(); kind.GroupVersionKind().Empty() {
		// default the apiVersion/kind with the real value from the resource if not already defined
		c := RetrieveConfigOrDie(ctx)
		kind.SetGroupVersionKind(gvk(c, resource))
	}
	ctx = StashResourceType(ctx, castResource)
	return ctx, castResource, nil
}
