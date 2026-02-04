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
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	_ client.Object = &TestDuck{}
)

// +kubebuilder:object:root=true
// +genclient

type TestDuck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestDuckSpec       `json:"spec"`
	Status TestResourceStatus `json:"status"`
}

func (*TestDuck) Default(ctx context.Context, obj *TestDuck) error {
	if obj.Spec.Fields == nil {
		obj.Spec.Fields = map[string]string{}
	}
	obj.Spec.Fields["Defaulter"] = "ran"
	return nil
}

func (r *TestDuck) ValidateCreate() (admission.Warnings, error) {
	return nil, r.validate().ToAggregate()
}

func (r *TestDuck) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	return nil, r.validate().ToAggregate()
}

func (r *TestDuck) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func (r *TestDuck) validate() field.ErrorList {
	errs := field.ErrorList{}

	if r.Spec.Fields != nil {
		if _, ok := r.Spec.Fields["invalid"]; ok {
			field.Invalid(field.NewPath("spec", "fields", "invalid"), r.Spec.Fields["invalid"], "")
		}
	}

	return errs
}

// +kubebuilder:object:generate=true
type TestDuckSpec struct {
	Immutable *bool             `json:"immutable,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// +kubebuilder:object:root=true

type TestDuckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []TestDuck `json:"items"`
}
