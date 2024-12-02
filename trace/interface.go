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

package trace

import (
	"context"
)

// Tracer observes the execution of a reconciler hierarchy.
type Tracer interface {
	// Enter opens a new frame.
	Enter(ctx context.Context, name string)
	// Exit closes the most recently entered open frame.
	Exit(ctx context.Context)
}

type TraceProvider = func(ctx context.Context) Tracer
