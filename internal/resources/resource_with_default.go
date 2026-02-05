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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"reconciler.io/runtime/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	_ validation.Defaulter                          = &TestResourceWithDefault{}
	_ admission.Validator[*TestResourceWithDefault] = &TestResourceWithDefault{}
	_ client.Object                                 = &TestResourceWithDefault{}
)

// +kubebuilder:object:root=true
// +genclient

type TestResourceWithDefault struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestResourceSpec   `json:"spec"`
	Status TestResourceStatus `json:"status"`
}

func (r *TestResourceWithDefault) Default(ctx context.Context) error {
	if r.Spec.Fields == nil {
		r.Spec.Fields = map[string]string{}
	}
	r.Spec.Fields["Defaulter"] = "ran"

	return nil
}

func (*TestResourceWithDefault) ValidateCreate(ctx context.Context, obj *TestResourceWithDefault) (admission.Warnings, error) {
	return nil, obj.validate(ctx).ToAggregate()
}

func (*TestResourceWithDefault) ValidateUpdate(ctx context.Context, oldObj, newObj *TestResourceWithDefault) (admission.Warnings, error) {
	return nil, newObj.validate(ctx).ToAggregate()
}

func (*TestResourceWithDefault) ValidateDelete(ctx context.Context, obj *TestResourceWithDefault) (admission.Warnings, error) {
	return nil, nil
}

func (r *TestResourceWithDefault) validate(ctx context.Context) field.ErrorList {
	errs := field.ErrorList{}

	if r.Spec.Fields != nil {
		if _, ok := r.Spec.Fields["invalid"]; ok {
			field.Invalid(field.NewPath("spec", "fields", "invalid"), r.Spec.Fields["invalid"], "")
		}
	}

	return errs
}

// +kubebuilder:object:root=true

type TestResourceWithDefaultList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []TestResourceWithDefault `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestResourceWithDefault{}, &TestResourceWithDefaultList{})
}
