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
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	diecorev1 "reconciler.io/dies/apis/core/v1"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	"reconciler.io/runtime/tracker"
	"reconciler.io/runtime/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestConfig_TrackAndGet(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		})

	configMap := diecorev1.ConfigMapBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace("track-namespace")
			d.Name("track-name")
		}).
		AddData("greeting", "hello")

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"track and get": {
			Resource: resource.DieReleasePtr(),
			GivenObjects: []client.Object{
				configMap,
			},
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(configMap, resource, scheme),
			},
		},
		"track with not found get": {
			Resource:  resource.DieReleasePtr(),
			ShouldErr: true,
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(configMap, resource, scheme),
			},
		},
	}

	// run with typed objects
	t.Run("typed", func(t *testing.T) {
		rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
			return &reconcilers.SyncReconciler[*resources.TestResource]{
				Sync: func(ctx context.Context, resource *resources.TestResource) error {
					c := reconcilers.RetrieveConfigOrDie(ctx)

					cm := &corev1.ConfigMap{}
					err := c.TrackAndGet(ctx, types.NamespacedName{Namespace: "track-namespace", Name: "track-name"}, cm)
					if err != nil {
						return err
					}

					if expected, actual := "hello", cm.Data["greeting"]; expected != actual {
						// should never get here
						panic(fmt.Errorf("expected configmap to have greeting %q, found %q", expected, actual))
					}
					return nil
				},
			}
		})
	})

	// run with unstructured objects
	t.Run("unstructured", func(t *testing.T) {
		rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
			return &reconcilers.SyncReconciler[*resources.TestResource]{
				Sync: func(ctx context.Context, resource *resources.TestResource) error {
					c := reconcilers.RetrieveConfigOrDie(ctx)

					cm := &unstructured.Unstructured{}
					cm.SetAPIVersion("v1")
					cm.SetKind("ConfigMap")
					err := c.TrackAndGet(ctx, types.NamespacedName{Namespace: "track-namespace", Name: "track-name"}, cm)
					if err != nil {
						return err
					}

					if expected, actual := "hello", cm.UnstructuredContent()["data"].(map[string]interface{})["greeting"].(string); expected != actual {
						// should never get here
						panic(fmt.Errorf("expected configmap to have greeting %q, found %q", expected, actual))
					}
					return nil
				},
			}
		})
	})
}

func TestConfig_TrackAndList(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"
	testSelector, _ := labels.Parse("app=test-app")

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		})

	configMap := diecorev1.ConfigMapBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace("track-namespace")
			d.Name("track-name")
			d.AddLabel("app", "test-app")
		}).
		AddData("greeting", "hello")

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"track and list": {
			Resource: resource.DieReleasePtr(),
			GivenObjects: []client.Object{
				configMap,
			},
			Metadata: map[string]interface{}{
				"listOpts": []client.ListOption{},
			},
			ExpectTracks: []rtesting.TrackRequest{
				{
					Tracker: types.NamespacedName{
						Namespace: testNamespace,
						Name:      testName,
					},
					TrackedReference: tracker.Reference{
						Kind:     "ConfigMap",
						Selector: labels.Everything(),
					},
				},
			},
		},
		"track and list constrained": {
			Resource: resource.DieReleasePtr(),
			GivenObjects: []client.Object{
				configMap,
			},
			Metadata: map[string]interface{}{
				"listOpts": []client.ListOption{
					client.InNamespace("track-namespace"),
					client.MatchingLabels(map[string]string{"app": "test-app"}),
				},
			},
			ExpectTracks: []rtesting.TrackRequest{
				{
					Tracker: types.NamespacedName{
						Namespace: testNamespace,
						Name:      testName,
					},
					TrackedReference: tracker.Reference{
						Kind:      "ConfigMap",
						Namespace: "track-namespace",
						Selector:  testSelector,
					},
				},
			},
		},
		"track with errored list": {
			Resource:  resource.DieReleasePtr(),
			ShouldErr: true,
			WithReactors: []rtesting.ReactionFunc{
				rtesting.InduceFailure("list", "ConfigMapList"),
			},
			Metadata: map[string]interface{}{
				"listOpts": []client.ListOption{},
			},
			ExpectTracks: []rtesting.TrackRequest{
				{
					Tracker: types.NamespacedName{
						Namespace: testNamespace,
						Name:      testName,
					},
					TrackedReference: tracker.Reference{
						Kind:     "ConfigMap",
						Selector: labels.Everything(),
					},
				},
			},
		},
	}

	// run with typed objects
	t.Run("typed", func(t *testing.T) {
		rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
			return &reconcilers.SyncReconciler[*resources.TestResource]{
				Sync: func(ctx context.Context, resource *resources.TestResource) error {
					c := reconcilers.RetrieveConfigOrDie(ctx)

					cms := &corev1.ConfigMapList{}
					listOpts := rtc.Metadata["listOpts"].([]client.ListOption)
					err := c.TrackAndList(ctx, cms, listOpts...)
					if err != nil {
						return err
					}

					if expected, actual := "hello", cms.Items[0].Data["greeting"]; expected != actual {
						// should never get here
						panic(fmt.Errorf("expected configmap to have greeting %q, found %q", expected, actual))
					}
					return nil
				},
			}
		})
	})

	// run with unstructured objects
	t.Run("unstructured", func(t *testing.T) {
		rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
			return &reconcilers.SyncReconciler[*resources.TestResource]{
				Sync: func(ctx context.Context, resource *resources.TestResource) error {
					c := reconcilers.RetrieveConfigOrDie(ctx)

					cms := &unstructured.UnstructuredList{}
					cms.SetAPIVersion("v1")
					cms.SetKind("ConfigMapList")
					listOpts := rtc.Metadata["listOpts"].([]client.ListOption)
					err := c.TrackAndList(ctx, cms, listOpts...)
					if err != nil {
						return err
					}

					if expected, actual := "hello", cms.UnstructuredContent()["items"].([]interface{})[0].(map[string]interface{})["data"].(map[string]interface{})["greeting"].(string); expected != actual {
						// should never get here
						panic(fmt.Errorf("expected configmap to have greeting %q, found %q", expected, actual))
					}
					return nil
				},
			}
		})
	})
}

