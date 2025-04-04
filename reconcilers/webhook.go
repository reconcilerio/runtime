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

package reconcilers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"gomodules.xyz/jsonpatch/v3"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reconciler.io/runtime/internal"
	rtime "reconciler.io/runtime/time"
	"reconciler.io/runtime/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AdmissionWebhookAdapter allows using sub reconcilers to process admission webhooks. The full
// suite of sub reconcilers are available, however, behavior that is generally not accepted within
// a webhook is discouraged. For example, new requests against the API server are discouraged
// (reading from an informer is ok), mutation requests against the API Server can cause a loop with
// the webhook processing its own requests.
//
// All requests are allowed by default unless the response.Allowed field is explicitly unset, or
// the reconciler returns an error. The raw admission request and response can be retrieved from
// the context via the RetrieveAdmissionRequest and RetrieveAdmissionResponse methods,
// respectively. The Result typically returned by a reconciler is unused.
//
// The request object is unmarshaled from the request object for most operations, and the old
// object for delete operations. If the webhhook handles multiple resources or versions of the
// same resource with different shapes, use of an unstructured type is recommended.
//
// If the resource being reconciled is mutated and the response does not already define a patch, a
// json patch is computed for the mutation and set on the response.
//
// If the webhook can handle multiple types, use *unstructured.Unstructured as the generic type.
type AdmissionWebhookAdapter[Type client.Object] struct {
	// Name used to identify this reconciler.  Defaults to `{Type}AdmissionWebhookAdapter`.  Ideally
	// unique, but not required to be so.
	//
	// +optional
	Name string

	// Type of resource to reconcile. Required when the generic type is not a struct.
	//
	// +optional
	Type Type

	// Reconciler is called for each reconciler request with the resource being reconciled.
	// Typically, Reconciler is a Sequence of multiple SubReconcilers.
	Reconciler SubReconciler[Type]

	// BeforeHandle is called first thing for each admission request.  A modified context may be
	// returned.
	//
	// If BeforeHandle is not defined, there is no effect.
	//
	// +optional
	BeforeHandle func(ctx context.Context, req admission.Request, resp *admission.Response) context.Context

	// AfterHandle is called following all work for the admission request. The response is provided
	// and may be modified before returning.
	//
	// If AfterHandle is not defined, the response is returned directly.
	//
	// +optional
	AfterHandle func(ctx context.Context, req admission.Request, resp *admission.Response)

	Config Config

	lazyInit sync.Once
}

func (r *AdmissionWebhookAdapter[T]) init() {
	r.lazyInit.Do(func() {
		if internal.IsNil(r.Type) {
			var nilT T
			r.Type = newEmpty(nilT).(T)
		}
		if r.Name == "" {
			r.Name = fmt.Sprintf("%sAdmissionWebhookAdapter", typeName(r.Type))
		}
		if r.BeforeHandle == nil {
			r.BeforeHandle = func(ctx context.Context, req admission.Request, resp *admission.Response) context.Context {
				return ctx
			}
		}
		if r.AfterHandle == nil {
			r.AfterHandle = func(ctx context.Context, req admission.Request, resp *admission.Response) {}
		}
	})
}

func (r *AdmissionWebhookAdapter[T]) Validate(ctx context.Context) error {
	r.init()

	// validate Reconciler value
	if r.Reconciler == nil {
		return fmt.Errorf("AdmissionWebhookAdapter %q must define Reconciler", r.Name)
	}
	if validation.IsRecursive(ctx) {
		if v, ok := r.Reconciler.(validation.Validator); ok {
			if err := v.Validate(ctx); err != nil {
				return fmt.Errorf("AdmissionWebhookAdapter %q must have a valid Reconciler: %w", r.Name, err)
			}
		}
	}

	return nil
}

// Deprecated use BuildWithContext
func (r *AdmissionWebhookAdapter[T]) Build() *admission.Webhook {
	webhook, err := r.BuildWithContext(context.TODO())
	if err != nil {
		panic(err)
	}
	return webhook
}

func (r *AdmissionWebhookAdapter[T]) BuildWithContext(ctx context.Context) (*admission.Webhook, error) {
	r.init()

	if err := r.Validate(r.withContext(ctx)); err != nil {
		return nil, err
	}

	return &admission.Webhook{
		Handler: r,
		WithContextFunc: func(ctx context.Context, req *http.Request) context.Context {
			ctx = r.withContext(ctx)

			log := crlog.FromContext(ctx).
				WithValues(
					"webhook", req.URL.Path,
				)
			ctx = logr.NewContext(ctx, log)

			ctx = StashHTTPRequest(ctx, req)

			return ctx
		},
	}, nil
}

func (r *AdmissionWebhookAdapter[T]) withContext(ctx context.Context) context.Context {
	log := crlog.FromContext(ctx).
		WithName("controller-runtime.webhook.webhooks").
		WithName(r.Name)
	ctx = logr.NewContext(ctx, log)

	ctx = WithStash(ctx)

	ctx = StashConfig(ctx, r.Config)
	ctx = StashOriginalConfig(ctx, r.Config)
	ctx = StashResourceType(ctx, r.Type)
	ctx = StashOriginalResourceType(ctx, r.Type)

	return ctx
}

