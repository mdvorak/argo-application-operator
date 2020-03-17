package application

import (
	"errors"
	"fmt"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"os"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
)

const TargetNamespaceEnvVar = "TARGET_NAMESPACE"

func GetTargetNamespace() (string, error) {
	if targetNamespace, ok := os.LookupEnv(TargetNamespaceEnvVar); ok && len(targetNamespace) > 0 {
		return targetNamespace, nil
	} else {
		return "", errors.New(fmt.Sprintf("%s not set", TargetNamespaceEnvVar))
	}
}

func AddTargetNamespaceToWatched() error {
	log := logf.Log.WithName("controller")

	targetNamespace, ok := os.LookupEnv(TargetNamespaceEnvVar)
	if !ok {
		return errors.New(fmt.Sprintf("%s not set, cannot add it as watched namespace", TargetNamespaceEnvVar))
	}

	watchNamespace, ok := os.LookupEnv(k8sutil.WatchNamespaceEnvVar)
	// Empty string means everything is watched
	if !ok || len(watchNamespace) == 0 {
		return nil
	}

	// Add to env variable
	if !contains(strings.Split(watchNamespace, ","), targetNamespace) {
		// Set env var
		if err := os.Setenv(k8sutil.WatchNamespaceEnvVar, watchNamespace+","+targetNamespace); err != nil {
			// Failed
			return err
		} else {
			// OK
			log.Info(fmt.Sprintf("%s '%s' is not part of %s, forcefully added", TargetNamespaceEnvVar, targetNamespace, k8sutil.WatchNamespaceEnvVar))
		}
	}

	return nil
}
