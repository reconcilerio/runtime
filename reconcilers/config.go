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
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"

	"github.com/go-logr/logr"
	"reconciler.io/runtime/duck"
	"reconciler.io/runtime/tracker"
	"reconciler.io/runtime/validation"
)

// Config holds common resources for controllers. The configuration may be
// passed to sub-reconcilers.
type Config struct {
	client.Client
	APIReader client.Reader
	Discovery discovery.DiscoveryInterface
	Recorder  record.EventRecorder
	Tracker   tracker.Tracker

	syncPeriod time.Duration
}

func (c Config) IsEmpty() bool {
	return c == Config{}
}

// WithCluster extends the config to access a new cluster.
func (c Config) WithCluster(cluster cluster.Cluster) Config {
	return Config{
		Client:    duck.NewDuckAwareClientWrapper(cluster.GetClient()),
		APIReader: duck.NewDuckAwareAPIReaderWrapper(cluster.GetAPIReader(), cluster.GetClient()),
		Discovery: discovery.NewDiscoveryClientForConfigOrDie(cluster.GetConfig()),
		Recorder:  cluster.GetEventRecorderFor("controller"),
		Tracker:   c.Tracker,

		syncPeriod: c.syncPeriod,
	}
}

// WithTracker extends the config with a new tracker.
func (c Config) WithTracker() Config {
	return Config{
		Client:    c.Client,
		APIReader: c.APIReader,
		Discovery: c.Discovery,
		Recorder:  c.Recorder,
		Tracker:   tracker.New(c.Scheme(), 2*c.syncPeriod),

		syncPeriod: c.syncPeriod,
	}
}

// WithDangerousDuckClientOperations returns a new Config with client Create and Update methods for
// duck typed objects enabled.
//
// This is dangerous because duck types typically represent a subset of the target resource and may
// cause data loss if the resource's server representation contains fields that do not exist on the
// duck typed object.
func (c Config) WithDangerousDuckClientOperations() Config {
	return Config{
		Client:    duck.NewDangerousDuckAwareClientWrapper(c.Client),
		APIReader: c.APIReader,
		Discovery: c.Discovery,
		Recorder:  c.Recorder,
		Tracker:   c.Tracker,

		syncPeriod: c.syncPeriod,
	}
}

// TrackAndGet tracks the resources for changes and returns the current value. The track is
// registered even when the resource does not exists so that its creation can be tracked.
//
// Equivalent to calling both `c.Tracker.TrackObject(...)` and `c.Client.Get(...)`
func (c Config) TrackAndGet(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	// create synthetic resource to track from known type and request
	req := RetrieveRequest(ctx)
	resource := RetrieveResourceType(ctx).DeepCopyObject().(client.Object)
	resource.SetNamespace(req.Namespace)
	resource.SetName(req.Name)
	ref := obj.DeepCopyObject().(client.Object)
	ref.SetNamespace(key.Namespace)
	ref.SetName(key.Name)
	c.Tracker.TrackObject(ref, resource)

	return c.Get(ctx, key, obj, opts...)
}

// TrackAndList tracks the resources for changes and returns the current value.
//
// Equivalent to calling both `c.Tracker.TrackReference(...)` and `c.Client.List(...)`
func (c Config) TrackAndList(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	// create synthetic resource to track from known type and request
	req := RetrieveRequest(ctx)
	resource := RetrieveResourceType(ctx).DeepCopyObject().(client.Object)
	resource.SetNamespace(req.Namespace)
	resource.SetName(req.Name)

	or, err := reference.GetReference(c.Scheme(), list)
	if err != nil {
		return err
	}
	gvk := schema.FromAPIVersionAndKind(or.APIVersion, or.Kind)
	listOpts := (&client.ListOptions{}).ApplyOptions(opts)
	if listOpts.LabelSelector == nil {
		listOpts.LabelSelector = labels.Everything()
	}
	ref := tracker.Reference{
		APIGroup:  gvk.Group,
		Kind:      strings.TrimSuffix(gvk.Kind, "List"),
		Namespace: listOpts.Namespace,
		Selector:  listOpts.LabelSelector,
	}
	c.Tracker.TrackReference(ref, resource)

	return c.List(ctx, list, opts...)
}

// NewConfig creates a Config for a specific API type. Typically passed into a
// reconciler.
func NewConfig(mgr ctrl.Manager, apiType client.Object, syncPeriod time.Duration) Config {
	return Config{syncPeriod: syncPeriod}.WithCluster(mgr).WithTracker()
}

var _ SubReconciler[client.Object] = (*WithConfig[client.Object])(nil)

// Experimental: WithConfig injects the provided config into the reconcilers nested under it. For
// example, the client can be swapped to use a service account with different permissions, or to
// target an entirely different cluster.
//
// The specified config can be accessed with `RetrieveConfig(ctx)`, the original config used to
// load the reconciled resource can be accessed with `RetrieveOriginalConfig(ctx)`.
type WithConfig[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `WithConfig`.  Ideally unique, but
	// not required to be so.
	//
	// +optional
	Name string

	// Config to use for this portion of the reconciler hierarchy. This method is called during
	// setup and during reconciliation, if context is needed, it should be available durring both
	// phases.
	Config func(context.Context, Config) (Config, error)

	// Reconciler is called for each reconciler request with the reconciled
	// resource being reconciled. Typically a Sequence is used to compose
	// multiple SubReconcilers.
	Reconciler SubReconciler[Type]

	lazyInit sync.Once
}

func (r *WithConfig[T]) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	if err := r.Validate(ctx); err != nil {
		return err
	}
	c, err := r.Config(ctx, RetrieveConfigOrDie(ctx))
	if err != nil {
		return err
	}
	ctx = StashConfig(ctx, c)
	return r.Reconciler.SetupWithManager(ctx, mgr, bldr)
}

func (r *WithConfig[T]) init() {
	r.lazyInit.Do(func() {
		if r.Name == "" {
			r.Name = "WithConfig"
		}
	})
}

func (r *WithConfig[T]) Validate(ctx context.Context) error {
	r.init()

	// validate Config value
	if r.Config == nil {
		return fmt.Errorf("WithConfig %q must define Config", r.Name)
	}

	// validate Reconciler value
	if r.Reconciler == nil {
		return fmt.Errorf("WithConfig %q must define Reconciler", r.Name)
	}
	if validation.IsRecursive(ctx) {
		if v, ok := r.Reconciler.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("WithConfig %q must have a valid Reconciler: %w", r.Name, err)
			}
		}
	}

	return nil
}

func (r *WithConfig[T]) Reconcile(ctx context.Context, resource T) (Result, error) {
	log := logr.FromContextOrDiscard(ctx).
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	c, err := r.Config(ctx, RetrieveConfigOrDie(ctx))
	if err != nil {
		return Result{}, err
	}
	ctx = StashConfig(ctx, c)
	return r.Reconciler.Reconcile(ctx, resource)
}
