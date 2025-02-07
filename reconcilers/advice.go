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

package reconcilers

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"reconciler.io/runtime/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ SubReconciler[client.Object] = (*Advice[client.Object])(nil)

// Advice is a sub reconciler for advising the lifecycle of another sub reconciler in an aspect
// oriented programming style.
type Advice[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `Advice`.  Ideally unique, but
	// not required to be so.
	//
	// +optional
	Name string

	// Setup performs initialization on the manager and builder this reconciler
	// will run with.  It's common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error

	// Reconciler being advised
	Reconciler SubReconciler[Type]

	// Before is called preceding Around.  A modified context may be returned.  Errors are returned
	// immediately.
	//
	// If Before is not defined, there is no effect.
	//
	// +optional
	Before func(ctx context.Context, resource Type) (context.Context, Result, error)

	// Around is responsible for invoking the reconciler and returning the result. Implementations
	// may choose to not invoke the reconciler, invoke a different reconciler or invoke the
	// reconciler multiple times.
	//
	// If Around is not defined, the Reconciler is invoked once.
	//
	// +optional
	Around func(ctx context.Context, resource Type, reconciler SubReconciler[Type]) (Result, error)

	// After is called following Around. The result and error from Around are provided and may be
	// modified before returning.
	//
	// If After is not defined, the result and error are returned directly.
	//
	// +optional
	After func(ctx context.Context, resource Type, result Result, err error) (Result, error)

	lazyInit sync.Once
}

func (r *Advice[T]) init() {
	r.lazyInit.Do(func() {
		if r.Name == "" {
			r.Name = "Advice"
		}
		if r.Before == nil {
			r.Before = func(ctx context.Context, resource T) (context.Context, Result, error) {
				return nil, Result{}, nil
			}
		}
		if r.Around == nil {
			r.Around = func(ctx context.Context, resource T, reconciler SubReconciler[T]) (Result, error) {
				return reconciler.Reconcile(ctx, resource)
			}
		}
		if r.After == nil {
			r.After = func(ctx context.Context, resource T, result Result, err error) (Result, error) {
				return result, err
			}
		}
	})
}

func (r *Advice[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if r.Setup == nil {
		return nil
	}
	if err := r.Validate(ctx); err != nil {
		return err
	}
	if err := r.Setup(ctx, mgr, bldr); err != nil {
		return err
	}
	return r.Reconciler.SetupWithManager(ctx, mgr, bldr)
}

func (r *Advice[T]) Validate(ctx context.Context) error {
	if r.Reconciler == nil {
		return fmt.Errorf("Advice %q must implement Reconciler", r.Name)
	}
	if r.Before == nil && r.Around == nil && r.After == nil {
		return fmt.Errorf("Advice %q must implement at least one of Before, Around or After", r.Name)
	}

	if validation.IsRecursive(ctx) {
		if v, ok := r.Reconciler.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("Advice %q must have a valid Reconciler: %w", r.Name, err)
			}
		}
	}

	return nil
}

func (r *Advice[T]) Reconcile(ctx context.Context, resource T) (Result, error) {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	// before phase
	beforeCtx, result, err := r.Before(ctx, resource)
	if err != nil {
		return result, err
	}
	if beforeCtx != nil {
		ctx = beforeCtx
	}

	// around phase
	aroundResult, err := r.Around(ctx, resource, r.Reconciler)
	result = AggregateResults(result, aroundResult)

	// after phase
	return r.After(ctx, resource, result, err)
}
