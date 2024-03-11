/*
Copyright 2023 the original author or authors.

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

package time

import (
	"context"
	"time"
)

type stashKey struct{}

var nowStashKey = stashKey{}

func StashNow(ctx context.Context, now time.Time) context.Context {
	if ctx.Value(nowStashKey) != nil {
		// avoid overwriting
		return ctx
	}
	return context.WithValue(ctx, nowStashKey, &now)
}

// RetrieveNow returns the stashed time, or the current time if not found.
func RetrieveNow(ctx context.Context) time.Time {
	value := ctx.Value(nowStashKey)
	if now, ok := value.(*time.Time); ok {
		return *now
	}
	return time.Now()
}
