//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2019-2024 the original author or authors.

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

// Code generated by diegen. DO NOT EDIT.

package dies

import (
	testing "reconciler.io/dies/testing"
	testingx "testing"
)

func TestTestResourceDie_MissingMethods(t *testingx.T) {
	die := TestResourceBlank
	ignore := []string{"TypeMeta", "ObjectMeta"}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceDie: %s", diff.List())
	}
}

func TestTestResourceSpecDie_MissingMethods(t *testingx.T) {
	die := TestResourceSpecBlank
	ignore := []string{}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceSpecDie: %s", diff.List())
	}
}

func TestTestResourceStatusDie_MissingMethods(t *testingx.T) {
	die := TestResourceStatusBlank
	ignore := []string{}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceStatusDie: %s", diff.List())
	}
}

func TestTestResourceEmptyStatusDie_MissingMethods(t *testingx.T) {
	die := TestResourceEmptyStatusBlank
	ignore := []string{"TypeMeta", "ObjectMeta"}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceEmptyStatusDie: %s", diff.List())
	}
}

func TestTestResourceEmptyStatusStatusDie_MissingMethods(t *testingx.T) {
	die := TestResourceEmptyStatusStatusBlank
	ignore := []string{}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceEmptyStatusStatusDie: %s", diff.List())
	}
}

func TestTestResourceNoStatusDie_MissingMethods(t *testingx.T) {
	die := TestResourceNoStatusBlank
	ignore := []string{"TypeMeta", "ObjectMeta"}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceNoStatusDie: %s", diff.List())
	}
}

func TestTestResourceNilableStatusDie_MissingMethods(t *testingx.T) {
	die := TestResourceNilableStatusBlank
	ignore := []string{"TypeMeta", "ObjectMeta"}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceNilableStatusDie: %s", diff.List())
	}
}

func TestTestDuckDie_MissingMethods(t *testingx.T) {
	die := TestDuckBlank
	ignore := []string{"TypeMeta", "ObjectMeta"}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestDuckDie: %s", diff.List())
	}
}

func TestTestDuckSpecDie_MissingMethods(t *testingx.T) {
	die := TestDuckSpecBlank
	ignore := []string{}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestDuckSpecDie: %s", diff.List())
	}
}

func TestTestResourceUnexportedFieldsDie_MissingMethods(t *testingx.T) {
	die := TestResourceUnexportedFieldsBlank
	ignore := []string{"TypeMeta", "ObjectMeta"}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceUnexportedFieldsDie: %s", diff.List())
	}
}

func TestTestResourceUnexportedFieldsSpecDie_MissingMethods(t *testingx.T) {
	die := TestResourceUnexportedFieldsSpecBlank
	ignore := []string{"unexportedFields"}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceUnexportedFieldsSpecDie: %s", diff.List())
	}
}

func TestTestResourceUnexportedFieldsStatusDie_MissingMethods(t *testingx.T) {
	die := TestResourceUnexportedFieldsStatusBlank
	ignore := []string{"unexportedFields"}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceUnexportedFieldsStatusDie: %s", diff.List())
	}
}

func TestTestResourceWithLegacyDefaultDie_MissingMethods(t *testingx.T) {
	die := TestResourceWithLegacyDefaultBlank
	ignore := []string{"TypeMeta", "ObjectMeta"}
	diff := testing.DieFieldDiff(die).Delete(ignore...)
	if diff.Len() != 0 {
		t.Errorf("found missing fields for TestResourceWithLegacyDefaultDie: %s", diff.List())
	}
}
