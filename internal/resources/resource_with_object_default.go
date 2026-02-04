/*
Copyright 2019 the original author or authors.

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

package resources

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	_ admission.Defaulter[runtime.Object] = &TestResourceWithObjectDefault{}
	_ admission.Validator[runtime.Object] = &TestResourceWithObjectDefault{}
	_ client.Object                       = &TestResourceWithObjectDefault{}
)

// +kubebuilder:object:root=true
// +genclient

type TestResourceWithObjectDefault struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestResourceSpec   `json:"spec"`
	Status TestResourceStatus `json:"status"`
}

func (*TestResourceWithObjectDefault) Default(ctx context.Context, obj runtime.Object) error {
	r, ok := obj.(*TestResourceWithObjectDefault)
	if !ok {
		return fmt.Errorf("expected object to be TestResourceWithObjectDefault")
	}

	if r.Spec.Fields == nil {
		r.Spec.Fields = map[string]string{}
	}
	r.Spec.Fields["Defaulter"] = "ran"

	return nil
}

func (*TestResourceWithObjectDefault) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	r, ok := obj.(*TestResourceWithObjectDefault)
	if !ok {
		return nil, fmt.Errorf("expected obj to be TestResourceWithObjectDefault")
	}

	return nil, r.validate(ctx).ToAggregate()
}

func (*TestResourceWithObjectDefault) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	_, ok := oldObj.(*TestResourceWithObjectDefault)
	if !ok {
		return nil, fmt.Errorf("expected oldObj to be TestResourceWithObjectDefault")
	}
	r, ok := newObj.(*TestResourceWithObjectDefault)
	if !ok {
		return nil, fmt.Errorf("expected newObj to be TestResourceWithObjectDefault")
	}

	return nil, r.validate(ctx).ToAggregate()
}

func (*TestResourceWithObjectDefault) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, ok := obj.(*TestResourceWithObjectDefault)
	if !ok {
		return nil, fmt.Errorf("expected obj to be TestResourceWithObjectDefault")
	}

	return nil, nil
}

func (r *TestResourceWithObjectDefault) validate(ctx context.Context) field.ErrorList {
	errs := field.ErrorList{}

	if r.Spec.Fields != nil {
		if _, ok := r.Spec.Fields["invalid"]; ok {
			field.Invalid(field.NewPath("spec", "fields", "invalid"), r.Spec.Fields["invalid"], "")
		}
	}

	return errs
}

// +kubebuilder:object:root=true

type TestResourceWithObjectDefaultList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []TestResourceWithObjectDefault `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestResourceWithObjectDefault{}, &TestResourceWithObjectDefaultList{})
}
