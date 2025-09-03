/*
Copyright 2025 the original author or authors.

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
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgotesting "k8s.io/client-go/testing"
)

type Action = clientgotesting.Action
type GetAction = clientgotesting.GetAction
type ListAction = clientgotesting.ListAction
type CreateAction = clientgotesting.CreateAction
type UpdateAction = clientgotesting.UpdateAction
type PatchAction = clientgotesting.PatchAction
type DeleteAction = clientgotesting.DeleteAction
type DeleteCollectionAction = clientgotesting.DeleteCollectionAction

type ApplyAction interface {
	Action
	GetName() string
	GetApplyConfiguration() runtime.ApplyConfiguration
}

type ApplyActionImpl struct {
	clientgotesting.ActionImpl
	Name               string
	ApplyConfiguration runtime.ApplyConfiguration
}

func (a ApplyActionImpl) GetName() string {
	return a.Name
}

func (a ApplyActionImpl) GetApplyConfiguration() runtime.ApplyConfiguration {
	return a.ApplyConfiguration
}

func NewApplyAction(ac runtime.ApplyConfiguration) ApplyAction {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ac)
	if err != nil {
		panic(fmt.Errorf("unable to convert from apply configuration: %w", err))
	}
	obj := unstructured.Unstructured{
		Object: u,
	}
	gvk := obj.GetObjectKind().GroupVersionKind()

	action := ApplyActionImpl{}

	action.Verb = "apply"
	action.Resource.Group = gvk.Group
	action.Resource.Version = gvk.Version
	// gvk != gvr, following established practice
	action.Resource.Resource = gvk.Kind
	action.Namespace = obj.GetNamespace()
	action.Name = obj.GetName()
	action.ApplyConfiguration = ac

	return action
}