func TestWithConfig(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		})

	rts := rtesting.SubReconcilerTests[*resources.TestResource]{
		"with config": {
			Resource: resource.DieReleasePtr(),
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, oc reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					c := reconcilers.Config{
						Tracker: tracker.New(oc.Scheme(), 0),
					}

					return &reconcilers.WithConfig[*resources.TestResource]{
						Config: func(ctx context.Context, _ reconcilers.Config) (reconcilers.Config, error) {
							return c, nil
						},
						Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, parent *resources.TestResource) error {
								rc := reconcilers.RetrieveConfigOrDie(ctx)
								roc := reconcilers.RetrieveOriginalConfigOrDie(ctx)

								if rc != c {
									t.Errorf("unexpected config")
								}
								if roc != oc {
									t.Errorf("unexpected original config")
								}

								oc.Recorder.Event(resource, corev1.EventTypeNormal, "AllGood", "")

								return nil
							},
						},
					}
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(resource, scheme, corev1.EventTypeNormal, "AllGood", ""),
			},
		},
	}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*resources.TestResource], c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
		return rtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource])(t, c)
	})
}

func TestWithConfig_Validate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	config := reconcilers.Config{
		Tracker: tracker.New(scheme, 0),
	}

	tests := []struct {
		name           string
		resource       client.Object
		reconciler     *reconcilers.WithConfig[*corev1.ConfigMap]
		validateNested bool
		shouldErr      string
	}{
		{
			name:       "empty",
			resource:   &corev1.ConfigMap{},
			reconciler: &reconcilers.WithConfig[*corev1.ConfigMap]{},
			shouldErr:  `WithConfig "" must define Config`,
		},
		{
			name:     "valid",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithConfig[*corev1.ConfigMap]{
				Reconciler: &reconcilers.Sequence[*corev1.ConfigMap]{},
				Config: func(ctx context.Context, c reconcilers.Config) (reconcilers.Config, error) {
					return config, nil
				},
			},
		},
		{
			name:     "missing config",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithConfig[*corev1.ConfigMap]{
				Name:       "missing config",
				Reconciler: &reconcilers.Sequence[*corev1.ConfigMap]{},
			},
			shouldErr: `WithConfig "missing config" must define Config`,
		},
		{
			name:     "missing reconciler",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithConfig[*corev1.ConfigMap]{
				Name: "missing reconciler",
				Config: func(ctx context.Context, c reconcilers.Config) (reconcilers.Config, error) {
					return config, nil
				},
			},
			shouldErr: `WithConfig "missing reconciler" must define Reconciler`,
		},
		{
			name:     "valid reconciler",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithConfig[*corev1.ConfigMap]{
				Config: func(ctx context.Context, c reconcilers.Config) (reconcilers.Config, error) {
					return config, nil
				},
				Reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
					Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name:     "invalid reconciler",
			resource: &corev1.ConfigMap{},
			reconciler: &reconcilers.WithConfig[*corev1.ConfigMap]{
				Config: func(ctx context.Context, c reconcilers.Config) (reconcilers.Config, error) {
					return config, nil
				},
				Reconciler: &reconcilers.SyncReconciler[*corev1.ConfigMap]{
					// Sync: func(ctx context.Context, resource *corev1.ConfigMap) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr: `WithConfig "" must have a valid Reconciler: SyncReconciler "" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.TODO()
			if c.validateNested {
				ctx = validation.WithRecursive(ctx)
			}
			err := c.reconciler.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
		})
	}
}
