package application

import (
	"context"
	"errors"
	"github.com/go-logr/logr"
	"os"
	"strings"

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
var targetNamespace string

const TargetNamespaceEnvVar = "TARGET_NAMESPACE"
const applicationFinalizer = "finalizer.application.ops.csas.cz"
const ownerApiVersionLabel = "application.ops.csas.cz/owner-apiVersion"
const ownerKindLabel = "application.ops.csas.cz/owner-kind"
const ownerNameLabel = "application.ops.csas.cz/owner-name"
const ownerNamespaceLabel = "application.ops.csas.cz/owner-namespace"

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
	// Store targetNamespace
	if ns, ok := os.LookupEnv(TargetNamespaceEnvVar); ok && len(ns) > 0 {
		targetNamespace = ns
	} else {
		// TODO fallback
		return errors.New(TargetNamespaceEnvVar + " not set")
	}

	// Create a new controller
	c, err := controller.New("application-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Application
	err = c.Watch(&source.Kind{Type: &opsv1alpha1.Application{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Application and requeue the owner Application
	err = c.Watch(&source.Kind{Type: &argocdv1alpha1.Application{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
			// TODO kind, ver, and only if matches
			// managed-by
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      obj.Meta.GetLabels()[ownerNameLabel],
					Namespace: obj.Meta.GetLabels()[ownerNamespaceLabel],
				}},
			}
		}),
	})
	if err != nil {
		return err
	}

	return nil
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

	// Fetch the Application instance
	instance := &opsv1alpha1.Application{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
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

	// Check if the instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	markedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if markedToBeDeleted {
		if contains(instance.GetFinalizers(), applicationFinalizer) {
			// Run finalization logic for our finalizer. If the finalization logic fails,
			// don't remove the finalizer so that we can retry during the next reconciliation.
			if err := r.finalizeApplication(reqLogger, instance, app); err != nil {
				return reconcile.Result{}, err
			}

			// Remove the finalizer. Once all finalizers have been removed, the object will be deleted.
			if err := r.removeFinalizer(reqLogger, instance); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer for this CR
	if !contains(instance.GetFinalizers(), applicationFinalizer) {
		if err := r.addFinalizer(reqLogger, instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	// Check if this Application already exists
	found := &argocdv1alpha1.Application{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		reqLogger.Info("Creating a new Application", "Application.Namespace", app.Namespace, "Application.Name", app.Name)
		err = r.client.Create(context.TODO(), app)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Application created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Application already exists - don't requeue
	reqLogger.Info("Skip reconcile: Application already exists", "Application.Namespace", found.Namespace, "Application.Name", found.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileApplication) addFinalizer(reqLogger logr.Logger, instance *opsv1alpha1.Application) error {
	reqLogger.Info("Adding Finalizer")
	instance.SetFinalizers(append(instance.GetFinalizers(), applicationFinalizer))

	// Update CR
	err := r.client.Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Error(err, "Failed to update Application with finalizer")
		return err
	}
	return nil
}

func (r *ReconcileApplication) removeFinalizer(reqLogger logr.Logger, instance *opsv1alpha1.Application) error {
	instance.SetFinalizers(remove(instance.GetFinalizers(), applicationFinalizer))
	err := r.client.Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Error(err, "Failed to update Application without finalizer")
		return err
	}
	return nil
}

func (r *ReconcileApplication) finalizeApplication(reqLogger logr.Logger, instance *opsv1alpha1.Application, app *argocdv1alpha1.Application) error {
	reqLogger.Info("Running finalizer", "Application.Namespace", instance.Namespace, "Application.Name", instance.Name)

	// Check if this Application exists
	found := &argocdv1alpha1.Application{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, found)
	if err != nil {
		// If there was error but it wasn't NotFound, propagate the error
		if k8serrors.IsNotFound(err) {
			// Already deleted, nothing to do
			return nil
		}
		return err
	}

	// Delete
	err = r.client.Delete(context.TODO(), found)
	if err != nil {
		reqLogger.Error(err, "Failed to delete Application")
		return err
	}
	return nil
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
			Labels: map[string]string{
				ownerApiVersionLabel: opsv1alpha1.SchemeGroupVersion.String(),
				ownerKindLabel:       "Application",
				ownerNamespaceLabel:  cr.Namespace,
				ownerNameLabel:       cr.Name,
			},
		},
		Spec: argocdv1alpha1.ApplicationSpec{
			Source: cr.Spec.Source,
			Destination: argocdv1alpha1.ApplicationDestination{
				Server:    "https://kubernetes.default.svc", // TODO
				Namespace: cr.Namespace,
			},
			Project:              cr.Namespace,
			SyncPolicy:           cr.Spec.SyncPolicy,
			IgnoreDifferences:    cr.Spec.IgnoreDifferences,
			Info:                 cr.Spec.Info,
			RevisionHistoryLimit: nil,
		},
	}
}
