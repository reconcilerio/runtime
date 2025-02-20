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

package reconcilers

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	"reconciler.io/runtime/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ SubReconciler[client.Object] = (*IfThen[client.Object])(nil)
	_ SubReconciler[client.Object] = (*While[client.Object])(nil)
	_ SubReconciler[client.Object] = (*ForEach[client.Object, any])(nil)
	_ SubReconciler[client.Object] = (*TryCatch[client.Object])(nil)
	_ SubReconciler[client.Object] = (*OverrideSetup[client.Object])(nil)
)

// IfThen conditionally branches the reconcilers called for a request based on
// a condition. When the If condition is true, Then is invoked; when false,
// Else is invoked.
type IfThen[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `IfThen`.  Ideally
	// unique, but not required to be so.
	//
	// +optional
	Name string

	// Setup performs initialization on the manager and builder this reconciler
	// will run with. It's common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr Manager, bldr *Builder) error

	// If controls the flow of execution calling the Then reconciler when true,
	// and the Else reconciler when false.
	If func(ctx context.Context, resource Type) bool

	// Then is called when If() returns true. Typically, Then is a Sequence of
	// multiple SubReconcilers.
	Then SubReconciler[Type]

	// Else is called when If() returns false. Typically, Else is a Sequence of
	// multiple SubReconcilers.
	//
	// +optional
	Else SubReconciler[Type]

	lazyInit sync.Once
}

func (r *IfThen[T]) init() {
	r.lazyInit.Do(func() {
		if r.Name == "" {
			r.Name = "IfThen"
		}
	})
}

func (r *IfThen[T]) Validate(ctx context.Context) error {
	r.init()

	// validate If
	if r.If == nil {
		return fmt.Errorf("IfThen %q must implement If", r.Name)
	}

	// validate Then
	if r.Then == nil {
		return fmt.Errorf("IfThen %q must implement Then", r.Name)
	}
	if validation.IsRecursive(ctx) {
		if v, ok := r.Then.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("IfThen %q must have a valid Then: %w", r.Name, err)
			}
		}
	}

	// validate Else
	if r.Else != nil && validation.IsRecursive(ctx) {
		if v, ok := r.Else.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("IfThen %q must have a valid Else: %w", r.Name, err)
			}
		}
	}

	return nil
}

