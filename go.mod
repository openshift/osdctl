module github.com/openshift/osdctl

go 1.16

require (
	github.com/aws/aws-sdk-go v1.42.20
	github.com/aws/aws-sdk-go-v2/credentials v1.12.2
	github.com/coreos/go-semver v0.3.0
	github.com/deckarep/golang-set v1.7.1
	github.com/golang/mock v1.6.0
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/onsi/gomega v1.16.0
	github.com/openshift-online/ocm-cli v0.1.59
	github.com/openshift-online/ocm-sdk-go v0.1.224
	github.com/openshift/api v3.9.1-0.20191111211345-a27ff30ebf09+incompatible
	github.com/openshift/aws-account-operator/pkg/apis v0.0.0-20210611151019-01b1df7a3e9e
	github.com/openshift/gcp-project-operator v0.0.0-20210906153132-ce9b2425f1a7
	github.com/openshift/hive v1.0.5
	github.com/openshift/osd-network-verifier v0.0.0-20220518154805-047e42cfe29f
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4
	github.com/pkg/errors v0.9.1
	github.com/prometheus/common v0.28.0
	github.com/shopspring/decimal v1.2.0
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.16.0 // indirect
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.18.5
	k8s.io/apimachinery v0.18.5
	k8s.io/cli-runtime v0.18.3
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog/v2 v2.9.0
	k8s.io/kubectl v0.18.3
	k8s.io/utils v0.0.0-20200327001022-6496210b90e8
	sigs.k8s.io/controller-runtime v0.6.0
)

replace (
	bitbucket.org/ww/goautoneg => github.com/munnerz/goautoneg v0.0.0-20190414153302-2ae31c8b6b30
	github.com/coreos/go-systemd => github.com/coreos/go-systemd/v22 v22.0.0 // Pin non-versioned import to v22.0.0
	github.com/metal3-io/baremetal-operator => github.com/openshift/baremetal-operator v0.0.0-20200206190020-71b826cc0f0a // Use OpenShift fork
	github.com/metal3-io/cluster-api-provider-baremetal => github.com/openshift/cluster-api-provider-baremetal v0.0.0-20190821174549-a2a477909c1d // Pin OpenShift fork
	github.com/openshift/api v3.9.0+incompatible => github.com/openshift/api v0.0.0-20200617152309-e9562717e6cd
	github.com/terraform-providers/terraform-provider-aws => github.com/openshift/terraform-provider-aws v1.60.1-0.20200526184553-1a716dcc0fa8 // Pin to openshift fork with tag v2.60.0-openshift-1
	github.com/terraform-providers/terraform-provider-azurerm => github.com/openshift/terraform-provider-azurerm v1.41.1-openshift-3 // Pin to openshift fork with IPv6 fixes
	k8s.io/client-go => k8s.io/client-go v0.18.3
	sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20200506073438-9d49428ff837 // Pin OpenShift fork
	sigs.k8s.io/cluster-api-provider-azure => github.com/openshift/cluster-api-provider-azure v0.1.0-alpha.3.0.20200120114645-8a9592f1f87b // Pin OpenShift fork
	sigs.k8s.io/cluster-api-provider-openstack => github.com/openshift/cluster-api-provider-openstack v0.0.0-20200526112135-319a35b2e38e // Pin OpenShift fork
)
