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
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"reconciler.io/runtime/internal"
)

var (
	_ SubReconciler[client.Object] = (*ChildReconciler[client.Object, client.Object, client.ObjectList])(nil)
)

var (
	OnlyReconcileChildStatus = errors.New("skip reconciler create/update/delete behavior for the child resource, while still reflecting the existing child's status on the reconciled resource")
)

// ChildReconciler is a sub reconciler that manages a single child resource for a reconciled
// resource. The reconciler will ensure that exactly one child will match the desired state by:
//   - creating a child if none exists
//   - updating an existing child
//   - removing an unneeded child
//   - removing extra children
//
// The flow for each reconciliation request is:
//   - DesiredChild
//   - ObjectManager#Manage
//   - ReflectChildStatusOnParent
//
// During setup, the child resource type is registered to watch for changes.
type ChildReconciler[Type, ChildType client.Object, ChildListType client.ObjectList] struct {
	// Name used to identify this reconciler.  Defaults to `{ChildType}ChildReconciler`.  Ideally
	// unique, but not required to be so.
	//
	// +optional
	Name string

	// ChildType is the resource being created/updated/deleted by the reconciler. For example, a
	// reconciled resource Deployment would have a ReplicaSet as a child. Required when the
	// generic type is not a struct, or is unstructured.
	//
	// +optional
	ChildType ChildType
	// ChildListType is the listing type for the child type. For example,
	// PodList is the list type for Pod. Required when the generic type is not
	// a struct, or is unstructured.
	//
	// +optional
	ChildListType ChildListType

	// SkipOwnerReference when true will not create and find child resources via an owner
	// reference. OurChild must be defined for the reconciler to distinguish the child being
	// reconciled from other resources of the same type.
	//
	// Any child resource created is tracked for changes.
	SkipOwnerReference bool

	// Setup performs initialization on the manager and builder this reconciler
	// will run with. It's common to setup field indexes and watch resources.
	//
	// +optional
	Setup func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error

	// DesiredChild returns the desired child object for the given reconciled resource, or nil if
	// the child should not exist.
	//
	// To skip reconciliation of the child resource while still reflecting an existing child's
	// status on the reconciled resource, return OnlyReconcileChildStatus as an error.
	DesiredChild func(ctx context.Context, resource Type) (ChildType, error)

	// ReflectChildStatusOnParent updates the reconciled resource's status with values from the
	// child. Select types of errors are passed, including:
	//   - apierrs.IsAlreadyExists
	//   - apierrs.IsInvalid
	//
	// Most errors are returned directly, skipping this method. The set of handled error types
	// may grow, implementations should be defensive rather than assuming the error type.
	ReflectChildStatusOnParent func(ctx context.Context, parent Type, child ChildType, err error)

	// ChildObjectManager synchronizes the desired child state to the API Server.
	ChildObjectManager ObjectManager[ChildType]

	// ListOptions allows custom options to be use when listing potential child resources. Each
	// resource retrieved as part of the listing is confirmed via OurChild. There is a performance
	// benefit to limiting the number of resource return for each List operation, however,
	// excluding an actual child will orphan that resource.
	//
	// Defaults to filtering by the reconciled resource's namespace:
	//     []client.ListOption{
	//         client.InNamespace(resource.GetNamespace()),
	//     }
	//
	// ListOptions is required when a Finalizer is defined or SkipOwnerReference is true. An empty
	// list is often sufficient although it may incur a performance penalty, especially when
	// querying the API sever instead of an informer cache.
	//
	// +optional
	ListOptions func(ctx context.Context, resource Type) []client.ListOption

	// OurChild is used when there are multiple ChildReconciler for the same ChildType controlled
	// by the same reconciled resource. The function return true for child resources managed by
	// this ChildReconciler. Objects returned from the DesiredChild function should match this
	// function, otherwise they may be orphaned. If not specified, all children match.
	//
	// OurChild is required when a Finalizer is defined or SkipOwnerReference is true.
	//
	// +optional
	OurChild func(resource Type, child ChildType) bool

	lazyInit sync.Once
}

func (r *ChildReconciler[T, CT, CLT]) init() {
	r.lazyInit.Do(func() {
		if internal.IsNil(r.ChildType) {
			var nilCT CT
			r.ChildType = newEmpty(nilCT).(CT)
		}
		if internal.IsNil(r.ChildListType) {
			var nilCLT CLT
			r.ChildListType = newEmpty(nilCLT).(CLT)
		}
		if r.Name == "" {
			r.Name = fmt.Sprintf("%sChildReconciler", typeName(r.ChildType))
		}
	})
}

func (r *ChildReconciler[T, CT, CLT]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	c := RetrieveConfigOrDie(ctx)

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name).
		WithValues("childType", gvk(c, r.ChildType))
	ctx = logr.NewContext(ctx, log)

	if err := r.validate(ctx); err != nil {
		return err
	}

	if !r.SkipOwnerReference {
		bldr.Owns(r.ChildType)
	}

	if err := r.ChildObjectManager.SetupWithManager(ctx, mgr, bldr); err != nil {
		return err
	}

	if r.Setup != nil {
		if err := r.Setup(ctx, mgr, bldr); err != nil {
			return err
		}
	}

	return nil
}

