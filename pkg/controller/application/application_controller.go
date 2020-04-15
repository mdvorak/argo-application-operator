package application

import (
	"context"
	"fmt"
	argocdv1alpha1 "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	opsv1alpha1 "github.com/mdvorak/argo-application-operator/pkg/apis/ops/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/status"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
var argoNamespace string

const applicationFinalizer = "finalizer.application.ops.csas.cz"
const availableCondition = "Available"

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
	argoNamespace, err = GetArgoNamespace()
	if err != nil {
		return fmt.Errorf("argo namespace must be set: %w", err)
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
	}, applicationUpdatedPredicate{})
	if err != nil {
		return fmt.Errorf("failed to watch target objects: %w", err)
	}

	return nil
}

// Filtering function for generic watcher
func watchMapFunc(obj handler.MapObject) []reconcile.Request {
	apiGroup := obj.Meta.GetLabels()[ownerApiGroupLabel]
	apiVersion := obj.Meta.GetLabels()[ownerApiVersionLabel]
	kind := obj.Meta.GetLabels()[ownerKindLabel]

	if apiGroup != opsv1alpha1.SchemeGroupVersion.Group ||
		apiVersion != opsv1alpha1.SchemeGroupVersion.Version ||
		kind != opsv1alpha1.KindApplication {
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
//
// Manages Application.argocd.io object with corresponding specification in namespace set by ARGO_NAMESPACE env var.
//
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileApplication) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("reconciling Application.ops.csas.cz")

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

	// Reconciliation logic
	result, available, err := r.reconcileApplication(ctx, reqLogger, instance)

	// Update status
	r.updateCondition(ctx, reqLogger, instance, r.newAvailableCondition(available, err))

	// Return
	reqLogger.Info("reconcile finished")
	return result, err
}

func (r *ReconcileApplication) reconcileApplication(ctx context.Context, logger logr.Logger, cr *opsv1alpha1.Application) (reconcile.Result, bool, error) {
	// Define a new Argo Application object
	app := newApplication(cr)
	appLogger := logger.WithValues("Application.Namespace", app.Namespace, "Application.Name", app.Name)

	// Check if the instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	markedToBeDeleted := cr.GetDeletionTimestamp() != nil
	if markedToBeDeleted {
		// Delete target object
		appLogger.Info("Application.ops.csas.cz is marked to be deleted")
		result, err := r.reconcileDeletion(ctx, appLogger, cr, app)

		return result, false, err
	}

	// Add finalizer for this CR
	if !contains(cr.GetFinalizers(), applicationFinalizer) {
		logger.Info("adding finalizer to Application.ops.csas.cz")
		if err := r.updateFinalizers(ctx, cr, append(cr.GetFinalizers(), applicationFinalizer)); err != nil {
			return reconcile.Result{}, false, fmt.Errorf("failed to add %s to Application.ops.csas.cz: %w", applicationFinalizer, err)
		}
	}

	// Update application
	result, err := r.reconcileUpdate(ctx, appLogger, cr, app)
	return result, true, err
}

func (r *ReconcileApplication) reconcileUpdate(ctx context.Context, logger logr.Logger, cr *opsv1alpha1.Application, app *argocdv1alpha1.Application) (reconcile.Result, error) {
	// Check if this Application already exists
	found := &argocdv1alpha1.Application{}
	err := r.client.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		logger.Info("creating a new Application.argocd.io")
		err = r.client.Create(ctx, app)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to create Application.argocd.io: %w", err)
		}

		// WORKAROUND: sometimes Create removes TypeMeta information, dunno why
		if app.Kind == "" {
			app.TypeMeta = newApplicationTypeMeta()
		}

		// Add reference
		r.addReference(ctx, logger, cr, app)

		// Application created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get existing Application.argocd.io: %w", err)
	}

	// Verify ownership
	if isApplicationOwnedBy(found, cr) {
		// Not owned by this CR! This will fail repeatedly, but its ok - should not happen in real-life
		return reconcile.Result{}, fmt.Errorf("object %s.%s \"%s\" in namespace \"%s\" already exists, and it is not owned by this object", found.Kind, found.GroupVersionKind().Group, found.Name, found.Namespace)
	}

	// Add reference
	r.addReference(ctx, logger, cr, found)

	// Application exists, update
	if patchApplication(found, app) {
		logger.Info("updating existing Application.argocd.io")
		err = r.client.Update(ctx, found)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update existing Application.argocd.io: %w", err)
		}
	}

	// Application already exists - don't requeue
	return reconcile.Result{}, nil
}

