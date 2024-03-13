/*
Copyright 2024 the original author or authors.

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

package reconcilers_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"reconciler.io/runtime/internal/resources"
	"reconciler.io/runtime/internal/resources/dies"
	"reconciler.io/runtime/reconcilers"
)

type TestResourceUnexportedSpec struct {
	spec resources.TestResourceUnexportedFieldsSpec
}

func TestIgnoreAllUnexported(t *testing.T) {
	tests := map[string]struct {
		a          interface{}
		b          interface{}
		shouldDiff bool
	}{
		"nil is equivalent": {
			a:          nil,
			b:          nil,
			shouldDiff: false,
		},
		"different exported fields have a difference": {
			a: dies.TestResourceUnexportedFieldsSpecBlank.
				AddField("name", "hello").
				DieRelease(),
			b: dies.TestResourceUnexportedFieldsSpecBlank.
				AddField("name", "world").
				DieRelease(),
			shouldDiff: true,
		},
		"different unexported fields do not have a difference": {
			a: dies.TestResourceUnexportedFieldsSpecBlank.
				AddUnexportedField("name", "hello").
				DieRelease(),
			b: dies.TestResourceUnexportedFieldsSpecBlank.
				AddUnexportedField("name", "world").
				DieRelease(),
			shouldDiff: false,
		},
		"different exported fields nested in an unexported field do not have a difference": {
			a: TestResourceUnexportedSpec{
				spec: dies.TestResourceUnexportedFieldsSpecBlank.
					AddField("name", "hello").
					DieRelease(),
			},
			b: TestResourceUnexportedSpec{
				spec: dies.TestResourceUnexportedFieldsSpecBlank.
					AddField("name", "world").
					DieRelease(),
			},
			shouldDiff: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if name[0:1] == "#" {
				t.SkipNow()
			}

			diff := cmp.Diff(tc.a, tc.b, reconcilers.IgnoreAllUnexported)
			hasDiff := diff != ""
			shouldDiff := tc.shouldDiff

			if !hasDiff && shouldDiff {
				t.Errorf("expected equality, found diff")
			}
			if hasDiff && !shouldDiff {
				t.Errorf("found diff, expected equality: %s", diff)
			}
		})
	}
}