func (r *ChildReconciler[T, CT, CLT]) validate(ctx context.Context) error {
	// require DesiredChild
	if r.DesiredChild == nil {
		return fmt.Errorf("ChildReconciler %q must implement DesiredChild", r.Name)
	}

	// require ReflectChildStatusOnParent
	if r.ReflectChildStatusOnParent == nil {
		return fmt.Errorf("ChildReconciler %q must implement ReflectChildStatusOnParent", r.Name)
	}

	if r.OurChild == nil && r.SkipOwnerReference {
		// OurChild is required when SkipOwnerReference is true
		return fmt.Errorf("ChildReconciler %q must implement OurChild since owner references are not used", r.Name)
	}

	if r.ListOptions == nil && r.SkipOwnerReference {
		// ListOptions is required when SkipOwnerReference is true
		return fmt.Errorf("ChildReconciler %q must implement ListOptions since owner references are not used", r.Name)
	}

	// require ChildObjectManager
	if r.ChildObjectManager == nil {
		return fmt.Errorf("ChildReconciler %q must implement ChildObjectManager", r.Name)
	}

	return nil
}

func (r *ChildReconciler[T, CT, CLT]) Reconcile(ctx context.Context, resource T) (Result, error) {
	r.init()

	c := RetrieveConfigOrDie(ctx)

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name).
		WithValues("childType", gvk(c, r.ChildType))
	ctx = logr.NewContext(ctx, log)

	child, err := r.reconcile(ctx, resource)
	if resource.GetDeletionTimestamp() != nil {
		return Result{}, err
	}
	if err != nil {
		switch {
		case apierrs.IsAlreadyExists(err):
			// check if the resource blocking create is owned by the reconciled resource.
			// the created child from a previous turn may be slow to appear in the informer cache, but shouldn't appear
			// on the reconciled resource as being not ready.
			apierr := err.(apierrs.APIStatus)
			conflicted := r.ChildType.DeepCopyObject().(CT)
			_ = c.APIReader.Get(ctx, types.NamespacedName{Namespace: resource.GetNamespace(), Name: apierr.Status().Details.Name}, conflicted)
			if r.ourChild(resource, conflicted) {
				// skip updating the reconciled resource's status, fail and try again
				return Result{}, err
			}
			log.Info("unable to reconcile child, not owned", "child", namespaceName(conflicted), "ownerRefs", conflicted.GetOwnerReferences())
			r.ReflectChildStatusOnParent(ctx, resource, child, err)
			return Result{}, nil
		case apierrs.IsInvalid(err):
			r.ReflectChildStatusOnParent(ctx, resource, child, err)
			return Result{}, nil
		}
		if !errors.Is(err, ErrQuiet) {
			log.Error(err, "unable to reconcile child")
		}
		return Result{}, err
	}
	r.ReflectChildStatusOnParent(ctx, resource, child, nil)

	return Result{}, nil
}

func (r *ChildReconciler[T, CT, CLT]) reconcile(ctx context.Context, resource T) (CT, error) {
	var nilCT CT
	log := logr.FromContextOrDiscard(ctx)
	c := RetrieveConfigOrDie(ctx)

	actual := r.ChildType.DeepCopyObject().(CT)
	children := RetrieveKnownChildren[CT](ctx)
	if children == nil {
		// use existing known children when available, fall back to lookup
		list := r.ChildListType.DeepCopyObject().(CLT)
		if err := c.List(ctx, list, r.listOptions(ctx, resource)...); err != nil {
			return nilCT, err
		}
		children = extractItems[CT](list)
	}
	children = r.filterChildren(resource, children)
	if len(children) == 1 {
		actual = children[0]
	} else if len(children) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extra := range children {
			log.Info("extra child detected", "child", namespaceName(extra))
			if _, err := r.ChildObjectManager.Manage(ctx, resource, extra, nilCT); err != nil {
				return nilCT, err
			}
		}
	}

	desired, err := r.desiredChild(ctx, resource)
	if err != nil {
		if errors.Is(err, OnlyReconcileChildStatus) {
			return actual, nil
		}
		return nilCT, err
	}
	if !internal.IsNil(desired) {
		if !r.SkipOwnerReference && metav1.GetControllerOfNoCopy(desired) == nil {
			if err := ctrl.SetControllerReference(resource, desired, c.Scheme()); err != nil {
				return nilCT, err
			}
		}
		if !r.ourChild(resource, desired) {
			log.Info("object returned from DesiredChild does not match OurChild, this can result in orphaned children", "child", namespaceName(desired))
		}
	}

	// create/update/delete desired child
	return r.ChildObjectManager.Manage(ctx, resource, actual, desired)
}

func (r *ChildReconciler[T, CT, CLT]) desiredChild(ctx context.Context, resource T) (CT, error) {
	var nilCT CT

	if resource.GetDeletionTimestamp() != nil {
		// the reconciled resource is pending deletion, cleanup the child resource
		return nilCT, nil
	}

	return r.DesiredChild(ctx, resource)
}

func (r *ChildReconciler[T, CT, CLT]) filterChildren(resource T, children []CT) []CT {
	items := []CT{}
	for _, child := range children {
		if r.ourChild(resource, child) {
			items = append(items, child)
		}
	}
	return items
}

func (r *ChildReconciler[T, CT, CLT]) listOptions(ctx context.Context, resource T) []client.ListOption {
	if r.ListOptions == nil {
		return []client.ListOption{
			client.InNamespace(resource.GetNamespace()),
		}
	}
	return r.ListOptions(ctx, resource)
}

func (r *ChildReconciler[T, CT, CLT]) ourChild(resource T, obj CT) bool {
	if !r.SkipOwnerReference && !metav1.IsControlledBy(obj, resource) {
		return false
	}
	// TODO do we need to remove resources pending deletion?
	if r.OurChild == nil {
		return true
	}
	return r.OurChild(resource, obj)
}
