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
	"reconciler.io/runtime/reconcilers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type staticDiffer struct {
	diff string
}

func (d *staticDiffer) Result(expected, actual reconcilers.Result) string {
	return d.diff
}

func (d *staticDiffer) TrackRequest(expected, actual TrackRequest) string {
	return d.diff
}

func (d *staticDiffer) Event(expected, actual Event) string {
	return d.diff
}

func (d *staticDiffer) PatchRef(expected, actual PatchRef) string {
	return d.diff
}

func (d *staticDiffer) DeleteRef(expected, actual DeleteRef) string {
	return d.diff
}

func (d *staticDiffer) DeleteCollectionRef(expected, actual DeleteCollectionRef) string {
	return d.diff
}

func (d *staticDiffer) StashedValue(expected, actual any, key reconcilers.StashKey) string {
	return d.diff
}

func (d *staticDiffer) Resource(expected, actual client.Object) string {
	return d.diff
}

func (d *staticDiffer) ResourceStatusUpdate(expected, actual client.Object) string {
	return d.diff
}

func (d *staticDiffer) ResourceUpdate(expected, actual client.Object) string {
	return d.diff
}

func (d *staticDiffer) ResourceCreate(expected, actual client.Object) string {
	return d.diff
}

func (d *staticDiffer) WebhookResponse(expected, actual admission.Response) string {
	return d.diff
}
