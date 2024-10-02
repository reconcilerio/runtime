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
	"reflect"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"reconciler.io/runtime/duck"
	"reconciler.io/runtime/internal"
	rtime "reconciler.io/runtime/time"
	"reconciler.io/runtime/tracker"
)

var (
	_ reconcile.Reconciler = (*AggregateReconciler[client.Object])(nil)
)

// AggregateReconciler is a controller-runtime reconciler that reconciles a specific resource. The
// Type resource is fetched for the reconciler
// request and passed in turn to each SubReconciler. Finally, the reconciled
// resource's status is compared with the original status, updating the API
// server if needed.
type AggregateReconciler[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `{Type}ResourceReconciler`.  Ideally
	// unique, but not required to be so.
	//
	// +optional
	Name string

	// Setup performs initialization on the manager and builder this reconciler
	// will run with. It's common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr Manager, bldr *Builder) error

	// Type of resource to reconcile. Required when the generic type is not a
	// struct, or is unstructured.
	//
	// +optional
	Type Type

	// Request of resource to reconcile. Only the specific resource matching the namespace and name
	// is reconciled. The namespace may be empty for cluster scoped resources.
	Request Request

	// Reconciler is called for each reconciler request with the resource being reconciled.
	// Typically, Reconciler is a Sequence of multiple SubReconcilers.
	//
	// When ErrHaltSubReconcilers is returned as an error, execution continues as if no error was
	// returned.
	//
	// +optional
	Reconciler SubReconciler[Type]

	// DesiredResource returns the desired resource to create/update, or nil if
	// the resource should not exist.
	//
	// +optional
	DesiredResource func(ctx context.Context, resource Type) (Type, error)

	// AggregateObjectManager synchronizes the aggregated resource with the API Server.
	AggregateObjectManager ObjectManager[Type]

	// BeforeReconcile is called first thing for each reconcile request.  A modified context may be
	// returned.  Errors are returned immediately.
	//
	// If BeforeReconcile is not defined, there is no effect.
	//
	// +optional
	BeforeReconcile func(ctx context.Context, req Request) (context.Context, Result, error)

	// AfterReconcile is called following all work for the reconcile request. The result and error
	// are provided and may be modified before returning.
	//
	// If AfterReconcile is not defined, the result and error are returned directly.
	//
	// +optional
	AfterReconcile func(ctx context.Context, req Request, res Result, err error) (Result, error)

	Config Config

	lazyInit sync.Once
}

func (r *AggregateReconciler[T]) init() {
	r.lazyInit.Do(func() {
		if internal.IsNil(r.Type) {
			var nilT T
			r.Type = newEmpty(nilT).(T)
		}
		if r.Name == "" {
			r.Name = fmt.Sprintf("%sAggregateReconciler", typeName(r.Type))
		}
		if r.Reconciler == nil {
			r.Reconciler = Sequence[T]{}
		}
		if r.DesiredResource == nil {
			r.DesiredResource = func(ctx context.Context, resource T) (T, error) {
				return resource, nil
			}
		}
		if r.BeforeReconcile == nil {
			r.BeforeReconcile = func(ctx context.Context, req reconcile.Request) (context.Context, reconcile.Result, error) {
				return ctx, Result{}, nil
			}
		}
		if r.AfterReconcile == nil {
			r.AfterReconcile = func(ctx context.Context, req reconcile.Request, res reconcile.Result, err error) (reconcile.Result, error) {
				return res, err
			}
		}
	})
}

func (r *AggregateReconciler[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	_, err := r.SetupWithManagerYieldingController(ctx, mgr)
	return err
}

func (r *AggregateReconciler[T]) SetupWithManagerYieldingController(ctx context.Context, mgr ctrl.Manager) (controller.Controller, error) {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name).
		WithValues(
			"resourceType", gvk(r.Config, r.Type),
			"request", r.Request,
		)
	ctx = logr.NewContext(ctx, log)

	ctx = StashConfig(ctx, r.Config)
	ctx = StashOriginalConfig(ctx, r.Config)
	ctx = StashResourceType(ctx, r.Type)
	ctx = StashOriginalResourceType(ctx, r.Type)

	if err := r.validate(ctx); err != nil {
		return nil, err
	}

	bldr := ctrl.NewControllerManagedBy(mgr)
	if !duck.IsDuck(r.Type, r.Config.Scheme()) {
		bldr.For(r.Type)
	} else {
		gvk, err := r.Config.GroupVersionKindFor(r.Type)
		if err != nil {
			return nil, err
		}
		apiVersion, kind := gvk.ToAPIVersionAndKind()
		u := &unstructured.Unstructured{}
		u.SetAPIVersion(apiVersion)
		u.SetKind(kind)
		bldr.For(u)
	}
	if r.Setup != nil {
		if err := r.Setup(ctx, mgr, bldr); err != nil {
			return nil, err
		}
	}
	if err := r.Reconciler.SetupWithManager(ctx, mgr, bldr); err != nil {
		return nil, err
	}
	if err := r.AggregateObjectManager.SetupWithManager(ctx, mgr, bldr); err != nil {
		return nil, err
	}
	return bldr.Build(r)
}

