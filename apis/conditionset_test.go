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

package apis

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConditionStatus(t *testing.T) {
	tests := []struct {
		name            string
		condition       *metav1.Condition
		expectedTrue    bool
		expectedFalse   bool
		expectedUnknown bool
	}{
		{
			name:            "true",
			condition:       &metav1.Condition{Status: metav1.ConditionTrue},
			expectedTrue:    true,
			expectedFalse:   false,
			expectedUnknown: false,
		},
		{
			name:            "false",
			condition:       &metav1.Condition{Status: metav1.ConditionFalse},
			expectedTrue:    false,
			expectedFalse:   true,
			expectedUnknown: false,
		},
		{
			name:            "unknown",
			condition:       &metav1.Condition{Status: metav1.ConditionUnknown},
			expectedTrue:    false,
			expectedFalse:   false,
			expectedUnknown: true,
		},
		{
			name:            "unset",
			condition:       &metav1.Condition{},
			expectedTrue:    false,
			expectedFalse:   false,
			expectedUnknown: true,
		},
		{
			name:            "nil",
			condition:       nil,
			expectedTrue:    false,
			expectedFalse:   false,
			expectedUnknown: true,
		},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			if expected, actual := c.expectedTrue, ConditionIsTrue(c.condition); expected != actual {
				t.Errorf("%s: IsTrue() actually = %v, expected %v", c.name, actual, expected)
			}
			if expected, actual := c.expectedFalse, ConditionIsFalse(c.condition); expected != actual {
				t.Errorf("%s: IsFalse() actually = %v, expected %v", c.name, actual, expected)
			}
			if expected, actual := c.expectedUnknown, ConditionIsUnknown(c.condition); expected != actual {
				t.Errorf("%s: IsUnknown() actually = %v, expected %v", c.name, actual, expected)
			}
		})
	}
}
