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
	"reconciler.io/runtime/stash"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Builder = builder.Builder
type Manager = manager.Manager
type Request = reconcile.Request
type Result = reconcile.Result

var WithStash = stash.WithContext
var StashValue = stash.StoreValue
var RetrieveValue = stash.RetrieveValue
var HasValue = stash.HasValue
var ClearValue = stash.ClearValue
var ErrStashValueNotFound = stash.ErrValueNotFound
var ErrStashValueNotAssignable = stash.ErrValueNotAssignable

type StashKey = stash.Key
type Stasher[T any] = stash.Stasher[T]

// NewStasher creates a stasher for the value type
func NewStasher[T any](key stash.Key) stash.Stasher[T] {
	// TODO switch to an alias once generic functions can be aliased
	return stash.New[T](key)
}
