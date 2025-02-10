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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type Differ interface {
	Raw(expected, actual any) string
	TrackRequest(expected, actual TrackRequest) string
	DeleteCollectionRef(expected, actual DeleteCollectionRef) string
	StashedValue(expected, actual any, key reconcilers.StashKey) string
	Resource(expected, actual client.Object) string
	ResourceStatusUpdate(expected, actual client.Object) string
	ResourceUpdate(expected, actual client.Object) string
	ResourceCreate(expected, actual client.Object) string
	WebhookResponse(expected, actual admission.Response) string
}

// DefaultDiffer is a basic implementation of the Differ interface that is used by default unless
// overridden for a specific test case or globally.
var DefaultDiffer Differ = &differ{}

type differ struct{}

func (*differ) Raw(expected, actual any) string {
	return cmp.Diff(expected, actual)
}

func (*differ) TrackRequest(expected, actual TrackRequest) string {
	return cmp.Diff(expected, actual, NormalizeLabelSelector)
}

func (*differ) DeleteCollectionRef(expected, actual DeleteCollectionRef) string {
	return cmp.Diff(expected, actual, NormalizeLabelSelector, NormalizeFieldSelector)
}

func (*differ) StashedValue(expected, actual any, key reconcilers.StashKey) string {
	return cmp.Diff(expected, actual, reconcilers.IgnoreAllUnexported,
		IgnoreLastTransitionTime,
		IgnoreTypeMeta,
		IgnoreCreationTimestamp,
		IgnoreResourceVersion,
		cmpopts.EquateEmpty())
}

func (*differ) Resource(expected, actual client.Object) string {
	return cmp.Diff(expected, actual, reconcilers.IgnoreAllUnexported,
		IgnoreLastTransitionTime,
		IgnoreTypeMeta,
		cmpopts.EquateEmpty())
}

func (*differ) ResourceStatusUpdate(expected, actual client.Object) string {
	return cmp.Diff(expected, actual, statusSubresourceOnly,
		reconcilers.IgnoreAllUnexported,
		IgnoreLastTransitionTime,
		cmpopts.EquateEmpty())
}

func (*differ) ResourceUpdate(expected, actual client.Object) string {
	return cmp.Diff(expected, actual, reconcilers.IgnoreAllUnexported,
		IgnoreLastTransitionTime,
		IgnoreTypeMeta,
		IgnoreCreationTimestamp,
		IgnoreResourceVersion,
		cmpopts.EquateEmpty())
}

func (*differ) ResourceCreate(expected, actual client.Object) string {
	return cmp.Diff(expected, actual, reconcilers.IgnoreAllUnexported,
		IgnoreLastTransitionTime,
		IgnoreTypeMeta,
		IgnoreCreationTimestamp,
		IgnoreResourceVersion,
		cmpopts.EquateEmpty())
}

func (*differ) WebhookResponse(expected, actual admission.Response) string {
	return cmp.Diff(expected, actual, reconcilers.IgnoreAllUnexported)
}