func (r *IfThen[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if err := r.Validate(ctx); err != nil {
		return err
	}

	if r.Setup != nil {
		if err := r.Setup(ctx, mgr, bldr); err != nil {
			return err
		}
	}
	if err := r.Then.SetupWithManager(ctx, mgr, bldr); err != nil {
		return err
	}
	if r.Else != nil {
		if err := r.Else.SetupWithManager(ctx, mgr, bldr); err != nil {
			return err
		}
	}

	return nil
}

func (r *IfThen[T]) Reconcile(ctx context.Context, resource T) (Result, error) {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if r.If(ctx, resource) {
		return r.Then.Reconcile(ctx, resource)
	} else if r.Else != nil {
		return r.Else.Reconcile(ctx, resource)
	}

	return Result{}, nil
}

// ErrMaxIterations indicates the maximum number of loop iterations was
// exceeded.
type ErrMaxIterations struct {
	Iterations int
}

func (err *ErrMaxIterations) Error() string {
	return fmt.Sprintf("exceeded max iterations: %d", err.Iterations)
}

const iterationStashKey StashKey = "reconciler.io/runtime:iteration"

func stashIteration(ctx context.Context, i int) context.Context {
	return context.WithValue(ctx, iterationStashKey, i)
}

// RetrieveIteration returns the iteration index. If nested loops are executing
// only the inner most iteration index is returned.
//
// -1 indicates the call is not within a loop.
func RetrieveIteration(ctx context.Context) int {
	v := ctx.Value(iterationStashKey)
	if i, ok := v.(int); ok {
		return i
	}
	return -1
}

// While the Condition is true call the Reconciler.
//
// While must not be used to block the reconciler awaiting a remote condition
// change. Instead watch the remote condition for changes and enqueue a new
// request to reconcile the resource.
//
// To avoid infinite loops, MaxIterations is defaulted to 100. Set
// MaxIterations to 0 to disable. ErrMaxIterations is returned when the limit
// is exceeded. The current iteration for the most local loop is available via
// RetrieveIteration.
type While[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `While`.  Ideally
	// unique, but not required to be so.
	//
	// +optional
	Name string

	// Setup performs initialization on the manager and builder this reconciler
	// will run with. It's common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr Manager, bldr *Builder) error

	// Condition controls the execution flow calling the reconciler so long as
	// the returned value remains true.
	Condition func(ctx context.Context, resource Type) bool

	// Reconciler is called so long as Condition() returns true. Typically,
	// Reconciler is a Sequence of multiple SubReconcilers.
	Reconciler SubReconciler[Type]

	// MaxIterations guards against infinite loops by limiting the number of
	// allowed iterations before returning an error. Defaults to 100, set to 0
	// to disable.
	MaxIterations *int

	lazyInit sync.Once
}

func (r *While[T]) init() {
	r.lazyInit.Do(func() {
		if r.Name == "" {
			r.Name = "While"
		}
		if r.MaxIterations == nil {
			r.MaxIterations = ptr.To(100)
		}
	})
}

func (r *While[T]) Validate(ctx context.Context) error {
	r.init()

	// validate Condition
	if r.Condition == nil {
		return fmt.Errorf("While %q must implement Condition", r.Name)
	}

	// validate Reconciler
	if r.Reconciler == nil {
		return fmt.Errorf("While %q must implement Reconciler", r.Name)
	}
	if validation.IsRecursive(ctx) {
		if v, ok := r.Reconciler.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("While %q must have a valid Reconciler: %w", r.Name, err)
			}
		}
	}

	return nil
}

func (r *While[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if err := r.Validate(ctx); err != nil {
		return err
	}

	if r.Setup != nil {
		if err := r.Setup(ctx, mgr, bldr); err != nil {
			return err
		}
	}
	if err := r.Reconciler.SetupWithManager(ctx, mgr, bldr); err != nil {
		return err
	}

	return nil
}

func (r *While[T]) Reconcile(ctx context.Context, resource T) (Result, error) {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	aggregateResult := Result{}
	for i := 0; true; i++ {
		if *r.MaxIterations != 0 && i >= *r.MaxIterations {
			return aggregateResult, &ErrMaxIterations{Iterations: i}
		}

		log := logr.FromContextOrDiscard(ctx).
			WithName(fmt.Sprintf("%d", i))
		ctx := logr.NewContext(ctx, log)
		ctx = stashIteration(ctx, i)

		if !r.Condition(ctx, resource) {
			break
		}

		result, err := r.Reconciler.Reconcile(ctx, resource)
		aggregateResult = AggregateResults(aggregateResult, result)
		if err != nil {
			return aggregateResult, err
		}
	}

	return aggregateResult, nil
}

// ForEach calls the reconcilers for each item. The current value of the iteration is exposed as a
// Cursor that is available via the CursorStasher helper. Multiple ForEach reconcilers are nestable
// so long as the types being iterated over are unique.
type ForEach[Type client.Object, Item any] struct {
	// Name used to identify this reconciler.  Defaults to `ForEach`. Ideally unique, but
	// not required to be so.
	//
	// +optional
	Name string

	// Setup performs initialization on the manager and builder this reconciler
	// will run with. It's common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error

	// Reconciler to be called for each iterable item
	Reconciler SubReconciler[Type]

	// Items returns the items to iterate over
	Items func(ctx context.Context, resource Type) ([]Item, error)

	lazyInit sync.Once
}

func (r *ForEach[T, I]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if err := r.Validate(ctx); err != nil {
		return err
	}
	if err := r.Reconciler.SetupWithManager(ctx, mgr, bldr); err != nil {
		return err
	}
	if r.Setup == nil {
		return nil
	}
	return r.Setup(ctx, mgr, bldr)
}

func (r *ForEach[T, I]) init() {
	r.lazyInit.Do(func() {
		if r.Name == "" {
			r.Name = "ForEach"
		}
	})
}

func (r *ForEach[T, I]) Validate(ctx context.Context) error {
	r.init()

	// validate Reconciler
	if r.Reconciler == nil {
		return fmt.Errorf("ForEach %q must implement Reconciler", r.Name)
	}
	if r.Items == nil {
		return fmt.Errorf("ForEach %q must implement Items", r.Name)
	}
	if validation.IsRecursive(ctx) {
		if v, ok := r.Reconciler.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("ForEach %q must have a valid Reconciler: %w", r.Name, err)
			}
		}
	}

	return nil
}

