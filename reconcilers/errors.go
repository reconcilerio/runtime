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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"reconciler.io/runtime/internal"
	rtime "reconciler.io/runtime/time"
	"reconciler.io/runtime/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrDurable is an error which the reconcile request should not be retried until the observed
	// state has changed. Meaningful state about the error has been captured on the status
	ErrDurable = errors.Join(ErrQuiet, ErrHaltSubReconcilers)
)

type SuppressTransientErrors[Type client.Object, ListType client.ObjectList] struct {
	// Name used to identify this reconciler.  Defaults to `ForEach`. Ideally unique, but not
	// required to be so.
	//
	// +optional
	Name string

	// ListType is the listing type for the type. For example, PodList is the list type for Pod.
	// Required when the generic type is not a struct, or is unstructured.
	//
	// +optional
	ListType ListType

	// Setup performs initialization on the manager and builder this reconciler will run with. It's
	// common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error

	// Threshold is the number of non-ErrDurable error reconciles encountered for a specific
	// resource generation before which status update is suppressed.
	Threshold uint8

	// Reconciler to be called for each iterable item
	Reconciler SubReconciler[Type]

	lazyInit     sync.Once
	m            sync.Mutex
	lastPurge    time.Time
	errorCounter map[types.UID]transientErrorCounter
}

func (r *SuppressTransientErrors[T, LT]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
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

func (r *SuppressTransientErrors[T, LT]) init() {
	r.lazyInit.Do(func() {
		if r.Name == "" {
			r.Name = "SuppressTransientErrors"
		}
		if internal.IsNil(r.ListType) {
			var nilLT LT
			r.ListType = newEmpty(nilLT).(LT)
		}
		if r.Threshold == 0 {
			r.Threshold = 3
		}
		r.errorCounter = map[types.UID]transientErrorCounter{}
	})
}

func (r *SuppressTransientErrors[T, LT]) checkStaleCounters(ctx context.Context) {
	now := rtime.RetrieveNow(ctx)
	log := logr.FromContextOrDiscard(ctx)

	r.m.Lock()
	defer r.m.Unlock()

	if r.lastPurge.IsZero() {
		r.lastPurge = now
		return
	}
	if r.lastPurge.Add(24 * time.Hour).After(now) {
		return
	}

	log.Info("purging stale resource counters")

	c := RetrieveConfigOrDie(ctx)
	list := r.ListType.DeepCopyObject().(LT)
	if err := c.List(ctx, list); err != nil {
		log.Error(err, "purge failed to list resources")
		return
	}

	validIds := sets.New[types.UID]()
	for _, item := range extractItems[T](list) {
		validIds.Insert(item.GetUID())
	}

	counterIds := sets.New[types.UID]()
	for uid := range r.errorCounter {
		counterIds.Insert(uid)
	}

	for _, uid := range counterIds.Difference(validIds).UnsortedList() {
		log.V(2).Info("purging counter", "id", uid)
		delete(r.errorCounter, uid)
	}

	r.lastPurge = now
}

func (r *SuppressTransientErrors[T, LT]) Validate(ctx context.Context) error {
	r.init()

	// validate Reconciler
	if r.Reconciler == nil {
		return fmt.Errorf("SuppressTransientErrors %q must implement Reconciler", r.Name)
	}
	if validation.IsRecursive(ctx) {
		if v, ok := r.Reconciler.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("SuppressTransientErrors %q must have a valid Reconciler: %w", r.Name, err)
			}
		}
	}

	return nil
}

func (r *SuppressTransientErrors[T, LT]) Reconcile(ctx context.Context, resource T) (Result, error) {
	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	defer r.checkStaleCounters(ctx)

	result, err := r.Reconciler.Reconcile(ctx, resource)

	if err == nil || errors.Is(err, ErrDurable) {
		delete(r.errorCounter, resource.GetUID())
		return result, err
	}

	// concurrent map access is ok, since keys are resources specific and a given resource will never be processed concurrently
	counter, ok := r.errorCounter[resource.GetUID()]
	if !ok || counter.Generation != resource.GetGeneration() {
		counter = transientErrorCounter{
			Generation: resource.GetGeneration(),
			Count:      0,
		}
	}

	// check overflow before incrementing
	if counter.Count != uint8(255) {
		counter.Count = counter.Count + 1
		r.errorCounter[resource.GetUID()] = counter
	}

	if counter.Count < r.Threshold {
		// suppress status update
		return result, errors.Join(err, ErrSkipStatusUpdate, ErrQuiet)
	}

	return result, err
}

type transientErrorCounter struct {
	Generation int64
	Count      uint8
}
