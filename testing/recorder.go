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

package testing

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
	ref "k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Event struct {
	metav1.TypeMeta
	types.NamespacedName
	Related *corev1.ObjectReference
	Type    string
	Reason  string
	// Deprecated use Action and Note
	Message string
	Action  string
	Note    string
}

// Deprecated, prefer NewEventf
func NewEvent(factory client.Object, scheme *runtime.Scheme, eventtype, reason, messageFormat string, a ...interface{}) Event {
	obj := factory.DeepCopyObject()
	objref, err := ref.GetReference(scheme, obj)
	if err != nil {
		panic(fmt.Sprintf("Could not construct reference to: '%#v' due to: '%v'. Will not report event: '%v' '%v' '%v'", obj, err, eventtype, reason, fmt.Sprintf(messageFormat, a...)))
	}

	return Event{
		TypeMeta: metav1.TypeMeta{
			APIVersion: objref.APIVersion,
			Kind:       objref.Kind,
		},
		NamespacedName: types.NamespacedName{
			Namespace: objref.Namespace,
			Name:      objref.Name,
		},
		Type:    eventtype,
		Reason:  reason,
		Message: fmt.Sprintf(messageFormat, a...),
	}
}

func NewEventf(regarding, related client.Object, scheme *runtime.Scheme, eventtype, reason, action, note string, a ...interface{}) Event {
	regardingref, err := ref.GetReference(scheme, regarding.DeepCopyObject())
	if err != nil {
		panic(fmt.Sprintf("Could not construct reference to: '%#v' due to: '%v'. Will not report event: '%v' '%v' '%v' '%v'", regarding, err, eventtype, reason, action, fmt.Sprintf(note, a...)))
	}
	var relatedref *corev1.ObjectReference
	if related != nil {
		relatedref, err = ref.GetReference(scheme, related.DeepCopyObject())
		if err != nil {
			panic(fmt.Sprintf("Could not construct reference to: '%#v' due to: '%v'. Will not report event: '%v' '%v' '%v' '%v'", related, err, eventtype, reason, action, fmt.Sprintf(note, a...)))
		}

	}

	return Event{
		TypeMeta: metav1.TypeMeta{
			APIVersion: regardingref.APIVersion,
			Kind:       regardingref.Kind,
		},
		NamespacedName: types.NamespacedName{
			Namespace: regardingref.Namespace,
			Name:      regardingref.Name,
		},
		Related: relatedref,
		Type:    eventtype,
		Reason:  reason,
		Action:  action,
		Note:    fmt.Sprintf(note, a...),
	}
}

type deprecatedEventRecorder struct {
	recorder *eventRecorder
}

var (
	_ record.EventRecorder = (*deprecatedEventRecorder)(nil)
)

func (r *deprecatedEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.Eventf(object, eventtype, reason, "%s", message)
}

func (r *deprecatedEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	r.recorder.events = append(r.recorder.events, NewEvent(object.(client.Object), r.recorder.scheme, eventtype, reason, messageFmt, args...))
}

func (r *deprecatedEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	r.Eventf(object, eventtype, reason, messageFmt, args...)
}

type eventRecorder struct {
	events []Event
	scheme *runtime.Scheme
}

var (
	_ events.EventRecorder = (*eventRecorder)(nil)
)

func (r *eventRecorder) Eventf(regarding runtime.Object, related runtime.Object, eventtype, reason, action, note string, args ...interface{}) {
	var regardingObj, relatedObj client.Object
	regardingObj = regarding.(client.Object)
	if related != nil {
		relatedObj = related.(client.Object)
	}

	r.events = append(r.events, NewEventf(regardingObj, relatedObj, r.scheme, eventtype, reason, action, note, args...))
}
