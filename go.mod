module github.com/mdvorak/argo-application-operator

go 1.13

require (
	github.com/argoproj/argo-cd v1.5.1
	github.com/argoproj/pkg v0.0.0-20200319004004-f46beff7cd54 // indirect
	github.com/go-logr/logr v0.1.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/operator-framework/operator-sdk v0.17.0
	github.com/spf13/pflag v1.0.5
	gopkg.in/src-d/go-git.v4 v4.13.1 // indirect
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.5.2
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.17.4 // Required by prometheus-operator
)
