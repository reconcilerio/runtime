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

package reconcilers_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	diecorev1 "reconciler.io/dies/apis/core/v1"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	rtime "reconciler.io/runtime/time"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestAggregateReconciler(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testFinalizer := "test.finalizer"
	request := reconcilers.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testName},
	}

	now := metav1.NewTime(time.Now().Truncate(time.Second))

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	configMapCreate := diecorev1.ConfigMapBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		})
	configMapGiven := configMapCreate.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
		})

	defaultAggregateReconciler := func(c reconcilers.Config) *reconcilers.AggregateReconciler[*corev1.ConfigMap] {
		return &reconcilers.AggregateReconciler[*corev1.ConfigMap]{
			Request: request,

			DesiredResource: func(ctx context.Context, resource *corev1.ConfigMap) (*corev1.ConfigMap, error) {
				resource.Data = map[string]string{
					"foo": "bar",
				}
				return resource, nil
			},
			AggregateObjectManager: &rtesting.StubObjectManager[*corev1.ConfigMap]{},

			Config: c,
		}
	}

	rts := rtesting.ReconcilerTests{
		"resource is in sync": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					return defaultAggregateReconciler(c)
				},
			},
		},
		"ignore other resources": {
			Request: reconcilers.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "not-it"},
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					return defaultAggregateReconciler(c)
				},
			},
		},
		"ignore terminating resources": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.DeletionTimestamp(&now)
						d.Finalizers(testFinalizer)
					}),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					return defaultAggregateReconciler(c)
				},
			},
		},
		"create resource": {
			Request: request,
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					return defaultAggregateReconciler(c)
				},
			},
			ExpectCreates: []client.Object{
				configMapCreate.
					AddData("foo", "bar"),
			},
		},
		"update resource": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					return defaultAggregateReconciler(c)
				},
			},
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
		},
		"delete resource": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.DesiredResource = func(ctx context.Context, resource *corev1.ConfigMap) (*corev1.ConfigMap, error) {
						return nil, nil
					}
					return r
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapGiven, scheme),
			},
		},
		"error getting resources": {
			Request: request,
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("get", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					return defaultAggregateReconciler(c)
				},
			},
			ShouldErr: true,
		},
		"error creating resource": {
			Request: request,
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("create", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					return defaultAggregateReconciler(c)
				},
			},
			ExpectCreates: []client.Object{
				configMapCreate.
					AddData("foo", "bar"),
			},
			ShouldErr: true,
		},
		"error updating resource": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("update", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					return defaultAggregateReconciler(c)
				},
			},
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
			ShouldErr: true,
		},
		"error deleting resource": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("delete", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.DesiredResource = func(ctx context.Context, resource *corev1.ConfigMap) (*corev1.ConfigMap, error) {
						return nil, nil
					}
					return r
				},
			},
			ExpectDeletes: []rtesting.DeleteRef{
				rtesting.NewDeleteRefFromObject(configMapGiven, scheme),
			},
			ShouldErr: true,
		},
		"reconcile result": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.Reconciler = &reconcilers.SyncReconciler[*corev1.ConfigMap]{
						SyncWithResult: func(ctx context.Context, resource *corev1.ConfigMap) (reconcilers.Result, error) {
							return reconcilers.Result{RequeueAfter: time.Hour}, nil
						},
					}
					return r
				},
			},
			ExpectedResult: reconcilers.Result{RequeueAfter: time.Hour},
		},
		"reconcile error": {
			Request: request,
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.Reconciler = &reconcilers.SyncReconciler[*corev1.ConfigMap]{
						Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
							return fmt.Errorf("test error")
						},
					}
					return r
				},
			},
			ShouldErr: true,
		},
		"reconcile halted": {
			Request: request,
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.Reconciler = reconcilers.Sequence[*corev1.ConfigMap]{
						&reconcilers.SyncReconciler[*corev1.ConfigMap]{
							Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
								return reconcilers.ErrHaltSubReconcilers
							},
						},
						&reconcilers.SyncReconciler[*corev1.ConfigMap]{
							Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
								return fmt.Errorf("test error")
							},
						},
					}
					return r
				},
			},
			ExpectCreates: []client.Object{
				configMapCreate.
					AddData("foo", "bar"),
			},
		},
		"reconcile halted with result": {
			Request: request,
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.Reconciler = reconcilers.Sequence[*corev1.ConfigMap]{
						&reconcilers.SyncReconciler[*corev1.ConfigMap]{
							SyncWithResult: func(ctx context.Context, resource *corev1.ConfigMap) (reconcilers.Result, error) {
								return reconcilers.Result{Requeue: true}, reconcilers.ErrHaltSubReconcilers
							},
						},
						&reconcilers.SyncReconciler[*corev1.ConfigMap]{
							Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
								return fmt.Errorf("test error")
							},
						},
					}
					return r
				},
			},
			ExpectCreates: []client.Object{
				configMapCreate.
					AddData("foo", "bar"),
			},
			ExpectedResult: reconcilers.Result{Requeue: true},
		},
		"context is stashable": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.Reconciler = &reconcilers.SyncReconciler[*corev1.ConfigMap]{
						Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
							var key reconcilers.StashKey = "foo"
							// StashValue will panic if the context is not setup correctly
							reconcilers.StashValue(ctx, key, "bar")
							return nil
						},
					}
					return r
				},
			},
		},
		"context has config": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.Reconciler = &reconcilers.SyncReconciler[*corev1.ConfigMap]{
						Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
							if config := reconcilers.RetrieveConfigOrDie(ctx); config != c {
								t.Errorf("expected config in context, found %#v", config)
							}
							if resourceConfig := reconcilers.RetrieveOriginalConfigOrDie(ctx); resourceConfig != c {
								t.Errorf("expected original config in context, found %#v", resourceConfig)
							}
							return nil
						},
					}
					return r
				},
			},
		},
		"context has resource type": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.Reconciler = &reconcilers.SyncReconciler[*corev1.ConfigMap]{
						Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
							if resourceType, ok := reconcilers.RetrieveOriginalResourceType(ctx).(*corev1.ConfigMap); !ok {
								t.Errorf("expected original resource type not in context, found %#v", resourceType)
							}
							if resourceType, ok := reconcilers.RetrieveResourceType(ctx).(*corev1.ConfigMap); !ok {
								t.Errorf("expected resource type not in context, found %#v", resourceType)
							}
							return nil
						},
					}
					return r
				},
			},
		},
		"context can be augmented in Prepare and accessed in Cleanup": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
			Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) (context.Context, error) {
				key := "test-key"
				value := "test-value"
				ctx = context.WithValue(ctx, key, value)

				tc.Metadata["Reconciler"] = func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.Reconciler = &reconcilers.SyncReconciler[*corev1.ConfigMap]{
						Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
							if v := ctx.Value(key); v != value {
								t.Errorf("expected %s to be in context", key)
							}
							return nil
						},
					}
					return r
				}
				tc.CleanUp = func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) error {
					if v := ctx.Value(key); v != value {
						t.Errorf("expected %s to be in context", key)
					}
					return nil
				}

				return ctx, nil
			},
		},
		"before reconcile is called before reconcile and after the context is populated": {
			Request: request,
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.BeforeReconcile = func(ctx context.Context, req reconcile.Request) (context.Context, reconcile.Result, error) {
						c := reconcilers.RetrieveConfigOrDie(ctx)
						// create the object manually rather than as a given
						r := configMapCreate.
							MetadataDie(func(d *diemetav1.ObjectMetaDie) {
								d.CreationTimestamp(metav1.NewTime(rtime.RetrieveNow(ctx)))
							}).
							DieReleasePtr()
						err := c.Create(ctx, r)
						return nil, reconcile.Result{}, err
					}
					return r
				},
			},
			ExpectCreates: []client.Object{
				configMapCreate,
			},
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
		},
		"before reconcile can influence the result": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.BeforeReconcile = func(ctx context.Context, req reconcile.Request) (context.Context, reconcile.Result, error) {
						return nil, reconcile.Result{Requeue: true}, nil
					}
					return r
				},
			},
			ExpectedResult: reconcile.Result{
				Requeue: true,
			},
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
		},
		"before reconcile can replace the context": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.BeforeReconcile = func(ctx context.Context, req reconcile.Request) (context.Context, reconcile.Result, error) {
						ctx = context.WithValue(ctx, "message", "hello world")
						return ctx, reconcile.Result{}, nil
					}
					r.DesiredResource = func(ctx context.Context, resource *corev1.ConfigMap) (*corev1.ConfigMap, error) {
						resource.Data = map[string]string{
							"message": ctx.Value("message").(string),
						}
						return resource, nil
					}
					return r
				},
			},
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("message", "hello world"),
			},
		},
		"before reconcile errors shortcut execution": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven,
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.BeforeReconcile = func(ctx context.Context, req reconcile.Request) (context.Context, reconcile.Result, error) {
						return nil, reconcile.Result{}, errors.New("test")
					}
					return r
				},
			},
			ShouldErr: true,
		},
		"after reconcile can overwrite the result": {
			Request: request,
			GivenObjects: []client.Object{
				configMapGiven,
			},
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("update", "ConfigMap"),
			},
			Metadata: map[string]interface{}{
				"Reconciler": func(t *testing.T, c reconcilers.Config) reconcile.Reconciler {
					r := defaultAggregateReconciler(c)
					r.AfterReconcile = func(ctx context.Context, req reconcile.Request, res reconcile.Result, err error) (reconcile.Result, error) {
						// suppress error
						return reconcile.Result{}, nil
					}
					return r
				},
			},
			ExpectUpdates: []client.Object{
				configMapGiven.
					AddData("foo", "bar"),
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		return rtc.Metadata["Reconciler"].(func(*testing.T, reconcilers.Config) reconcile.Reconciler)(t, c)
	})
}

