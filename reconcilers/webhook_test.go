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
	"net/http"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	jsonpatch "gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	dieadmissionv1 "reconciler.io/dies/apis/admission/v1"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/apis"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
	rtesting "reconciler.io/runtime/testing"
	"reconciler.io/runtime/tracker"
	"reconciler.io/runtime/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestAdmissionWebhookAdapter(t *testing.T) {
	testNamespace := "test-namespace"
	testName := "test-resource"

	scheme := runtime.NewScheme()
	_ = resources.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	resource := dies.TestResourceBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(testNamespace)
			d.Name(testName)
		}).
		StatusDie(func(d *dies.TestResourceStatusDie) {
			d.ConditionsDie(
				diemetav1.ConditionBlank.Type(apis.ConditionReady).Status(metav1.ConditionUnknown).Reason("Initializing"),
			)
		})

	requestUID := types.UID("9deefaa1-2c90-4f40-9c7b-3f5c1fd75dde")

	httpRequest := &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Path: "/path"},
	}
	request := dieadmissionv1.AdmissionRequestBlank.
		Namespace(testNamespace).
		Name(testName).
		UID(requestUID).
		Operation(admissionv1.Create)
	response := dieadmissionv1.AdmissionResponseBlank.
		Allowed(true)

	wts := rtesting.AdmissionWebhookTests{
		"allowed by default with no mutation": {
			Request: &admission.Request{
				AdmissionRequest: request.
					Object(resource.DieReleaseRawExtension()).
					DieRelease(),
			},
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.DieRelease(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return nil
						},
					}
				},
			},
		},
		"mutations generate patches in response": {
			Request: &admission.Request{
				AdmissionRequest: request.
					Object(resource.DieReleaseRawExtension()).
					DieRelease(),
			},
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.DieRelease(),
				Patches: []jsonpatch.Operation{
					{
						Operation: "add",
						Path:      "/spec/fields",
						Value: map[string]interface{}{
							"hello": "world",
						},
					},
				},
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							resource.Spec.Fields = map[string]string{
								"hello": "world",
							}
							return nil
						},
					}
				},
			},
		},
		"reconcile errors return http errors": {
			Request: &admission.Request{
				AdmissionRequest: request.
					Object(resource.DieReleaseRawExtension()).
					DieRelease(),
			},
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.
					Allowed(false).
					ResultDie(func(d *diemetav1.StatusDie) {
						d.Code(500)
						d.Message("reconcile error")
					}).
					DieRelease(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return fmt.Errorf("reconcile error")
						},
					}
				},
			},
		},
		"invalid json returns http errors": {
			Request: &admission.Request{
				AdmissionRequest: request.
					Object(runtime.RawExtension{
						Raw: []byte("{"),
					}).
					DieRelease(),
			},
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.
					Allowed(false).
					ResultDie(func(d *diemetav1.StatusDie) {
						d.Code(500)
						d.Message("unexpected end of JSON input")
					}).
					DieRelease(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							return nil
						},
					}
				},
			},
		},
		"delete operations load resource from old object": {
			Request: &admission.Request{
				AdmissionRequest: request.
					Operation(admissionv1.Delete).
					OldObject(
						resource.
							SpecDie(func(d *dies.TestResourceSpecDie) {
								d.AddField("hello", "world")
							}).
							DieReleaseRawExtension(),
					).
					DieRelease(),
			},
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.DieRelease(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if resource.Spec.Fields["hello"] != "world" {
								t.Errorf("expected field %q to have value %q", "hello", "world")
							}
							return nil
						},
					}
				},
			},
		},
		"config observations are asserted, should be extremely rare in standard use": {
			Request: &admission.Request{
				AdmissionRequest: request.
					Object(resource.DieReleaseRawExtension()).
					DieRelease(),
			},
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.DieRelease(),
			},
			GivenObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      testName,
					},
				},
			},
			ExpectTracks: []rtesting.TrackRequest{
				{
					Tracker: types.NamespacedName{
						Namespace: testNamespace,
						Name:      testName,
					},
					Tracked: tracker.Key{
						GroupKind: schema.GroupKind{
							Group: "",
							Kind:  "ConfigMap",
						},
						NamespacedName: types.NamespacedName{
							Namespace: testNamespace,
							Name:      testName,
						},
					},
				},
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							c := reconcilers.RetrieveConfigOrDie(ctx)
							cm := &corev1.ConfigMap{}
							if err := c.TrackAndGet(ctx, types.NamespacedName{Namespace: testNamespace, Name: testName}, cm); err != nil {
								t.Errorf("unexpected client error: %s", err)
							}
							return nil
						},
					}
				},
			},
		},
		"context is defined": {
			Request: &admission.Request{
				AdmissionRequest: request.
					Object(resource.DieReleaseRawExtension()).
					DieRelease(),
			},
			HTTPRequest: httpRequest,
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.DieRelease(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, _ *resources.TestResource) error {
							actualRequest := reconcilers.RetrieveAdmissionRequest(ctx)
							expectedRequest := admission.Request{
								AdmissionRequest: request.
									Object(resource.DieReleaseRawExtension()).
									DieRelease(),
							}
							if diff := cmp.Diff(actualRequest, expectedRequest); diff != "" {
								t.Errorf("expected stashed admission request to match actual request: %s", diff)
							}

							actualResponse := reconcilers.RetrieveAdmissionResponse(ctx)
							expectedResponse := &admission.Response{
								AdmissionResponse: dieadmissionv1.AdmissionResponseBlank.
									Allowed(true).
									UID(requestUID).
									DieRelease(),
							}
							if diff := cmp.Diff(actualResponse, expectedResponse); diff != "" {
								t.Errorf("expected stashed admission response to match actual response: %s", diff)
							}

							actualHTTPRequest := reconcilers.RetrieveHTTPRequest(ctx)
							expectedHTTPRequest := httpRequest
							if actualHTTPRequest != expectedHTTPRequest {
								t.Errorf("expected stashed http request to match actual request")
							}

							return nil
						},
					}
				},
			},
		},
		"context is stashable": {
			Request: &admission.Request{
				AdmissionRequest: request.
					Object(resource.DieReleaseRawExtension()).
					DieRelease(),
			},
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.DieRelease(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return reconcilers.Sequence[*resources.TestResource]{
						&reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, _ *resources.TestResource) error {
								// StashValue will panic if context is not setup for stashing
								reconcilers.StashValue(ctx, reconcilers.StashKey("greeting"), "hello")

								return nil
							},
						},
						&reconcilers.SyncReconciler[*resources.TestResource]{
							Sync: func(ctx context.Context, _ *resources.TestResource) error {
								// StashValue will panic if context is not setup for stashing
								greeting := reconcilers.RetrieveValue(ctx, reconcilers.StashKey("greeting"))
								if greeting != "hello" {
									t.Errorf("unexpected stash value retrieved")
								}

								return nil
							},
						},
					}
				},
			},
		},
		"context can be augmented in Prepare and accessed in Cleanup": {
			Request: &admission.Request{
				AdmissionRequest: request.
					Object(resource.DieReleaseRawExtension()).
					DieRelease(),
			},
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.DieRelease(),
			},
			Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.AdmissionWebhookTestCase) (context.Context, error) {
				key := reconcilers.StashKey("test-key")
				value := "test-value"
				ctx = context.WithValue(ctx, key, value)

				tc.Metadata["SubReconciler"] = func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						Sync: func(ctx context.Context, resource *resources.TestResource) error {
							if v := ctx.Value(key); v != value {
								t.Errorf("expected %s to be in context", key)
							}
							return nil
						},
					}
				}
				tc.CleanUp = func(t *testing.T, ctx context.Context, tc *rtesting.AdmissionWebhookTestCase) error {
					if v := ctx.Value(key); v != value {
						t.Errorf("expected %s to be in context", key)
					}
					return nil
				}

				return ctx, nil
			},
		},
		"invalid nested reconciler": {
			Skip: true,
			Request: &admission.Request{
				AdmissionRequest: request.
					Object(resource.DieReleaseRawExtension()).
					DieRelease(),
			},
			ExpectedResponse: admission.Response{
				AdmissionResponse: response.DieRelease(),
			},
			Metadata: map[string]interface{}{
				"SubReconciler": func(t *testing.T, c reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource] {
					return &reconcilers.SyncReconciler[*resources.TestResource]{
						// Sync: func(ctx context.Context, resource *resources.TestResource) error {
						// 	return nil
						// },
					}
				},
			},
		},
	}

	wts.RunWithContext(t, scheme, func(t *testing.T, ctx context.Context, wtc *rtesting.AdmissionWebhookTestCase, c reconcilers.Config) (*admission.Webhook, error) {
		return (&reconcilers.AdmissionWebhookAdapter[*resources.TestResource]{
			Type:       &resources.TestResource{},
			Reconciler: wtc.Metadata["SubReconciler"].(func(*testing.T, reconcilers.Config) reconcilers.SubReconciler[*resources.TestResource])(t, c),
			Config:     c,
		}).BuildWithContext(ctx)
	})
}