func (r *AggregateReconciler[T]) validate(ctx context.Context) error {
	// validate Request value
	if r.Request.Name == "" {
		return fmt.Errorf("AggregateReconciler %q must define Request", r.Name)
	}

	// validate Reconciler value
	if r.Reconciler == nil && r.DesiredResource == nil {
		return fmt.Errorf("AggregateReconciler %q must define Reconciler and/or DesiredResource", r.Name)
	}

	// validate AggregateObjectManager value
	if r.AggregateObjectManager == nil {
		return fmt.Errorf("AggregateReconciler %q must define AggregateObjectManager", r.Name)
	}

	return nil
}

func (r *AggregateReconciler[T]) Reconcile(ctx context.Context, req Request) (Result, error) {
	r.init()

	if req.Namespace != r.Request.Namespace || req.Name != r.Request.Name {
		// ignore other requests
		return Result{}, nil
	}

	ctx = WithStash(ctx)

	c := r.Config

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name).
		WithValues("resourceType", gvk(c, r.Type))
	ctx = logr.NewContext(ctx, log)

	ctx = rtime.StashNow(ctx, time.Now())
	ctx = StashRequest(ctx, req)
	ctx = StashConfig(ctx, c)
	ctx = StashOriginalConfig(ctx, r.Config)
	ctx = StashOriginalResourceType(ctx, r.Type)
	ctx = StashResourceType(ctx, r.Type)

	beforeCtx, beforeResult, err := r.BeforeReconcile(ctx, req)
	if err != nil {
		return beforeResult, err
	}
	if beforeCtx != nil {
		ctx = beforeCtx
	}

	reconcileResult, err := r.reconcile(ctx, req)

	return r.AfterReconcile(ctx, req, AggregateResults(beforeResult, reconcileResult), err)
}

func (r *AggregateReconciler[T]) reconcile(ctx context.Context, req Request) (Result, error) {
	log := logr.FromContextOrDiscard(ctx)
	c := RetrieveConfigOrDie(ctx)

	resource := r.Type.DeepCopyObject().(T)
	if err := c.Get(ctx, req.NamespacedName, resource); err != nil {
		if apierrs.IsNotFound(err) {
			// not found is ok
			resource.SetNamespace(r.Request.Namespace)
			resource.SetName(r.Request.Name)
		} else {
			if !errors.Is(err, ErrQuiet) {
				log.Error(err, "unable to fetch resource")
			}
			return Result{}, err
		}
	}

	if resource.GetDeletionTimestamp() != nil {
		// resource is being deleted, nothing to do
		return Result{}, nil
	}

	result, err := r.Reconciler.Reconcile(ctx, resource)
	if err != nil && !errors.Is(err, ErrHaltSubReconcilers) {
		return result, err
	}

	// hack, ignore track requests from the child reconciler, we have it covered
	ctx = StashConfig(ctx, Config{
		Client:    c.Client,
		APIReader: c.APIReader,
		Recorder:  c.Recorder,
		Tracker:   tracker.New(c.Scheme(), 0),
	})
	desired, err := r.desiredResource(ctx, resource)
	if err != nil {
		return result, err
	}
	_, err = r.AggregateObjectManager.Manage(ctx, resource, resource, desired)
	return result, err
}

func (r *AggregateReconciler[T]) desiredResource(ctx context.Context, resource T) (T, error) {
	var nilT T

	if resource.GetDeletionTimestamp() != nil {
		// the reconciled resource is pending deletion, cleanup the child resource
		return nilT, nil
	}

	fn := reflect.ValueOf(r.DesiredResource)
	out := fn.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(resource.DeepCopyObject()),
	})
	var obj T
	if !out[0].IsNil() {
		obj = out[0].Interface().(T)
	}
	var err error
	if !out[1].IsNil() {
		err = out[1].Interface().(error)
	}
	return obj, err
}
