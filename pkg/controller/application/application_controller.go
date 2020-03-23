package application

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"reflect"
	"strings"
	"time"

	argocdv1alpha1 "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	opsv1alpha1 "github.com/mdvorak/argo-application-operator/pkg/apis/ops/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_application")
var destinationServer string
var targetNamespace string

const applicationKind = "Application"
const applicationFinalizer = "finalizer.application.ops.csas.cz"
const ownerApiGroupLabel = "application.ops.csas.cz/owner-api-group"
const ownerApiVersionLabel = "application.ops.csas.cz/owner-api-version"
const ownerKindLabel = "application.ops.csas.cz/owner-kind"
const ownerNameLabel = "application.ops.csas.cz/owner-name"
const ownerNamespaceLabel = "application.ops.csas.cz/owner-namespace"
const managedByLabel = "app.kubernetes.io/managed-by"

// Add creates a new Application Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileApplication{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	var err error

	destinationServer = GetDestinationServer()
	targetNamespace, err = GetTargetNamespace()
	if err != nil {
		return fmt.Errorf("target namespace must be set: %w", err)
	}

	// Create a new controller
	c, err := controller.New("application-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return fmt.Errorf("failed to create new controller: %w", err)
	}

	// Watch for changes to primary resource Application
	err = c.Watch(&source.Kind{Type: &opsv1alpha1.Application{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("failed to watch source objects: %w", err)
	}

	// Watch for changes to secondary resource Application and requeue the owner Application
	err = c.Watch(&source.Kind{Type: &argocdv1alpha1.Application{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(watchMapFunc),
	})
	if err != nil {
		return fmt.Errorf("failed to watch target objects: %w", err)
	}

	return nil
}

func watchMapFunc(obj handler.MapObject) []reconcile.Request {
	apiGroup := obj.Meta.GetLabels()[ownerApiGroupLabel]
	apiVersion := obj.Meta.GetLabels()[ownerApiVersionLabel]
	kind := obj.Meta.GetLabels()[ownerKindLabel]

	if apiGroup != opsv1alpha1.SchemeGroupVersion.Group ||
		apiVersion != opsv1alpha1.SchemeGroupVersion.Version ||
		kind != applicationKind {
		// Mismatch, ignore
		return []reconcile.Request{}
	}

	// Start reconcile
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name:      obj.Meta.GetLabels()[ownerNameLabel],
			Namespace: obj.Meta.GetLabels()[ownerNamespaceLabel],
		}},
	}
}

// blank assignment to verify that ReconcileApplication implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileApplication{}

// ReconcileApplication reconciles a Application object
type ReconcileApplication struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Application object and makes changes based on the state read
// and what is in the Application.Spec
// TODO docs
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileApplication) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Application")

	ctx := context.TODO()

	// Fetch the Application instance
	instance := &opsv1alpha1.Application{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Define a new Argo Application object
	app := newApplication(instance)
	appLogger := reqLogger.WithValues("Application.Namespace", app.Namespace, "Application.Name", app.Name)

	// Check if the instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	markedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if markedToBeDeleted {
		appLogger.Info("Application is marked to be deleted")

		if contains(instance.GetFinalizers(), applicationFinalizer) {
			// Run finalization logic for our finalizer. If the finalization logic fails,
			// don't remove the finalizer so that we can retry during the next reconciliation.
			if err := r.finalizeApplication(ctx, appLogger, app); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to finalize application: %w", err)
			}

			// Remove the finalizer. Once all finalizers have been removed, the object will be deleted.
			if err := r.removeFinalizer(ctx, appLogger, instance); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to remove %s: %w", applicationFinalizer, err)
			}
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer for this CR
	if !contains(instance.GetFinalizers(), applicationFinalizer) {
		appLogger.Info("Adding finalizer")
		if err := r.addFinalizer(ctx, appLogger, instance); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to add %s: %w", applicationFinalizer, err)
		}
	}

	// Check if this Application already exists
	found := &argocdv1alpha1.Application{}
	err = r.client.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		appLogger.Info("Creating a new Application")
		err = r.client.Create(ctx, app)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to create Application: %w", err)
		}

		// Update status
		err = r.updateStatus(ctx, instance, app)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update status: %w", err)
		}

		// Application created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get existing Application: %w", err)
	}

	// Application exists, update
	isChanged := false
	for label, value := range app.Labels {
		if found.Labels[label] != value {
			found.Labels[label] = value
			isChanged = true
		}
	}
	if !reflect.DeepEqual(found.Spec, app.Spec) {
		found.Spec = app.Spec
		isChanged = true
	}

	// Change detected
	if isChanged {
		appLogger.Info("Updating Application")
		err = r.client.Update(ctx, found)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update existing Application: %w", err)
		}

		// Update status
		err = r.updateStatus(ctx, instance, found)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update status: %w", err)
		}
	}

	// Application already exists - don't requeue
	appLogger.Info("Reconcile finished")
	return reconcile.Result{}, nil
}