func TestAggregateReconciler_Validate(t *testing.T) {
	req := reconcilers.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "my-namespace",
			Name:      "my-name",
		},
	}

	tests := []struct {
		name         string
		reconciler   *reconcilers.AggregateReconciler[*resources.TestResource]
		shouldErr    string
		expectedLogs []string
	}{
		{
			name:       "empty",
			reconciler: &reconcilers.AggregateReconciler[*resources.TestResource]{},
			shouldErr:  `AggregateReconciler "" must define Request`,
		},
		{
			name: "valid",
			reconciler: &reconcilers.AggregateReconciler[*resources.TestResource]{
				Type:                   &resources.TestResource{},
				Request:                req,
				Reconciler:             reconcilers.Sequence[*resources.TestResource]{},
				AggregateObjectManager: &reconcilers.UpdatingObjectManager[*resources.TestResource]{},
			},
		},
		{
			name: "Type missing",
			reconciler: &reconcilers.AggregateReconciler[*resources.TestResource]{
				Name: "Type missing",
				// Type:              &resources.TestResource{},
				Request:                req,
				Reconciler:             reconcilers.Sequence[*resources.TestResource]{},
				AggregateObjectManager: &reconcilers.UpdatingObjectManager[*resources.TestResource]{},
			},
		},
		{
			name: "Request missing",
			reconciler: &reconcilers.AggregateReconciler[*resources.TestResource]{
				Name: "Request missing",
				Type: &resources.TestResource{},
				// Request:           req,
				Reconciler:             reconcilers.Sequence[*resources.TestResource]{},
				AggregateObjectManager: &reconcilers.UpdatingObjectManager[*resources.TestResource]{},
			},
			shouldErr: `AggregateReconciler "Request missing" must define Request`,
		},
		{
			name: "Reconciler missing",
			reconciler: &reconcilers.AggregateReconciler[*resources.TestResource]{
				Name:    "Reconciler missing",
				Type:    &resources.TestResource{},
				Request: req,
				// Reconciler:        Sequence{},
				AggregateObjectManager: &reconcilers.UpdatingObjectManager[*resources.TestResource]{},
			},
			shouldErr: `AggregateReconciler "Reconciler missing" must define Reconciler and/or DesiredResource`,
		},
		{
			name: "DesiredResource",
			reconciler: &reconcilers.AggregateReconciler[*resources.TestResource]{
				Type:       &resources.TestResource{},
				Request:    req,
				Reconciler: reconcilers.Sequence[*resources.TestResource]{},
				DesiredResource: func(ctx context.Context, resource *resources.TestResource) (*resources.TestResource, error) {
					return nil, nil
				},
				AggregateObjectManager: &reconcilers.UpdatingObjectManager[*resources.TestResource]{},
			},
		},
		{
			name: "AggregateObjectManager missing",
			reconciler: &reconcilers.AggregateReconciler[*resources.TestResource]{
				Name:       "AggregateObjectManager missing",
				Type:       &resources.TestResource{},
				Request:    req,
				Reconciler: reconcilers.Sequence[*resources.TestResource]{},
				// AggregateObjectManager: &UpdatingObjectManager[*resources.TestResource]{},
			},
			shouldErr: `AggregateReconciler "AggregateObjectManager missing" must define AggregateObjectManager`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			sink := &bufferedSink{}
			ctx := logr.NewContext(context.TODO(), logr.New(sink))
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
			if diff := cmp.Diff(c.expectedLogs, sink.Lines); diff != "" {
				t.Errorf("%s: unexpected logs (-expected, +actual): %s", c.name, diff)
			}
		})
	}
}
