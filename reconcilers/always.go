/*
Copyright 2025 the original author or authors.

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

	"github.com/go-logr/logr"
	"reconciler.io/runtime/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ SubReconciler[client.Object] = (Always[client.Object])(nil)

// Always is a collection of SubReconcilers called in order. Each reconciler is called regardless
// of previous errors. The resulting errors are joined. The resulting joined error will only match
// ErrQuiet if all contributing errors match ErrQuiet.
type Always[Type client.Object] []SubReconciler[Type]

func (r Always[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	for i, reconciler := range r {
		log := logr.FromContextOrDiscard(ctx).
			WithName(fmt.Sprintf("%d", i))
		ctx = logr.NewContext(ctx, log)

		err := reconciler.SetupWithManager(ctx, mgr, bldr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r Always[T]) Reconcile(ctx context.Context, resource T) (Result, error) {
	aggregateResult := Result{}
	aggregateErrors := []error{}
	aggregateNonQuietErrors := []error{}
	for i, reconciler := range r {
		log := logr.FromContextOrDiscard(ctx).
			WithName(fmt.Sprintf("%d", i))
		ctx := logr.NewContext(ctx, log)

		result, err := reconciler.Reconcile(ctx, resource)
		aggregateResult = AggregateResults(result, aggregateResult)
		if err != nil {
			aggregateErrors = append(aggregateErrors, err)
			if !errors.Is(err, ErrQuiet) {
				aggregateNonQuietErrors = append(aggregateNonQuietErrors, err)
			}
		}
	}
	// only return an ErrQuiet if all errors are quiet
	if err := errors.Join(aggregateNonQuietErrors...); err != nil {
		return aggregateResult, err
	}
	return aggregateResult, errors.Join(aggregateErrors...)
}

func (r *Always[T]) Validate(ctx context.Context) error {
	// validate Always
	if validation.IsRecursive(ctx) {
		for i, reconciler := range *r {
			if v, ok := reconciler.(validation.Validator); ok {
				if err := v.Validate(ctx); err != nil {
					return fmt.Errorf("Always must have a valid Always[%d]: %w", i, err)
				}
			}
		}
	}

	return nil
}
