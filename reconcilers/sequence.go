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
	"fmt"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ SubReconciler[client.Object] = (Sequence[client.Object])(nil)

// Sequence is a collection of SubReconcilers called in order. If a
// reconciler errs, further reconcilers are skipped.
type Sequence[Type client.Object] []SubReconciler[Type]

func (r Sequence[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
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

func (r Sequence[T]) Reconcile(ctx context.Context, resource T) (Result, error) {
	aggregateResult := Result{}
	for i, reconciler := range r {
		log := logr.FromContextOrDiscard(ctx).
			WithName(fmt.Sprintf("%d", i))
		ctx := logr.NewContext(ctx, log)

		result, err := reconciler.Reconcile(ctx, resource)
		aggregateResult = AggregateResults(result, aggregateResult)
		if err != nil {
			return result, err
		}
	}

	return aggregateResult, nil
}

func (r *Sequence[T]) Validate(ctx context.Context) error {
	// validate Sequence
	if hasNestedValidation(ctx) {
		for i, reconciler := range *r {
			if v, ok := reconciler.(Validator); ok {
				if err := v.Validate(ctx); err != nil {
					return fmt.Errorf("Sequence must have a valid Sequence[%d]: %s", i, err)
				}
			}
		}
	}

	return nil
}
