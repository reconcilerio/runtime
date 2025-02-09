/*
Copyright 2025 the original author or authors.

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

package testing

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"reconciler.io/runtime/reconcilers"
)

type DiffReason float64

const (
	DiffReasonRaw DiffReason = iota
	DiffReasonTrackRequest
	DiffReasonDeleteCollectionRef
	DiffReasonStashedValue
	DiffReasonResource
	DiffReasonWebhookResponse
	DiffReasonResourceStatusUpdate
	DiffReasonResourceUpdate
	DiffReasonResourceCreate
)

func DefaultDiff[T any](expected, actual T, reason DiffReason) string {
	opts := []cmp.Option{}
	switch reason {
	case DiffReasonTrackRequest:
		opts = []cmp.Option{
			NormalizeLabelSelector,
		}
	case DiffReasonDeleteCollectionRef:
		opts = []cmp.Option{
			NormalizeLabelSelector,
			NormalizeFieldSelector,
		}
	case DiffReasonStashedValue:
		opts = []cmp.Option{
			reconcilers.IgnoreAllUnexported,
			IgnoreLastTransitionTime,
			IgnoreTypeMeta,
			IgnoreCreationTimestamp,
			IgnoreResourceVersion,
			cmpopts.EquateEmpty(),
		}
	case DiffReasonWebhookResponse:
		opts = []cmp.Option{
			reconcilers.IgnoreAllUnexported,
		}
	case DiffReasonResource:
		opts = []cmp.Option{
			reconcilers.IgnoreAllUnexported,
			IgnoreLastTransitionTime,
			IgnoreTypeMeta,
			cmpopts.EquateEmpty(),
		}
	case DiffReasonResourceStatusUpdate:
		opts = []cmp.Option{
			statusSubresourceOnly,
			reconcilers.IgnoreAllUnexported,
			IgnoreLastTransitionTime,
			cmpopts.EquateEmpty(),
		}
	case DiffReasonResourceUpdate:
		opts = []cmp.Option{
			reconcilers.IgnoreAllUnexported,
			IgnoreLastTransitionTime,
			IgnoreTypeMeta,
			IgnoreCreationTimestamp,
			IgnoreResourceVersion,
			cmpopts.EquateEmpty(),
		}
	case DiffReasonResourceCreate:
		opts = []cmp.Option{
			reconcilers.IgnoreAllUnexported,
			IgnoreLastTransitionTime,
			IgnoreTypeMeta,
			IgnoreCreationTimestamp,
			IgnoreResourceVersion,
			cmpopts.EquateEmpty(),
		}
	}
	return cmp.Diff(expected, actual, opts...)
}
