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

package stash

import (
	"context"
	"errors"
	"fmt"
)

const stashNonce string = "controller-stash-nonce"

type stashMap map[Key]interface{}

func WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, stashNonce, stashMap{})
}

type Key string

func retrieveStashMap(ctx context.Context) stashMap {
	stash, ok := ctx.Value(stashNonce).(stashMap)
	if !ok {
		panic(fmt.Errorf("context not configured for stashing, call `ctx = WithContext(ctx)` before use"))
	}
	return stash
}

func StoreValue(ctx context.Context, key Key, value interface{}) {
	stash := retrieveStashMap(ctx)
	stash[key] = value
}

func HasValue(ctx context.Context, key Key) bool {
	stash := retrieveStashMap(ctx)
	_, ok := stash[key]
	return ok
}

func RetrieveValue(ctx context.Context, key Key) interface{} {
	stash := retrieveStashMap(ctx)
	return stash[key]
}

func ClearValue(ctx context.Context, key Key) interface{} {
	stash := retrieveStashMap(ctx)
	value := stash[key]
	delete(stash, key)
	return value
}

// Stasher stores and retrieves values from the stash context. The context which gets passed to its methods must be configured
// with a stash via WithStash(). The stash is pre-configured for the context within a reconciler.
type Stasher[T any] interface {
	// Key is the stash key used to store and retrieve the value
	Key() Key

	// Store saves the value in the stash under the key
	Store(ctx context.Context, value T)

	// Clear removes the key from the stash returning the previous value, if any.
	Clear(ctx context.Context) T

	// Has returns true when the stash contains the key. The type of the value is not checked.
	Has(ctx context.Context) bool

	// RetrieveOrDie retrieves the value from the stash, or panics if the key is not in the stash
	RetrieveOrDie(ctx context.Context) T

	// RetrieveOrEmpty retrieves the value from the stash, or an error if the key is not in the stash
	RetrieveOrError(ctx context.Context) (T, error)

	// RetrieveOrEmpty retrieves the value from the stash, or the empty value if the key is not in the stash
	RetrieveOrEmpty(ctx context.Context) T
}

// New creates a stasher for the value type
func New[T any](key Key) Stasher[T] {
	return &stasher[T]{
		key: key,
	}
}

type stasher[T any] struct {
	key Key
}

func (s *stasher[T]) Key() Key {
	return s.key
}

func (s *stasher[T]) Store(ctx context.Context, value T) {
	StoreValue(ctx, s.Key(), value)
}

func (s *stasher[T]) Clear(ctx context.Context) T {
	previous, _ := ClearValue(ctx, s.Key()).(T)
	return previous
}

func (s *stasher[T]) Has(ctx context.Context) bool {
	return HasValue(ctx, s.Key())
}

func (s *stasher[T]) RetrieveOrDie(ctx context.Context) T {
	value, err := s.RetrieveOrError(ctx)
	if err != nil {
		panic(err)
	}
	return value
}

var ErrValueNotFound = errors.New("value not found in stash")
var ErrValueNotAssignable = errors.New("value found in stash is not of an assignable type")

func (s *stasher[T]) RetrieveOrError(ctx context.Context) (T, error) {
	var emptyT T
	value := RetrieveValue(ctx, s.Key())
	if value == nil {
		// distinguish nil and missing values in stash
		if !s.Has(ctx) {
			return emptyT, ErrValueNotFound
		}
		return emptyT, nil
	}
	typedValue, ok := value.(T)
	if !ok {
		return emptyT, ErrValueNotAssignable
	}
	return typedValue, nil
}

func (s *stasher[T]) RetrieveOrEmpty(ctx context.Context) T {
	value, _ := s.RetrieveOrError(ctx)
	return value
}