// Handle implements admission.Handler
func (r *AdmissionWebhookAdapter[T]) Handle(ctx context.Context, req admission.Request) admission.Response {
	r.init()

	log := logr.FromContextOrDiscard(ctx).
		WithValues(
			"UID", req.UID,
			"kind", req.Kind,
			"resource", req.Resource,
			"operation", req.Operation,
		)
	ctx = logr.NewContext(ctx, log)

	resp := &admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			UID: req.UID,
			// allow by default, a reconciler can flip this off, or return an err
			Allowed: true,
		},
	}

	ctx = rtime.StashNow(ctx, time.Now())
	ctx = StashAdmissionRequest(ctx, req)
	ctx = StashAdmissionResponse(ctx, resp)

	if beforeCtx := r.BeforeHandle(ctx, req, resp); beforeCtx != nil {
		ctx = beforeCtx
	}

	// defined for compatibility since this is not a reconciler
	ctx = StashRequest(ctx, Request{
		NamespacedName: types.NamespacedName{
			Namespace: req.Namespace,
			Name:      req.Name,
		},
	})

	if err := r.reconcile(ctx, req, resp); err != nil {
		if !errors.Is(err, ErrQuiet) {
			log.Error(err, "reconcile error")
		}
		resp.Allowed = false
		if resp.Result == nil {
			resp.Result = &metav1.Status{
				Code:    500,
				Message: err.Error(),
			}
		}
	}

	r.AfterHandle(ctx, req, resp)
	return *resp
}

func (r *AdmissionWebhookAdapter[T]) reconcile(ctx context.Context, req admission.Request, resp *admission.Response) error {
	log := logr.FromContextOrDiscard(ctx)

	resource := r.Type.DeepCopyObject().(T)
	resourceBytes := req.Object.Raw
	if req.Operation == admissionv1.Delete {
		resourceBytes = req.OldObject.Raw
	}
	if err := json.Unmarshal(resourceBytes, resource); err != nil {
		return err
	}

	if defaulter, ok := client.Object(resource).(webhook.CustomDefaulter); ok {
		// resource.Default(ctx, resource)
		if err := defaulter.Default(ctx, resource); err != nil {
			return err
		}
	} else if defaulter, ok := client.Object(resource).(objectDefaulter); ok {
		// resource.Default()
		defaulter.Default()
	}

	originalResource := resource.DeepCopyObject()
	if _, err := r.Reconciler.Reconcile(ctx, resource); err != nil {
		return err
	}

	if resp.Patches == nil && resp.Patch == nil && resp.PatchType == nil && !equality.Semantic.DeepEqual(originalResource, resource) {
		// add patch to response

		mutationBytes, err := json.Marshal(resource)
		if err != nil {
			return err
		}

		// create patch using jsonpatch v3 since it preserves order, then convert back to v2 used by AdmissionResponse
		patch, err := jsonpatch.CreatePatch(resourceBytes, mutationBytes)
		if err != nil {
			return err
		}
		data, _ := json.Marshal(patch)
		_ = json.Unmarshal(data, &resp.Patches)
		log.Info("mutating resource", "patch", resp.Patches)
	}

	return nil
}

const (
	admissionRequestStashKey  StashKey = "reconciler.io/runtime:admission-request"
	admissionResponseStashKey StashKey = "reconciler.io/runtime:admission-response"
	httpRequestStashKey       StashKey = "reconciler.io/runtime:http-request"
)

func StashAdmissionRequest(ctx context.Context, req admission.Request) context.Context {
	return context.WithValue(ctx, admissionRequestStashKey, req)
}

// RetrieveAdmissionRequest returns the admission Request from the context, or empty if not found.
func RetrieveAdmissionRequest(ctx context.Context) admission.Request {
	value := ctx.Value(admissionRequestStashKey)
	if req, ok := value.(admission.Request); ok {
		return req
	}
	return admission.Request{}
}

func StashAdmissionResponse(ctx context.Context, resp *admission.Response) context.Context {
	return context.WithValue(ctx, admissionResponseStashKey, resp)
}

// RetrieveAdmissionResponse returns the admission Response from the context, or nil if not found.
func RetrieveAdmissionResponse(ctx context.Context) *admission.Response {
	value := ctx.Value(admissionResponseStashKey)
	if resp, ok := value.(*admission.Response); ok {
		return resp
	}
	return nil
}

func StashHTTPRequest(ctx context.Context, req *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestStashKey, req)
}

// RetrieveHTTPRequest returns the http Request from the context, or nil if not found.
func RetrieveHTTPRequest(ctx context.Context) *http.Request {
	value := ctx.Value(httpRequestStashKey)
	if req, ok := value.(*http.Request); ok {
		return req
	}
	return nil
}