func (r *ReconcileApplication) reconcileDeletion(ctx context.Context, logger logr.Logger, cr *opsv1alpha1.Application, app *argocdv1alpha1.Application) (reconcile.Result, error) {
	if contains(cr.GetFinalizers(), applicationFinalizer) {
		// Run finalization logic for our finalizer. If the finalization logic fails,
		// don't remove the finalizer so that we can retry during the next reconciliation.
		if err := r.finalizeApplication(ctx, logger, app); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to finalize Application.ops.csas.cz: %w", err)
		}

		// Remove the finalizer. Once all finalizers have been removed, the object will be deleted.
		logger.Info("removing finalizer from Application.ops.csas.cz")
		if err := r.updateFinalizers(ctx, cr, remove(cr.GetFinalizers(), applicationFinalizer)); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to remove %s from Application.ops.csas.cz: %w", applicationFinalizer, err)
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileApplication) finalizeApplication(ctx context.Context, logger logr.Logger, app *argocdv1alpha1.Application) error {
	logger.Info("running finalizer " + applicationFinalizer)

	// Check if this Application exists
	found := &argocdv1alpha1.Application{}
	err := r.client.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, found)
	if err != nil {
		// If there was error but it wasn't NotFound, propagate the error
		if k8serrors.IsNotFound(err) {
			// Already deleted, nothing to do
			logger.Info("Application.argocd.io already deleted")
			return nil
		}

		return fmt.Errorf("failed to get Application.argocd.io for deletion: %w", err)
	}

	// Delete
	logger.Info("deleting Application.argocd.io")
	err = r.client.Delete(ctx, found)
	if err != nil {
		return fmt.Errorf("failed to delete Application.argocd.io: %w", err)
	}

	return nil
}

// Create new Condition of type Available with human readable message
func (r *ReconcileApplication) newAvailableCondition(available bool, err error) status.Condition {
	if err != nil {
		// Error
		return status.Condition{
			Type:    availableCondition,
			Status:  corev1.ConditionFalse,
			Reason:  "Failed",
			Message: err.Error(),
		}
	} else if available {
		// Exists
		return status.Condition{
			Type:    availableCondition,
			Status:  corev1.ConditionTrue,
			Reason:  "Created",
			Message: "reconciliation successful",
		}
	} else {
		// Deleted
		return status.Condition{
			Type:    availableCondition,
			Status:  corev1.ConditionFalse,
			Reason:  "Deleted",
			Message: "reconciliation successful",
		}
	}
}

// Store a Condition into CR status.conditions
func (r *ReconcileApplication) updateCondition(ctx context.Context, logger logr.Logger, cr *opsv1alpha1.Application, cond status.Condition) {
	// Copy instance for comparison
	newInstance := cr.DeepCopy()

	// Update only if changed
	if newInstance.Status.Conditions.SetCondition(cond) {
		logger.Info("updating condition", "Condition.Type", cond.Type, "Condition.Status", cond.Status, "Condition.Reason", cond.Reason, "Condition.Message", cond.Message)

		// Patch object
		if err := r.client.Status().Patch(ctx, newInstance, client.MergeFrom(cr)); err != nil && !k8serrors.IsNotFound(err) {
			// Log error without failing - note that NotFound is ignored silently
			logger.Error(err, "failed to update status of Application.ops.csas.cz")
		} else if err != nil {
			// Update original instance
			cr.Status = newInstance.Status
		}
	}
}

// Store a Reference to given Application into CR status.references
func (r *ReconcileApplication) addReference(ctx context.Context, logger logr.Logger, cr *opsv1alpha1.Application, app *argocdv1alpha1.Application) {
	// Copy instance for comparison
	newInstance := cr.DeepCopy()

	// Update only if changed
	if newInstance.Status.References.SetReference(opsv1alpha1.ReferenceFromApplication(app)) {
		logger.Info("updating reference")

		// Patch object
		if err := r.client.Status().Patch(ctx, newInstance, client.MergeFrom(cr)); err != nil {
			// Log error without failing
			logger.Error(err, "failed to add reference to Application.ops.csas.cz")
		} else {
			// Update original instance
			cr.Status = newInstance.Status
		}
	}
}

func (r *ReconcileApplication) updateFinalizers(ctx context.Context, cr *opsv1alpha1.Application, newFinalizers []string) error {
	// Copy instance for patch
	newInstance := cr.DeepCopy()
	newInstance.SetFinalizers(newFinalizers)

	// Patch object
	if err := r.client.Patch(ctx, newInstance, client.MergeFrom(cr)); err != nil {
		return err
	}

	// Propagate change to original instance
	cr.SetFinalizers(newFinalizers)
	return nil
}
