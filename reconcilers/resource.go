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
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"reconciler.io/runtime/duck"
	"reconciler.io/runtime/internal"
	rtime "reconciler.io/runtime/time"
	"reconciler.io/runtime/validation"
)

var _ reconcile.Reconciler = (*ResourceReconciler[client.Object])(nil)

// ErrSkipStatusUpdate indicates the ResourceReconciler should not update the status for the
// resource in this request, even if it has changed.
//
// This error should often be combined with ErrQuiet to avoid being logged:
//
//	errors.Join(ErrSkipStatusUpdate, ErrQuiet)
var ErrSkipStatusUpdate = errors.New("skip ResourceReconciler status update for this request")

// ResourceReconciler is a controller-runtime reconciler that reconciles a given
// existing resource. The Type resource is fetched for the reconciler
// request and passed in turn to each SubReconciler. Finally, the reconciled
// resource's status is compared with the original status, updating the API
// server if needed.
type ResourceReconciler[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `{Type}ResourceReconciler`.  Ideally
	// unique, but not required to be so.
	//
	// +optional
	Name string

	// Setup performs initialization on the manager and builder this reconciler
	// will run with. It's common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error

	// Type of resource to reconcile. Required when the generic type is not a
	// struct, or is unstructured.
	//
	// +optional
	Type Type

	// SkipStatusUpdate when true, the resource's status will not be updated. If this
	// is not the primary reconciler for a resource, skipping status updates can avoid
	// conflicts. Finalizers and events are still actionable.
	SkipStatusUpdate bool

	// SyncStatusDuringFinalization when true, the resource's status will be updated even
	// when the resource is marked for deletion.
	SyncStatusDuringFinalization bool

	// Reconciler is called for each reconciler request with the resource being reconciled.
	// Typically, Reconciler is a Sequence of multiple SubReconcilers.
	//
	// When ErrHaltSubReconcilers is returned as an error, execution continues as if no error was
	// returned.
	Reconciler SubReconciler[Type]

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

	// SkipResource shortcuts the reconciler for the specific request. While the context and logger
	// are initialized, no work is preformed. The request is removed from the workqueue.
	//
	// +optional
	SkipRequest func(ctx context.Context, req Request) bool

	// SkipResource shortcuts the reconciler for the specific resource. The Reconciler is not
	// called and the resource's status is not updated.
	//
	// BeforeReconcile and AfterReconcile are called, and the resource's defaults are applied
	// before calling this method.
	//
	// +optional
	SkipResource func(ctx context.Context, resource Type) bool

	Config Config

	lazyInit sync.Once
}

func (r *ResourceReconciler[T]) init() {
	r.lazyInit.Do(func() {
		if internal.IsNil(r.Type) {
			var nilT T
			r.Type = newEmpty(nilT).(T)
		}
		if r.Name == "" {
			r.Name = fmt.Sprintf("%sResourceReconciler", typeName(r.Type))
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
		if r.SkipRequest == nil {
			r.SkipRequest = func(ctx context.Context, req reconcile.Request) bool {
				return false
			}
		}
		if r.SkipResource == nil {
			r.SkipResource = func(ctx context.Context, resource T) bool {
				return false
			}
		}
	})
}

func (r *ResourceReconciler[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	_, err := r.SetupWithManagerYieldingController(ctx, mgr)
	return err
}

func (r *ResourceReconciler[T]) SetupWithManagerYieldingController(ctx context.Context, mgr ctrl.Manager) (controller.Controller, error) {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name).
		WithValues("resourceType", gvk(r.Config, r.Type))
	ctx = logr.NewContext(ctx, log)

	ctx = StashConfig(ctx, r.Config)
	ctx = StashOriginalConfig(ctx, r.Config)
	ctx = StashResourceType(ctx, r.Type)
	ctx = StashOriginalResourceType(ctx, r.Type)

	if err := r.Validate(ctx); err != nil {
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
	return bldr.Build(r)
}

func (r *ResourceReconciler[T]) Validate(ctx context.Context) error {
	r.init()

	// validate Reconciler value
	if r.Reconciler == nil {
		return fmt.Errorf("ResourceReconciler %q must define Reconciler", r.Name)
	}
	if validation.IsRecursive(ctx) {
		if v, ok := r.Reconciler.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("ResourceReconciler %q must have a valid Reconciler: %w", r.Name, err)
			}
		}
	}

	// warn users of common pitfalls. These are not blockers.

	log := logr.FromContextOrDiscard(ctx)

	resourceType := reflect.TypeOf(r.Type).Elem()
	statusField, hasStatus := resourceType.FieldByName("Status")
	if !hasStatus {
		log.Info("resource missing status field, operations related to status will be skipped")
		return nil
	}

	statusType := statusField.Type
	if statusType.Kind() == reflect.Ptr {
		log.Info("resource status is nilable, status is typically a struct")
		statusType = statusType.Elem()
	}

	observedGenerationField, hasObservedGeneration := statusType.FieldByName("ObservedGeneration")
	if !hasObservedGeneration || observedGenerationField.Type.Kind() != reflect.Int64 {
		log.Info("resource status missing ObservedGeneration field of type int64, generation will not be managed")
	}

	initializeConditionsMethod, hasInitializeConditions := reflect.PointerTo(statusType).MethodByName("InitializeConditions")
	if !hasInitializeConditions || initializeConditionsMethod.Type.NumIn() > 2 || initializeConditionsMethod.Type.NumOut() != 0 {
		log.Info("resource status missing InitializeConditions(context.Context) method, conditions will not be auto-initialized")
	} else if hasInitializeConditions && initializeConditionsMethod.Type.NumIn() == 1 {
		log.Info("resource status InitializeConditions() method is deprecated, use InitializeConditions(context.Context)")
	}

	conditionsField, hasConditions := statusType.FieldByName("Conditions")
	if !hasConditions || !conditionsField.Type.AssignableTo(reflect.TypeOf([]metav1.Condition{})) {
		log.Info("resource status is missing field Conditions of type []metav1.Condition, condition timestamps will not be managed")
	}

	return nil
}

