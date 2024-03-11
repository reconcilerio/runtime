/*
Copyright 2020 the original author or authors.

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
	"testing"

	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/api/equality"
)

func TestStash(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{
			name:  "string",
			value: "value",
		},
		{
			name:  "int",
			value: 42,
		},
		{
			name:  "map",
			value: map[string]string{"foo": "bar"},
		},
		{
			name:  "nil",
			value: nil,
		},
	}

	var key StashKey = "stash-key"
	ctx := WithStash(context.TODO())
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			StashValue(ctx, key, c.value)
			if expected, actual := c.value, RetrieveValue(ctx, key); !equality.Semantic.DeepEqual(expected, actual) {
				t.Errorf("%s: unexpected stash value, actually = %v, expected = %v", c.name, actual, expected)
			}
		})
	}
}

func TestStash_StashValue_UndecoratedContext(t *testing.T) {
	ctx := context.TODO()
	var key StashKey = "stash-key"
	value := "value"

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected StashValue() to panic")
		}
	}()
	StashValue(ctx, key, value)
}

func TestStash_RetrieveValue_UndecoratedContext(t *testing.T) {
	ctx := context.TODO()
	var key StashKey = "stash-key"

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected RetrieveValue() to panic")
		}
	}()
	RetrieveValue(ctx, key)
}

func TestStash_RetrieveValue_Undefined(t *testing.T) {
	ctx := WithStash(context.TODO())
	var key StashKey = "stash-key"

	if value := RetrieveValue(ctx, key); value != nil {
		t.Error("expected RetrieveValue() to return nil for undefined key")
	}
}
