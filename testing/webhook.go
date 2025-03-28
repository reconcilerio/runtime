/*
Copyright 2022 the original author or authors.

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

package testing

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"k8s.io/apimachinery/pkg/runtime"
	"reconciler.io/runtime/reconcilers"
	rtime "reconciler.io/runtime/time"
	"reconciler.io/runtime/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AdmissionWebhookTestCase holds a single testcase of an admission webhook.
type AdmissionWebhookTestCase struct {
	// Name is a descriptive name for this test suitable as a first argument to t.Run()
	Name string
	// Focus is true if and only if only this and any other focused tests are to be executed.
	// If one or more tests are focused, the overall test suite will fail.
	Focus bool
	// Skip is true if and only if this test should be skipped.
	Skip bool
	// Metadata contains arbitrary values that are stored with the test case
	Metadata map[string]interface{}

	// inputs

	// Request is the admission request passed to the handler
	Request *admission.Request
	// HTTPRequest is the http request used to create the admission request object. If not defined, a minimal request is provided.
	HTTPRequest *http.Request
	// WithClientBuilder allows a test to modify the fake client initialization.
	WithClientBuilder func(*fake.ClientBuilder) *fake.ClientBuilder
	// WithReactors and WithWatchReactors installs each ReactionFunc into each fake clientset. ReactionFuncs intercept
	// each call to the clientset providing the ability to mutate the resource or inject an error.
	WithReactors      []ReactionFunc
	WithWatchReactors []WatchReactionFunc
	// StatusSubResourceTypes is a set of object types that support the status sub-resource. For
	// these types, the only way to modify the resource's status is update or patch the status
	// sub-resource. Patching or updating the main resource will not mutated the status field.
	// Built-in Kubernetes types (e.g. Pod, Deployment, etc) are already accounted for and do not
	// need to be listed.
	//
	// Interacting with a status sub-resource for a type not enumerated as having a status
	// sub-resource will return a not found error.
	StatusSubResourceTypes []client.Object
	// GivenObjects build the kubernetes objects which are present at the onset of reconciliation
	GivenObjects []client.Object
	// APIGivenObjects contains objects that are only available via an API reader instead of the normal cache
	APIGivenObjects []client.Object
	// GivenTracks provide a set of tracked resources to seed the tracker with
	GivenTracks []TrackRequest

	// side effects

	// ExpectTracks holds the ordered list of Track calls expected during reconciliation
	ExpectTracks []TrackRequest
	// ExpectEvents holds the ordered list of events recorded during the reconciliation
	ExpectEvents []Event
	// ExpectCreates builds the ordered list of objects expected to be created during reconciliation
	ExpectCreates []client.Object
	// ExpectUpdates builds the ordered list of objects expected to be updated during reconciliation
	ExpectUpdates []client.Object
	// ExpectPatches builds the ordered list of objects expected to be patched during reconciliation
	ExpectPatches []PatchRef
	// ExpectDeletes holds the ordered list of objects expected to be deleted during reconciliation
	ExpectDeletes []DeleteRef
	// ExpectDeleteCollections holds the ordered list of collections expected to be deleted during reconciliation
	ExpectDeleteCollections []DeleteCollectionRef
	// ExpectStatusUpdates builds the ordered list of objects whose status is updated during reconciliation
	ExpectStatusUpdates []client.Object
	// ExpectStatusPatches builds the ordered list of objects whose status is patched during reconciliation
	ExpectStatusPatches []PatchRef

	// outputs

	// ShouldPanic is true if and only if webhook is expected to panic. A panic should only be
	// used to indicate the webhook is misconfigured.
	ShouldPanic bool
	// ExpectedResponse is compared to the response returned from the webhook
	ExpectedResponse admission.Response

	// lifecycle

	// Prepare is called before the reconciler is executed. It is intended to prepare the broader
	// environment before the specific test case is executed. For example, setting mock
	// expectations, or adding values to the context.
	Prepare func(t *testing.T, ctx context.Context, tc *AdmissionWebhookTestCase) (context.Context, error)
	// CleanUp is called after the test case is finished and all defined assertions complete.
	// It is intended to clean up any state created in the Prepare step or during the test
	// execution, or to make assertions for mocks.
	CleanUp func(t *testing.T, ctx context.Context, tc *AdmissionWebhookTestCase) error
	// Now is the time the test should run as, defaults to the current time. This value can be used
	// by reconcilers via the reconcilers.RetireveNow(ctx) method.
	Now time.Time
	// Differ methods to use to compare expected and actual values. An empty string is returned for equivalent items.
	Differ Differ
}

// AdmissionWebhookTests represents a map of reconciler test cases. The map key is the name of each
// test case.  Test cases are executed in random order.
type AdmissionWebhookTests map[string]AdmissionWebhookTestCase

// Deprecated use RunWithContext instead
func (wt AdmissionWebhookTests) Run(t *testing.T, scheme *runtime.Scheme, factory AdmissionWebhookFactory) {
	t.Helper()
	wts := AdmissionWebhookTestSuite{}
	for name, wtc := range wt {
		wtc.Name = name
		wts = append(wts, wtc)
	}
	wts.Run(t, scheme, factory)
}

// Run executes the test cases.
func (wt AdmissionWebhookTests) RunWithContext(t *testing.T, scheme *runtime.Scheme, factory AdmissionWebhookFactoryWithContext) {
	t.Helper()
	wts := AdmissionWebhookTestSuite{}
	for name, wtc := range wt {
		wtc.Name = name
		wts = append(wts, wtc)
	}
	wts.RunWithContext(t, scheme, factory)
}

// AdmissionWebhookTestSuite represents a list of webhook test cases. The test cases are
// executed in order.
type AdmissionWebhookTestSuite []AdmissionWebhookTestCase

// Deprecated use RunWithContext instead
func (tc *AdmissionWebhookTestCase) Run(t *testing.T, scheme *runtime.Scheme, factory AdmissionWebhookFactory) {
	tc.RunWithContext(t, scheme, func(t *testing.T, ctx context.Context, wtc *AdmissionWebhookTestCase, c reconcilers.Config) (*admission.Webhook, error) {
		webhook := factory(t, wtc, c)
		return webhook, nil
	})
}

// Run executes the test case.
func (tc *AdmissionWebhookTestCase) RunWithContext(t *testing.T, scheme *runtime.Scheme, factory AdmissionWebhookFactoryWithContext) {
	t.Helper()
	if tc.Skip {
		t.SkipNow()
	}

	ctx := reconcilers.WithStash(context.Background())
	if tc.Now == (time.Time{}) {
		tc.Now = time.Now()
	}
	ctx = rtime.StashNow(ctx, tc.Now)
	ctx = logr.NewContext(ctx, testr.New(t))
	if deadline, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	ctx = validation.WithRecursive(ctx)

	if tc.Metadata == nil {
		tc.Metadata = map[string]interface{}{}
	}
	if tc.Differ == nil {
		tc.Differ = DefaultDiffer
	}

	if tc.Prepare != nil {
		var err error
		if ctx, err = tc.Prepare(t, ctx, tc); err != nil {
			t.Errorf("error during prepare: %s", err)
		}
	}
	if tc.CleanUp != nil {
		defer func() {
			if err := tc.CleanUp(t, ctx, tc); err != nil {
				t.Fatalf("error during clean up: %s", err)
			}
		}()
	}

	expectConfig := &ExpectConfig{
		Name:                    "default",
		Scheme:                  scheme,
		StatusSubResourceTypes:  tc.StatusSubResourceTypes,
		Differ:                  tc.Differ,
		GivenObjects:            tc.GivenObjects,
		APIGivenObjects:         tc.APIGivenObjects,
		WithClientBuilder:       tc.WithClientBuilder,
		WithReactors:            tc.WithReactors,
		WithWatchReactors:       tc.WithWatchReactors,
		GivenTracks:             tc.GivenTracks,
		ExpectTracks:            tc.ExpectTracks,
		ExpectEvents:            tc.ExpectEvents,
		ExpectCreates:           tc.ExpectCreates,
		ExpectUpdates:           tc.ExpectUpdates,
		ExpectPatches:           tc.ExpectPatches,
		ExpectDeletes:           tc.ExpectDeletes,
		ExpectDeleteCollections: tc.ExpectDeleteCollections,
		ExpectStatusUpdates:     tc.ExpectStatusUpdates,
		ExpectStatusPatches:     tc.ExpectStatusPatches,
	}

	c := expectConfig.Config()
	r, err := factory(t, ctx, tc, c)
	if err != nil {
		t.Fatalf("Webhook factory failed: %s", err)
	}

	// Run the Reconcile we're testing.
	response := func() admission.Response {
		if tc.ShouldPanic {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected Reconcile() to panic")
				}
			}()
		}

		httpRequest := tc.HTTPRequest
		if httpRequest == nil {
			// provide a minimal default
			httpRequest = &http.Request{
				URL: &url.URL{Path: "/"},
			}
		}
		request := tc.Request
		if request == nil {
			// TODO parse admission request from http request
			t.Fatal("Request field is required")
		}

		if r.WithContextFunc != nil {
			ctx = r.WithContextFunc(ctx, httpRequest)
		}

		return r.Handle(ctx, *request)
	}()

	tc.ExpectedResponse.Complete(*tc.Request)
	if diff := tc.Differ.WebhookResponse(tc.ExpectedResponse, response); diff != "" {
		t.Errorf("ExpectedResponse differs (%s, %s): %s", DiffRemovedColor.Sprint("-expected"), DiffAddedColor.Sprint("+actual"), ColorizeDiff(diff))
	}

	expectConfig.AssertExpectations(t)
}

// Deprecated use RunWithContext instead
func (ts AdmissionWebhookTestSuite) Run(t *testing.T, scheme *runtime.Scheme, factory AdmissionWebhookFactory) {
	ts.RunWithContext(t, scheme, func(t *testing.T, ctx context.Context, wtc *AdmissionWebhookTestCase, c reconcilers.Config) (*admission.Webhook, error) {
		webhook := factory(t, wtc, c)
		return webhook, nil
	})
}

// Run executes the webhook test suite.
func (ts AdmissionWebhookTestSuite) RunWithContext(t *testing.T, scheme *runtime.Scheme, factory AdmissionWebhookFactoryWithContext) {
	t.Helper()
	focused := AdmissionWebhookTestSuite{}
	for _, test := range ts {
		if test.Focus {
			focused = append(focused, test)
			break
		}
	}
	testsToExecute := ts
	if len(focused) > 0 {
		testsToExecute = focused
	}
	for _, test := range testsToExecute {
		t.Run(test.Name, func(t *testing.T) {
			t.Helper()
			test.RunWithContext(t, scheme, factory)
		})
	}
	if len(focused) > 0 {
		t.Errorf("%d tests out of %d are still focused, so the test suite fails", len(focused), len(ts))
	}
}

type AdmissionWebhookFactory func(t *testing.T, wtc *AdmissionWebhookTestCase, c reconcilers.Config) *admission.Webhook
type AdmissionWebhookFactoryWithContext func(t *testing.T, ctx context.Context, wtc *AdmissionWebhookTestCase, c reconcilers.Config) (*admission.Webhook, error)
