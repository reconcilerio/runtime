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

package resources

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var _ webhook.CustomDefaulter = &TestResourceEmptyStatus{}

// +kubebuilder:object:root=true
// +genclient

type TestResourceEmptyStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestResourceSpec              `json:"spec"`
	Status TestResourceEmptyStatusStatus `json:"status"`
}

func (*TestResourceEmptyStatus) Default(ctx context.Context, obj runtime.Object) error {
	r, ok := obj.(*TestResourceEmptyStatus)
	if !ok {
		return fmt.Errorf("expected obj to be TestResourceEmptyStatus")
	}

	if r.Spec.Fields == nil {
		r.Spec.Fields = map[string]string{}
	}
	r.Spec.Fields["Defaulter"] = "ran"

	return nil
}

// +kubebuilder:object:generate=true
type TestResourceEmptyStatusStatus struct {
}

// +kubebuilder:object:root=true

type TestResourceEmptyStatusList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []TestResourceNoStatus `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestResourceEmptyStatus{}, &TestResourceEmptyStatusList{})
}
