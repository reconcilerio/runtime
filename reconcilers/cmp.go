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
	"unicode"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

// IgnoreAllUnexported is a cmp.Option that ignores unexported fields in all structs
var IgnoreAllUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	// from cmp.IgnoreUnexported with type info removed
	sf, ok := p.Index(-1).(cmp.StructField)
	if !ok {
		return false
	}
	r, _ := utf8.DecodeRuneInString(sf.Name())
	return !unicode.IsUpper(r)
}, cmp.Ignore())
