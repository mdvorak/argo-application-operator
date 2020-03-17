package v1alpha1

import (
	argocdv1alpha1 "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ApplicationSpec defines the desired state of Application
type ApplicationSpec struct {
	// Source is a reference to the location ksonnet application definition
	Source argocdv1alpha1.ApplicationSource `json:"source"`
	// SyncPolicy controls when a sync will be performed
	SyncPolicy *argocdv1alpha1.SyncPolicy `json:"syncPolicy,omitempty"`
	// IgnoreDifferences controls resources fields which should be ignored during comparison
	IgnoreDifferences []argocdv1alpha1.ResourceIgnoreDifferences `json:"ignoreDifferences,omitempty"`
	// Infos contains a list of useful information (URLs, email addresses, and plain text) that relates to the application
	Info []argocdv1alpha1.Info `json:"info,omitempty"`
}

// ApplicationStatus defines the observed state of Application
type ApplicationStatus struct {
	LastUpdated     string           `json:"lastUpdated,omitempty"`
	OwnedReferences []OwnedReference `json:"ownedReferences,omitempty"`
}

// OwnedReference defines managed object
type OwnedReference struct {
	// API version of the referenced object
	APIVersion string `json:"apiVersion"`
	// Kind of the referenced object
	Kind string `json:"kind"`
	// Name of the referenced object
	Name string `json:"name"`
	// Namespace of the referenced object
	Namespace string `json:"namespace"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Application is the Schema for the applications API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=applications,scope=Namespaced
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApplicationList contains a list of Application
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}
