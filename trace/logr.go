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
	"fmt"
	"time"

	"github.com/go-logr/logr"
)

var (
	_ Tracer = (*logrTracer)(nil)
)

// LogrTracer uses the context's logr to capture a message when entering and exiting.  The duration
// is captured in nanoseconds as the elapsedNs structured parameter.
//
// Not thread safe. Enter and Exit calls must be balanced.
func LogrTracer(opts LogrTracerOpts) Tracer {
	return &logrTracer{
		level: opts.Level,
		stack: []time.Time{},
	}
}

// LogrTraceProvider uses the context's logr to capture a message when entering and exiting.  The duration
// is captured in nanoseconds as the elapsedNs structured parameter.
//
// Not thread safe. Enter and Exit calls must be balanced.
func LogrTraceProvider(opts LogrTracerOpts) TraceProvider {
	return func(ctx context.Context) Tracer {
		return &logrTracer{
			level: opts.Level,
			stack: []time.Time{},
		}
	}
}

type LogrTracerOpts struct {
	// Level of verbosity to apply to log messages relative to the logger. A higher verbosity level
	// means a log message is less important.
	Level int
}

type logrTracer struct {
	level int
	stack []time.Time
}

func (t *logrTracer) Enter(ctx context.Context, name string) {
	log := logr.FromContextOrDiscard(ctx).V(t.level)
	now := time.Now()
	t.stack = append(t.stack, now)
	log.Info("trace enter")
}

func (t *logrTracer) Exit(ctx context.Context) {
	log := logr.FromContextOrDiscard(ctx).V(t.level)
	i := len(t.stack)
	if i == 0 {
		panic(fmt.Errorf("Exit() called more times than Enter() for a Tracer"))
	}
	start := t.stack[i-1]
	t.stack = t.stack[:i-1]
	if !log.Enabled() {
		return
	}
	now := time.Now()
	log.Info("trace exit", "elapsedNs", now.Sub(start))
}
