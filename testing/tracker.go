/*
Copyright 2019 the original author or authors.

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
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/reference"
	"reconciler.io/runtime/duck"
	"reconciler.io/runtime/tracker"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TrackRequest records that one object is tracking another object.
type TrackRequest struct {
	// Tracker is the object doing the tracking
	Tracker types.NamespacedName

	// Deprecated use TrackedReference
	// Tracked is the object being tracked
	Tracked tracker.Key

	// TrackedReference is a ref to the object being tracked
	TrackedReference tracker.Reference
}

func (tr *TrackRequest) normalize() {
	if tr.TrackedReference != (tracker.Reference{}) {
		return
	}
	tr.TrackedReference = tracker.Reference{
		APIGroup:  tr.Tracked.GroupKind.Group,
		Kind:      tr.Tracked.GroupKind.Kind,
		Namespace: tr.Tracked.NamespacedName.Namespace,
		Name:      tr.Tracked.NamespacedName.Name,
	}
	tr.Tracked = tracker.Key{}
}

type trackBy func(trackingObjNamespace, trackingObjName string) TrackRequest

func (t trackBy) By(trackingObjNamespace, trackingObjName string) TrackRequest {
	return t(trackingObjNamespace, trackingObjName)
}

func CreateTrackRequest(trackedObjGroup, trackedObjKind, trackedObjNamespace, trackedObjName string) trackBy {
	return func(trackingObjNamespace, trackingObjName string) TrackRequest {
		return TrackRequest{
			TrackedReference: tracker.Reference{
				APIGroup:  trackedObjGroup,
				Kind:      trackedObjKind,
				Namespace: trackedObjNamespace,
				Name:      trackedObjName,
			},
			Tracker: types.NamespacedName{Namespace: trackingObjNamespace, Name: trackingObjName},
		}
	}
}

func NewTrackRequest(t, b client.Object, scheme *runtime.Scheme) TrackRequest {
	tracked, by := t.DeepCopyObject().(client.Object), b.DeepCopyObject().(client.Object)

	gvk := tracked.GetObjectKind().GroupVersionKind()
	if !duck.IsDuck(tracked, scheme) {
		gvks, _, err := scheme.ObjectKinds(tracked)
		if err != nil {
			panic(err)
		}
		gvk = gvks[0]
	}

	return TrackRequest{
		TrackedReference: tracker.Reference{
			APIGroup:  gvk.Group,
			Kind:      gvk.Kind,
			Namespace: tracked.GetNamespace(),
			Name:      tracked.GetName(),
		},
		Tracker: types.NamespacedName{Namespace: by.GetNamespace(), Name: by.GetName()},
	}
}

func createTracker(given []TrackRequest, scheme *runtime.Scheme) *mockTracker {
	t := &mockTracker{
		Tracker: tracker.New(scheme, 24*time.Hour),
		scheme:  scheme,
	}
	for _, g := range given {
		g.normalize()
		obj := &unstructured.Unstructured{}
		obj.SetNamespace(g.Tracker.Namespace)
		obj.SetName(g.Tracker.Name)
		t.TrackReference(g.TrackedReference, obj)
	}
	// reset tracked requests
	t.reqs = []TrackRequest{}
	return t
}

type mockTracker struct {
	tracker.Tracker
	reqs   []TrackRequest
	scheme *runtime.Scheme
}

var _ tracker.Tracker = &mockTracker{}

// TrackObject tells us that "obj" is tracking changes to the
// referenced object.
func (t *mockTracker) TrackObject(ref client.Object, obj client.Object) error {
	or, err := reference.GetReference(t.scheme, ref)
	if err != nil {
		return err
	}
	gv := schema.FromAPIVersionAndKind(or.APIVersion, or.Kind)
	return t.TrackReference(tracker.Reference{
		APIGroup:  gv.Group,
		Kind:      gv.Kind,
		Namespace: ref.GetNamespace(),
		Name:      ref.GetName(),
	}, obj)
}

// TrackReference tells us that "obj" is tracking changes to the
// referenced object.
func (t *mockTracker) TrackReference(ref tracker.Reference, obj client.Object) error {
	t.reqs = append(t.reqs, TrackRequest{
		Tracker: types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		},
		TrackedReference: ref,
	})
	return t.Tracker.TrackReference(ref, obj)
}

func (t *mockTracker) getTrackRequests() []TrackRequest {
	return append([]TrackRequest{}, t.reqs...)
}
