module github.com/openshift/osd-utils-cli

go 1.14

require (
	github.com/aws/aws-sdk-go v1.31.10
	github.com/onsi/gomega v1.10.1
	github.com/openshift/api v3.9.0+incompatible
	github.com/openshift/aws-account-operator v0.0.0-20200529133510-076b8c994393
	github.com/pkg/errors v0.9.1
	github.com/prometheus/common v0.10.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/cli-runtime v0.18.3
	k8s.io/client-go v0.18.3
	k8s.io/component-base v0.18.3
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.18.3
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	sigs.k8s.io/controller-runtime v0.6.0
)

replace github.com/openshift/api v3.9.0+incompatible => github.com/openshift/api v0.0.0-20200617152309-e9562717e6cd