func TestAdmissionWebhookAdapter_Validate(t *testing.T) {
	tests := []struct {
		name           string
		webhook        *reconcilers.AdmissionWebhookAdapter[*resources.TestResource]
		validateNested bool
		shouldErr      string
	}{
		{
			name: "valid",
			webhook: &reconcilers.AdmissionWebhookAdapter[*resources.TestResource]{
				Reconciler: reconcilers.Sequence[*resources.TestResource]{},
			},
		},
		{
			name: "missing reconciler",
			webhook: &reconcilers.AdmissionWebhookAdapter[*resources.TestResource]{
				Name: "missing reconciler",
			},
			shouldErr: `AdmissionWebhookAdapter "missing reconciler" must define Reconciler`,
		},
		{
			name: "valid reconciler",
			webhook: &reconcilers.AdmissionWebhookAdapter[*resources.TestResource]{
				Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
					Sync: func(ctx context.Context, resource *resources.TestResource) error {
						return nil
					},
				},
			},
			validateNested: true,
		},
		{
			name: "invalid reconciler",
			webhook: &reconcilers.AdmissionWebhookAdapter[*resources.TestResource]{
				Reconciler: &reconcilers.SyncReconciler[*resources.TestResource]{
					// Sync: func(ctx context.Context, resource *resources.TestResource) error {
					// 	return nil
					// },
				},
			},
			validateNested: true,
			shouldErr:      `AdmissionWebhookAdapter "TestResourceAdmissionWebhookAdapter" must have a valid Reconciler: SyncReconciler "SyncReconciler" must implement Sync or SyncWithResult`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.TODO()
			if c.validateNested {
				ctx = validation.WithRecursive(ctx)
			}
			err := c.webhook.Validate(ctx)
			if (err != nil) != (c.shouldErr != "") || (c.shouldErr != "" && c.shouldErr != err.Error()) {
				t.Errorf("validate() error = %q, shouldErr %q", err, c.shouldErr)
			}
		})
	}
}