func (r *ForEach[T, I]) Reconcile(ctx context.Context, resource T) (Result, error) {
	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	result := Result{}

	items, err := r.Items(ctx, resource)
	if err != nil {
		return result, err
	}

	for i := range items {
		log := log.WithName(fmt.Sprintf("%d", i))
		ctx := logr.NewContext(ctx, log)

		CursorStasher[I]().Store(ctx, Cursor[I]{
			Index:  i,
			Length: len(items),
			Item:   items[i],
		})

		iterationResult, err := r.Reconciler.Reconcile(ctx, resource)
		if err != nil {
			return result, err
		}
		result = AggregateResults(result, iterationResult)
	}

	return result, nil
}

// Cursor represents the current value within an iterator
type Cursor[I any] struct {
	// Index of the current iteration
	Index int
	// Length of the current iteration, or -1 if not known and/or unbounded
	Length int
	// Item of the current iteration
	Item I
}

// CursorStasher creates a Stasher for a Cursor of the generic type
func CursorStasher[I any]() Stasher[Cursor[I]] {
	// avoid key collisions for nested iteration over different types
	var empty I
	key := StashKey(fmt.Sprintf("reconciler.io/runtime:cursor:%s", typeName(empty)))
	return NewStasher[Cursor[I]](key)
}

// TryCatch facilitates recovery from errors encountered within the Try
// reconciler. The results of the Try reconciler are passed to the Catch
// handler which can suppress, modify or continue the error. The Finally
// reconciler is always called before returning, but cannot prevent an
// error from being returned.
//
// The semantics mimic the try-catch-finally behavior from C-style languages:
//   - Try, Catch, Finally are called in order for each request.
//   - Catch can fully redefine the results from Try.
//   - if Catch is not defined, the Try results are implicitly propagated.
//   - if the results are in error before Finally is called, the final results
//     will be in error.
//   - an error returned from Finally will preempt an existing error.
//   - the existing Result is aggregated with the Finally Result.
//
// Use of Finally should be limited to common clean up logic that applies
// equally to normal and error conditions. Further flow control within Finally
// is discouraged as new errors can mask errors returned from Catch.
type TryCatch[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `TryCatch`.  Ideally
	// unique, but not required to be so.
	//
	// +optional
	Name string

	// Setup performs initialization on the manager and builder this reconciler
	// will run with. It's common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr Manager, bldr *Builder) error

	// Try is a reconciler that may return an error that needs to be handled.
	// Typically, Try is a Sequence of multiple SubReconcilers.
	Try SubReconciler[Type]

	// Catch is called with the results from Try(). New results can be returned
	// suppressing the original results.
	//
	// +optional
	Catch func(ctx context.Context, resource Type, result Result, err error) (Result, error)

	// Finally is always called before returning. An error from Finally will
	// always be returned. If Finally does not return an error, the error state
	// before Finally was called will be returned along with the result
	// aggregated.
	//
	// Typically, Finally is a Sequence of multiple SubReconcilers.
	//
	// +optional
	Finally SubReconciler[Type]

	lazyInit sync.Once
}