func (r *ReconcileApplication) addFinalizer(ctx context.Context, logger logr.Logger, instance *opsv1alpha1.Application) error {
	logger.Info("Adding Finalizer")

	// Prepare patch
	patch := client.MergeFrom(instance)

	// Copy object and set finalizers
	newInstance := instance.DeepCopy()
	newInstance.SetFinalizers(append(newInstance.GetFinalizers(), applicationFinalizer))

	// Patch object
	return r.client.Patch(ctx, instance, patch)
}

func (r *ReconcileApplication) removeFinalizer(ctx context.Context, logger logr.Logger, instance *opsv1alpha1.Application) error {
	logger.Info("Removing Finalizer")

	// Prepare patch
	patch := client.MergeFrom(instance)

	// Copy object and set finalizers
	newInstance := instance.DeepCopy()
	newInstance.SetFinalizers(remove(newInstance.GetFinalizers(), applicationFinalizer))

	// Patch object
	return r.client.Patch(ctx, instance, patch)
}

func (r *ReconcileApplication) finalizeApplication(ctx context.Context, logger logr.Logger, app *argocdv1alpha1.Application) error {
	logger.Info("Running finalizer")

	// Check if this Application exists
	found := &argocdv1alpha1.Application{}
	err := r.client.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, found)
	if err != nil {
		// If there was error but it wasn't NotFound, propagate the error
		if k8serrors.IsNotFound(err) {
			// Already deleted, nothing to do
			logger.Info("Application already deleted")
			return nil
		}

		return fmt.Errorf("failed to get Application for deletion: %w", err)
	}

	// Delete
	logger.Info("Deleting application")
	err = r.client.Delete(ctx, found)
	if err != nil {
		return fmt.Errorf("failed to delete Application: %w", err)
	}

	return nil
}

func (r *ReconcileApplication) updateStatus(ctx context.Context, cr *opsv1alpha1.Application, app *argocdv1alpha1.Application) error {
	// Create new status
	cr.Status = opsv1alpha1.ApplicationStatus{
		LastUpdated: time.Now().Format(time.RFC3339),
		OwnedReferences: []opsv1alpha1.OwnedReference{
			{
				APIVersion: app.APIVersion,
				Kind:       app.Kind,
				Name:       app.Name,
				Namespace:  app.Namespace,
			},
		},
	}

	// Update
	return r.client.Status().Update(ctx, cr)
}

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
	// Get name, ignore error
	operatorName, _ := k8sutil.GetOperatorName()

	// Return required labels
	return map[string]string{
		ownerApiGroupLabel:   opsv1alpha1.SchemeGroupVersion.Group,
		ownerApiVersionLabel: opsv1alpha1.SchemeGroupVersion.Version,
		ownerKindLabel:       applicationKind,
		ownerNamespaceLabel:  cr.Namespace,
		ownerNameLabel:       cr.Name,
		managedByLabel:       operatorName,
	}
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