func (r *ResourceReconciler[T]) Reconcile(ctx context.Context, req Request) (Result, error) {
	r.init()

	ctx = WithStash(ctx)

	c := r.Config

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name).
		WithValues("resourceType", gvk(c, r.Type))
	ctx = logr.NewContext(ctx, log)

	ctx = rtime.StashNow(ctx, time.Now())
	ctx = StashRequest(ctx, req)
	ctx = StashConfig(ctx, c)
	ctx = StashOriginalConfig(ctx, c)
	ctx = StashOriginalResourceType(ctx, r.Type)
	ctx = StashResourceType(ctx, r.Type)

	if r.SkipRequest(ctx, req) {
		return Result{}, nil
	}

	beforeCtx, beforeResult, err := r.BeforeReconcile(ctx, req)
	if err != nil {
		return beforeResult, err
	}
	if beforeCtx != nil {
		ctx = beforeCtx
	}

	reconcileResult, err := r.reconcileOuter(ctx, req)

	result, err := r.AfterReconcile(ctx, req, AggregateResults(beforeResult, reconcileResult), err)
	if errors.Is(err, ErrQuiet) {
		// suppress error, while forcing a requeue
		return Result{Requeue: true}, nil
	}
	return result, err
}

func (r *ResourceReconciler[T]) reconcileOuter(ctx context.Context, req Request) (Result, error) {
	log := logr.FromContextOrDiscard(ctx)
	c := RetrieveOriginalConfigOrDie(ctx)

	originalResource := r.Type.DeepCopyObject().(T)

	if err := c.Get(ctx, req.NamespacedName, originalResource); err != nil {
		if apierrs.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return Result{}, nil
		}
		if !errors.Is(err, ErrQuiet) {
			log.Error(err, "unable to fetch resource")
		}

		return Result{}, err
	}
	resource := originalResource.DeepCopyObject().(T)

	if defaulter, ok := client.Object(resource).(webhook.CustomDefaulter); ok {
		// resource.Default(ctx, resource)
		if err := defaulter.Default(ctx, resource); err != nil {
			return Result{}, err
		}
	} else if defaulter, ok := client.Object(resource).(objectDefaulter); ok {
		// resource.Default()
		defaulter.Default()
	}

	if r.SkipResource(ctx, resource) {
		return Result{}, nil
	}

	r.initializeConditions(ctx, resource)
	result, err := r.reconcileInner(ctx, resource)

	if r.SkipStatusUpdate {
		return result, err
	}

	// attempt to restore last transition time for unchanged conditions
	r.syncLastTransitionTime(r.conditions(resource), r.conditions(originalResource))

	// check if status has changed before updating
	resourceStatus, originalResourceStatus := r.status(resource), r.status(originalResource)
	if !errors.Is(err, ErrSkipStatusUpdate) && !equality.Semantic.DeepEqual(resourceStatus, originalResourceStatus) && (resource.GetDeletionTimestamp() == nil || r.SyncStatusDuringFinalization) {
		if duck.IsDuck(resource, c.Scheme()) {
			// patch status
			log.Info("patching status", "diff", cmp.Diff(originalResourceStatus, resourceStatus, IgnoreAllUnexported))
			if patchErr := c.Status().Patch(ctx, resource, client.MergeFrom(originalResource)); patchErr != nil {
				if !errors.Is(patchErr, ErrQuiet) {
					log.Error(patchErr, "unable to patch status")
					c.Recorder.Eventf(resource, corev1.EventTypeWarning, "StatusPatchFailed",
						"Failed to patch status: %v", patchErr)
				}

				return result, patchErr
			}
			c.Recorder.Eventf(resource, corev1.EventTypeNormal, "StatusPatched",
				"Patched status")
		} else {
			// update status
			log.Info("updating status", "diff", cmp.Diff(originalResourceStatus, resourceStatus, IgnoreAllUnexported))
			if updateErr := c.Status().Update(ctx, resource); updateErr != nil {
				if !errors.Is(updateErr, ErrQuiet) {
					log.Error(updateErr, "unable to update status")
					c.Recorder.Eventf(resource, corev1.EventTypeWarning, "StatusUpdateFailed",
						"Failed to update status: %v", updateErr)
				}
				return result, updateErr
			}
			c.Recorder.Eventf(resource, corev1.EventTypeNormal, "StatusUpdated",
				"Updated status")
		}

		// Suppress result. Let the informer discover the resource mutation and requeue. Requeueing
		// now may result in re-processing a stale cache.
		return Result{}, nil
	}

	// return original reconcile result
	return result, err
}

