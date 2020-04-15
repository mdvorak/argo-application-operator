package argocd

import (
	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Detect update only when object changes, ignores Status
type ApplicationUpdatedPredicate struct {
	predicate.Predicate
}

// Update returns true if the Update event should be processed
func (p ApplicationUpdatedPredicate) Update(e event.UpdateEvent) bool {
	objNew := e.ObjectNew.(*v1alpha1.Application)
	objOld := e.ObjectOld.(*v1alpha1.Application)

	// Compare what we are interested in
	// NOTE we need to ignore Status! Argo updates it every 5 secs
	return !reflect.DeepEqual(objNew.Labels, objOld.Labels) ||
		!reflect.DeepEqual(objNew.Annotations, objOld.Annotations) ||
		!reflect.DeepEqual(objNew.Spec, objOld.Spec) ||
		!reflect.DeepEqual(objNew.Operation, objOld.Operation) ||
		!reflect.DeepEqual(objNew.OwnerReferences, objOld.OwnerReferences) ||
		!reflect.DeepEqual(objNew.DeletionTimestamp, objOld.DeletionTimestamp) ||
		!reflect.DeepEqual(objNew.Finalizers, objOld.Finalizers)
}

// Create returns true if the Create event should be processed
func (p ApplicationUpdatedPredicate) Create(event.CreateEvent) bool {
	return true
}

// Delete returns true if the Delete event should be processed
func (p ApplicationUpdatedPredicate) Delete(event.DeleteEvent) bool {
	return true
}

// Generic returns true if the Generic event should be processed
func (p ApplicationUpdatedPredicate) Generic(event.GenericEvent) bool {
	return true
}
