package application

import (
	argocdv1alpha1 "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	opsv1alpha1 "github.com/mdvorak/argo-application-operator/pkg/apis/ops/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"strings"
)

func newApplication(cr *opsv1alpha1.Application) *argocdv1alpha1.Application {
	name := cr.Name
	if !strings.HasPrefix(name, cr.Namespace+"-") {
		name = cr.Namespace + "-" + cr.Name
	}

	return &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: targetNamespace,
			Labels:    applicationLabels(cr),
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: argocdv1alpha1.SchemeGroupVersion.String(),
		},
		Spec: newApplicationSpec(cr),
	}
}

func applicationLabels(cr *opsv1alpha1.Application) map[string]string {
	labels := map[string]string{
		ownerApiGroupLabel:   opsv1alpha1.SchemeGroupVersion.Group,
		ownerApiVersionLabel: opsv1alpha1.SchemeGroupVersion.Version,
		ownerKindLabel:       applicationKind,
		ownerNamespaceLabel:  cr.Namespace,
		ownerNameLabel:       cr.Name,
	}

	// Get name, ignore error
	operatorName, _ := k8sutil.GetOperatorName()
	if len(operatorName) > 0 {
		labels[managedByLabel] = operatorName
	}

	// Return
	return labels
}

func newApplicationSpec(cr *opsv1alpha1.Application) argocdv1alpha1.ApplicationSpec {
	return argocdv1alpha1.ApplicationSpec{
		Source: cr.Spec.Source,
		Destination: argocdv1alpha1.ApplicationDestination{
			Server:    destinationServer,
			Namespace: cr.Namespace,
		},
		Project:              cr.Namespace,
		SyncPolicy:           cr.Spec.SyncPolicy,
		IgnoreDifferences:    cr.Spec.IgnoreDifferences,
		Info:                 cr.Spec.Info,
		RevisionHistoryLimit: nil,
	}
}

func patchApplication(obj *argocdv1alpha1.Application, source *argocdv1alpha1.Application) (change bool) {
	// Compare and update labels
	for label, value := range source.Labels {
		if obj.Labels[label] != value {
			obj.Labels[label] = value
			change = true
		}
	}

	// Compare and update spec
	if !reflect.DeepEqual(obj.Spec, source.Spec) {
		obj.Spec = source.Spec
		change = true
	}

	return
}