func (r *ResourceReconciler[T]) reconcileInner(ctx context.Context, resource T) (Result, error) {
	if resource.GetDeletionTimestamp() != nil && len(resource.GetFinalizers()) == 0 {
		// resource is being deleted and has no pending finalizers, nothing to do
		return Result{}, nil
	}

	result, err := r.Reconciler.Reconcile(ctx, resource)
	if err != nil && !errors.Is(err, ErrHaltSubReconcilers) {
		return result, err
	}

	r.copyGeneration(resource)
	return result, err
}

func (r *ResourceReconciler[T]) initializeConditions(ctx context.Context, obj T) {
	status := r.status(obj)
	if status == nil {
		return
	}
	initializeConditions := reflect.ValueOf(status).MethodByName("InitializeConditions")
	if !initializeConditions.IsValid() {
		return
	}
	t := initializeConditions.Type()
	if t.Kind() != reflect.Func || t.NumOut() != 0 {
		return
	}
	args := []reflect.Value{}
	if t.NumIn() == 1 && t.In(0).AssignableTo(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		args = append(args, reflect.ValueOf(ctx))
	} else if t.NumIn() != 0 {
		return
	}
	initializeConditions.Call(args)
}

func (r *ResourceReconciler[T]) conditions(obj T) []metav1.Condition {
	// return obj.Status.Conditions
	status := r.status(obj)
	if status == nil {
		return nil
	}
	statusValue := reflect.ValueOf(status)
	if statusValue.Type().Kind() == reflect.Map {
		return nil
	}
	statusValue = statusValue.Elem()
	conditionsValue := statusValue.FieldByName("Conditions")
	if !conditionsValue.IsValid() || conditionsValue.IsZero() {
		return nil
	}
	conditions, ok := conditionsValue.Interface().([]metav1.Condition)
	if !ok {
		return nil
	}
	return conditions
}

func (r *ResourceReconciler[T]) copyGeneration(obj T) {
	// obj.Status.ObservedGeneration = obj.Generation
	status := r.status(obj)
	if status == nil {
		return
	}
	statusValue := reflect.ValueOf(status)
	if statusValue.Type().Kind() == reflect.Map {
		return
	}
	statusValue = statusValue.Elem()
	if !statusValue.IsValid() {
		return
	}
	observedGenerationValue := statusValue.FieldByName("ObservedGeneration")
	if observedGenerationValue.Kind() != reflect.Int64 || !observedGenerationValue.CanSet() {
		return
	}
	generation := obj.GetGeneration()
	observedGenerationValue.SetInt(generation)
}

func (r *ResourceReconciler[T]) hasStatus(obj T) bool {
	status := r.status(obj)
	return status != nil
}

func (r *ResourceReconciler[T]) status(obj T) interface{} {
	if client.Object(obj) == nil {
		return nil
	}
	if u, ok := client.Object(obj).(*unstructured.Unstructured); ok {
		return u.UnstructuredContent()["status"]
	}
	statusValue := reflect.ValueOf(obj).Elem().FieldByName("Status")
	if statusValue.Kind() == reflect.Ptr {
		statusValue = statusValue.Elem()
	}
	if !statusValue.IsValid() || !statusValue.CanAddr() {
		return nil
	}
	return statusValue.Addr().Interface()
}

// syncLastTransitionTime restores a condition's LastTransitionTime value for
// each proposed condition that is otherwise equivalent to the original value.
// This method is useful to prevent updating the status for a resource that is
// otherwise unchanged.
func (r *ResourceReconciler[T]) syncLastTransitionTime(proposed, original []metav1.Condition) {
	for _, o := range original {
		for i := range proposed {
			p := &proposed[i]
			if o.Type == p.Type {
				if o.Status == p.Status &&
					o.Reason == p.Reason &&
					o.Message == p.Message &&
					o.ObservedGeneration == p.ObservedGeneration {
					p.LastTransitionTime = o.LastTransitionTime
				}
				break
			}
		}
	}
}
