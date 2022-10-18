/*
Copyright 2022.

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

package controllers

import (
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var predicatesLog = log.Log.WithName("PatroniParty").WithName("eventFilters")

type PatroniPartyPredicate struct {
	predicate.Funcs
}

// Update implements default UpdateEvent filter for validating generation change.
func (PatroniPartyPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil {
		predicatesLog.Error(nil, "Update event has no old object to update", "event", e)
		return false
	}
	if e.ObjectNew == nil {
		predicatesLog.Error(nil, "Update event has no new object for update", "event", e)
		return false
	}

	if _, ok := e.ObjectNew.GetAnnotations()[partyAnnotation]; ok {
		// This is Patroni StatefulSet
		return reflect.DeepEqual(e.ObjectNew.GetAnnotations(), e.ObjectOld.GetAnnotations()) && reflect.DeepEqual(e.ObjectNew.GetLabels(), e.ObjectOld.GetLabels())
	}

	// This is PatroniParty Object
	return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
}
