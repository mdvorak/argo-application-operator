package argocd

import (
	"errors"
	"fmt"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"os"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
)

//noinspection GoUnusedConst
const (
	NamespaceEnvVar          = "ARGOCD_NAMESPACE"
	DestinationServerEnvVar  = "ARGOCD_DESTINATION_SERVER"
	DestinationServerDefault = "https://kubernetes.default.svc"
	ControllerServiceAccount = "argocd-application-controller"
	ServerServiceAccount     = "argocd-server"
)

func GetDestinationServer() string {
	if value, ok := os.LookupEnv(DestinationServerEnvVar); ok && len(value) > 0 {
		return value
	} else {
		// Default
		return DestinationServerDefault
	}
}

func GetNamespace() (string, error) {
	if value, ok := os.LookupEnv(NamespaceEnvVar); ok && len(value) > 0 {
		return value, nil
	} else {
		return "", errors.New(fmt.Sprintf("%s not set", NamespaceEnvVar))
	}
}

func AddNamespaceToWatched() error {
	log := logf.Log.WithName("argocd_env")

	argoNamespace, ok := os.LookupEnv(NamespaceEnvVar)
	if !ok {
		return errors.New(fmt.Sprintf("%s not set, cannot add it as watched namespace", NamespaceEnvVar))
	}

	watchNamespace, ok := os.LookupEnv(k8sutil.WatchNamespaceEnvVar)
	// Empty string means everything is watched
	if !ok || len(watchNamespace) == 0 {
		return nil
	}

	// Add to env variable
	if !contains(strings.Split(watchNamespace, ","), argoNamespace) {
		// Set env var
		if err := os.Setenv(k8sutil.WatchNamespaceEnvVar, watchNamespace+","+argoNamespace); err != nil {
			// Failed
			return err
		} else {
			// OK
			log.Info(fmt.Sprintf("%s '%s' is not part of %s, forcefully added", NamespaceEnvVar, argoNamespace, k8sutil.WatchNamespaceEnvVar))
		}
	}

	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
