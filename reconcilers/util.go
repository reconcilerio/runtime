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

package reconcilers

import (
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// objectDefaulter mirrors the former upstream interface webhook.Defaulter which was deprecated and
// removed. We use this interface when reconciling a resource to set default values to a common
// baseline.
type objectDefaulter interface {
	Default()
}

// extractItems returns a typed slice of objects from an object list
func extractItems[T client.Object](list client.ObjectList) []T {
	items := []T{}
	listValue := reflect.ValueOf(list).Elem()
	itemsValue := listValue.FieldByName("Items")
	for i := 0; i < itemsValue.Len(); i++ {
		itemValue := itemsValue.Index(i)
		var item T
		switch itemValue.Kind() {
		case reflect.Pointer:
			item = itemValue.Interface().(T)
		case reflect.Interface:
			item = itemValue.Interface().(T)
		case reflect.Struct:
			item = itemValue.Addr().Interface().(T)
		default:
			panic(fmt.Errorf("unknown type %s for Items slice, expected Pointer or Struct", itemValue.Kind().String()))
		}
		items = append(items, item)
	}
	return items
}