func (r *TryCatch[T]) init() {
	r.lazyInit.Do(func() {
		if r.Name == "" {
			r.Name = "TryCatch"
		}
	})
}

func (r *TryCatch[T]) Validate(ctx context.Context) error {
	r.init()

	// validate Try
	if r.Try == nil {
		return fmt.Errorf("TryCatch %q must implement Try", r.Name)
	}
	if validation.IsRecursive(ctx) {
		if v, ok := r.Try.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("TryCatch %q must have a valid Try: %w", r.Name, err)
			}
		}
	}

	// validate Finally
	if r.Finally != nil && validation.IsRecursive(ctx) {
		if v, ok := r.Finally.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("TryCatch %q must have a valid Finally: %w", r.Name, err)
			}
		}
	}

	return nil
}

func (r *TryCatch[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if err := r.Validate(ctx); err != nil {
		return err
	}

	if r.Setup != nil {
		if err := r.Setup(ctx, mgr, bldr); err != nil {
			return err
		}
	}
	if err := r.Try.SetupWithManager(ctx, mgr, bldr); err != nil {
		return err
	}
	if r.Finally != nil {
		if err := r.Finally.SetupWithManager(ctx, mgr, bldr); err != nil {
			return err
		}
	}

	return nil
}

func (r *TryCatch[T]) Reconcile(ctx context.Context, resource T) (Result, error) {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	result, err := r.Try.Reconcile(ctx, resource)
	if r.Catch != nil {
		result, err = r.Catch(ctx, resource, result, err)
	}
	if r.Finally != nil {
		fresult, ferr := r.Finally.Reconcile(ctx, resource)
		if ferr != nil {
			// an error from Finally overrides the existing err
			return AggregateResults(result, fresult), ferr
		}
		result = AggregateResults(result, fresult)
	}

	return result, err
}

// OverrideSetup suppresses the SetupWithManager on the nested Reconciler in
// favor of the local Setup method.
type OverrideSetup[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `SkipSetup`. Ideally
	// unique, but not required to be so.
	//
	// +optional
	Name string

	// Setup allows for custom initialization on the manager and builder this
	// reconciler will run with. Since the SetupWithManager method will not be
	// called on Reconciler, this method can be used to provide an alternative
	// setup. It's common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr Manager, bldr *Builder) error

	// Reconciler is called for each reconciler request with the reconciled
	// resource being reconciled. SetupWithManager will not be called on this
	// reconciler. Typically a Sequence is used to compose multiple
	// SubReconcilers.
	//
	// +optional
	Reconciler SubReconciler[Type]

	lazyInit sync.Once
}

func (r *OverrideSetup[T]) init() {
	r.lazyInit.Do(func() {
		if r.Name == "" {
			r.Name = "SkipSetup"
		}
	})
}

func (r *OverrideSetup[T]) Validate(ctx context.Context) error {
	r.init()

	// validate Setup || Reconciler
	if r.Setup == nil && r.Reconciler == nil {
		return fmt.Errorf("OverrideSetup %q must implement at least one of Setup or Reconciler", r.Name)
	}

	// validate Reconciler
	if validation.IsRecursive(ctx) {
		if v, ok := r.Reconciler.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("OverrideSetup %q must have a valid Reconciler: %w", r.Name, err)
			}
		}
	}

	return nil
}

func (r *OverrideSetup[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if err := r.Validate(ctx); err != nil {
		return err
	}

	if r.Setup != nil {
		if err := r.Setup(ctx, mgr, bldr); err != nil {
			return err
		}
	}

	return nil
}

func (r *OverrideSetup[T]) Reconcile(ctx context.Context, resource T) (Result, error) {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if r.Reconciler == nil {
		return Result{}, nil
	}

	return r.Reconciler.Reconcile(ctx, resource)
}
