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
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"reconciler.io/runtime/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	_ admission.Defaulter[*TestResource] = &TestResource{}
	_ admission.Validator[*TestResource] = &TestResource{}
	_ client.Object                      = &TestResource{}
)

// +kubebuilder:object:root=true
// +genclient

type TestResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestResourceSpec   `json:"spec"`
	Status TestResourceStatus `json:"status"`
}

func (*TestResource) Default(ctx context.Context, obj *TestResource) error {
	if obj.Spec.Fields == nil {
		obj.Spec.Fields = map[string]string{}
	}
	obj.Spec.Fields["Defaulter"] = "ran"

	return nil
}

func (*TestResource) ValidateCreate(ctx context.Context, obj *TestResource) (admission.Warnings, error) {
	return nil, obj.validate(ctx).ToAggregate()
}

func (*TestResource) ValidateUpdate(ctx context.Context, oldObj, newObj *TestResource) (admission.Warnings, error) {
	return nil, newObj.validate(ctx).ToAggregate()
}

func (*TestResource) ValidateDelete(ctx context.Context, obj *TestResource) (admission.Warnings, error) {
	return nil, nil
}

func (r *TestResource) validate(ctx context.Context) field.ErrorList {
	errs := field.ErrorList{}

	if r.Spec.Fields != nil {
		if _, ok := r.Spec.Fields["invalid"]; ok {
			field.Invalid(field.NewPath("spec", "fields", "invalid"), r.Spec.Fields["invalid"], "")
		}
	}

	return errs
}

// +kubebuilder:object:generate=true
type TestResourceSpec struct {
	Fields   map[string]string      `json:"fields,omitempty"`
	Template corev1.PodTemplateSpec `json:"template,omitempty"`

	ErrOnMarshal   bool `json:"errOnMarhsal,omitempty"`
	ErrOnUnmarshal bool `json:"errOnUnmarhsal,omitempty"`
}

func (r *TestResourceSpec) MarshalJSON() ([]byte, error) {
	if r.ErrOnMarshal {
		return nil, fmt.Errorf("ErrOnMarshal true")
	}
	return json.Marshal(&struct {
		Fields         map[string]string      `json:"fields,omitempty"`
		Template       corev1.PodTemplateSpec `json:"template,omitempty"`
		ErrOnMarshal   bool                   `json:"errOnMarshal,omitempty"`
		ErrOnUnmarshal bool                   `json:"errOnUnmarshal,omitempty"`
	}{
		Fields:         r.Fields,
		Template:       r.Template,
		ErrOnMarshal:   r.ErrOnMarshal,
		ErrOnUnmarshal: r.ErrOnUnmarshal,
	})
}

func (r *TestResourceSpec) UnmarshalJSON(data []byte) error {
	type alias struct {
		Fields         map[string]string      `json:"fields,omitempty"`
		Template       corev1.PodTemplateSpec `json:"template,omitempty"`
		ErrOnMarshal   bool                   `json:"errOnMarshal,omitempty"`
		ErrOnUnmarshal bool                   `json:"errOnUnmarshal,omitempty"`
	}
	a := &alias{}
	if err := json.Unmarshal(data, a); err != nil {
		return err
	}
	r.Fields = a.Fields
	r.Template = a.Template
	r.ErrOnMarshal = a.ErrOnMarshal
	r.ErrOnUnmarshal = a.ErrOnUnmarshal
	if r.ErrOnUnmarshal {
		return fmt.Errorf("ErrOnUnmarshal true")
	}
	return nil
}

// +kubebuilder:object:generate=true
type TestResourceStatus struct {
	apis.Status `json:",inline"`
	Fields      map[string]string `json:"fields,omitempty"`
}

var condSet = apis.NewLivingConditionSet()

func (rs *TestResourceStatus) InitializeConditions(ctx context.Context) {
	condSet.ManageWithContext(ctx, rs).InitializeConditions()
}

func (rs *TestResourceStatus) MarkReady(ctx context.Context) {
	condSet.ManageWithContext(ctx, rs).MarkTrue(apis.ConditionReady, "Ready", "")
}

func (rs *TestResourceStatus) MarkNotReady(ctx context.Context, reason, message string, messageA ...interface{}) {
	condSet.ManageWithContext(ctx, rs).MarkFalse(apis.ConditionReady, reason, message, messageA...)
}

// +kubebuilder:object:root=true

type TestResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []TestResource `json:"items"`
}

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "testing.reconciler.runtime", Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// compatibility with k8s.io/code-generator
var SchemeGroupVersion = GroupVersion

func init() {
	SchemeBuilder.Register(&TestResource{}, &TestResourceList{})
}
