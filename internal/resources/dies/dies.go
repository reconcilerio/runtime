/*
Copyright 2021 the original author or authors.

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

package dies

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	diemetav1 "reconciler.io/dies/apis/meta/v1"
	"reconciler.io/runtime/internal/resources"
)

// +die:object=true
type _ = resources.TestResource

// +die
// +die:field:name=Template,package=_/core/v1,die=PodTemplateSpecDie
type _ = resources.TestResourceSpec

func (d *TestResourceSpecDie) AddField(key, value string) *TestResourceSpecDie {
	return d.DieStamp(func(r *resources.TestResourceSpec) {
		if r.Fields == nil {
			r.Fields = map[string]string{}
		}
		r.Fields[key] = value
	})
}

// +die
type _ = resources.TestResourceStatus

func (d *TestResourceStatusDie) ConditionsDie(conditions ...*diemetav1.ConditionDie) *TestResourceStatusDie {
	return d.DieStamp(func(r *resources.TestResourceStatus) {
		r.Conditions = make([]metav1.Condition, len(conditions))
		for i := range conditions {
			r.Conditions[i] = conditions[i].DieRelease()
		}
	})
}

func (d *TestResourceStatusDie) AddField(key, value string) *TestResourceStatusDie {
	return d.DieStamp(func(r *resources.TestResourceStatus) {
		if r.Fields == nil {
			r.Fields = map[string]string{}
		}
		r.Fields[key] = value
	})
}

// +die:object=true,spec=TestResourceSpec
type _ = resources.TestResourceEmptyStatus

// +die
type _ = resources.TestResourceEmptyStatusStatus

// +die:object=true,spec=TestResourceSpec
type _ = resources.TestResourceNoStatus

// +die:object=true,spec=TestResourceSpec
// +die:field:name=Status,die=TestResourceStatusDie,pointer=true
type _ = resources.TestResourceNilableStatus

// +die:object=true
// +die:field:name=Status,die=TestResourceStatusDie
type _ = resources.TestDuck

// +die
type _ = resources.TestDuckSpec

func (d *TestDuckSpecDie) AddField(key, value string) *TestDuckSpecDie {
	return d.DieStamp(func(r *resources.TestDuckSpec) {
		if r.Fields == nil {
			r.Fields = map[string]string{}
		}
		r.Fields[key] = value
	})
}

// +die:object=true
type _ = resources.TestResourceUnexportedFields

// +die:ignore={unexportedFields}
// +die:field:name=Template,package=_/core/v1,die=PodTemplateSpecDie
type _ = resources.TestResourceUnexportedFieldsSpec

func (d *TestResourceUnexportedFieldsSpecDie) AddField(key, value string) *TestResourceUnexportedFieldsSpecDie {
	return d.DieStamp(func(r *resources.TestResourceUnexportedFieldsSpec) {
		if r.Fields == nil {
			r.Fields = map[string]string{}
		}
		r.Fields[key] = value
	})
}

func (d *TestResourceUnexportedFieldsSpecDie) AddUnexportedField(key, value string) *TestResourceUnexportedFieldsSpecDie {
	return d.DieStamp(func(r *resources.TestResourceUnexportedFieldsSpec) {
		f := r.GetUnexportedFields()
		if f == nil {
			f = map[string]string{}
		}
		f[key] = value
		r.SetUnexportedFields(f)
	})
}

// +die:ignore={unexportedFields}
type _ = resources.TestResourceUnexportedFieldsStatus

func (d *TestResourceUnexportedFieldsStatusDie) ConditionsDie(conditions ...*diemetav1.ConditionDie) *TestResourceUnexportedFieldsStatusDie {
	return d.DieStamp(func(r *resources.TestResourceUnexportedFieldsStatus) {
		r.Conditions = make([]metav1.Condition, len(conditions))
		for i := range conditions {
			r.Conditions[i] = conditions[i].DieRelease()
		}
	})
}

func (d *TestResourceUnexportedFieldsStatusDie) AddField(key, value string) *TestResourceUnexportedFieldsStatusDie {
	return d.DieStamp(func(r *resources.TestResourceUnexportedFieldsStatus) {
		if r.Fields == nil {
			r.Fields = map[string]string{}
		}
		r.Fields[key] = value
	})
}

func (d *TestResourceUnexportedFieldsStatusDie) AddUnexportedField(key, value string) *TestResourceUnexportedFieldsStatusDie {
	return d.DieStamp(func(r *resources.TestResourceUnexportedFieldsStatus) {
		f := r.GetUnexportedFields()
		if f == nil {
			f = map[string]string{}
		}
		f[key] = value
		r.SetUnexportedFields(f)
	})
}

// +die:object=true,spec=TestResourceSpec,status=TestResourceStatus
type _ = resources.TestResourceWithDefault

// +die:object=true,spec=TestResourceSpec,status=TestResourceStatus
type _ = resources.TestResourceWithLegacyDefault

// +die:object=true,spec=TestResourceSpec,status=TestResourceStatus
type _ = resources.TestResourceWithObjectDefault
