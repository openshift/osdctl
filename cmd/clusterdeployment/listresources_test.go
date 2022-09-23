package clusterdeployment_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	. "github.com/openshift/osdctl/cmd/clusterdeployment"
	mockk8s "github.com/openshift/osdctl/cmd/clusterdeployment/mock/k8s"
	mockprinter "github.com/openshift/osdctl/cmd/clusterdeployment/mock/printer"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveaws "github.com/openshift/hive/apis/hive/v1/aws"
)

type resources struct {
	accountClaims awsv1alpha1.AccountClaimList
	accounts      awsv1alpha1.AccountList
	cd            hivev1.ClusterDeployment
	l             ListResources

	mockPrinter *mockprinter.MockPrinter
	mockClient  *mockk8s.MockClient
	mockCtrl    *gomock.Controller
	g           *GomegaWithT
}

func setupResources(t *testing.T) *resources {
	var r resources

	r.accountClaims = awsv1alpha1.AccountClaimList{
		Items: []awsv1alpha1.AccountClaim{
			{
				TypeMeta: v1.TypeMeta{
					Kind:       "AccountClaim",
					APIVersion: "aws.managed.openshift.io/v1alpha1",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:      "fake-account-claim",
					Namespace: "fake-cluster-namespace",
				},
				Spec: awsv1alpha1.AccountClaimSpec{
					AccountLink: "fake-account",
				},
			},
		},
	}

	r.accounts = awsv1alpha1.AccountList{
		Items: []awsv1alpha1.Account{
			{
				TypeMeta: v1.TypeMeta{
					Kind:       "Account",
					APIVersion: "aws.managed.openshift.io/v1alpha1",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:      "fake-account",
					Namespace: "aws-account-operator",
				},
				Spec: awsv1alpha1.AccountSpec{
					IAMUserSecret: "fake-secret",
				},
			},
		},
	}

	r.cd = hivev1.ClusterDeployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "fake-cluster",
			Namespace: "fake-cluster-namespace",
		},
		TypeMeta: v1.TypeMeta{
			Kind:       "ClusterDeployment",
			APIVersion: "hive.openshift.io/v1",
		},
		Spec: hivev1.ClusterDeploymentSpec{
			Platform: hivev1.Platform{
				AWS: &hiveaws.Platform{},
			},
		},
	}

	r.g = NewGomegaWithT(t)
	r.mockCtrl = gomock.NewController(t)
	r.mockPrinter = mockprinter.NewMockPrinter(r.mockCtrl)
	r.mockClient = mockk8s.NewMockClient(r.mockCtrl)

	r.l = ListResources{
		ExternalResourcesOnly: false,
		Cmd:                   &cobra.Command{},
		P:                     r.mockPrinter,
		ClusterId:             "fake-id",
		ClusterDeployment:     r.cd,
		KubeCli:               r.mockClient,
	}

	return &r
}

func (r *resources) finish() {
	r.mockCtrl.Finish()
}

func (r *resources) externalOnly() *resources {
	r.l.ExternalResourcesOnly = true
	return r
}

func TestListResourcesAwsExternalOnly(t *testing.T) {

	r := setupResources(t).externalOnly()
	g := r.g

	gomock.InOrder(
		r.mockPrinter.EXPECT().AddRow(gomock.Any()), //title row
		r.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, r.accountClaims),
		r.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, r.accounts),
		r.mockPrinter.EXPECT().AddRow([]string{"aws.managed.openshift.io", "v1alpha1", "Account", "aws-account-operator", "fake-account"}),
		r.mockPrinter.EXPECT().AddRow([]string{`""`, "v1", "Secret", "aws-account-operator", "fake-secret"}),
		r.mockPrinter.EXPECT().Flush(),
	)
	err := r.l.RunListResources()

	g.Expect(err).NotTo(HaveOccurred())
	r.finish()
}

func TestListResourcesAws(t *testing.T) {

	r := setupResources(t)
	g := r.g

	gomock.InOrder(
		r.mockPrinter.EXPECT().AddRow(gomock.Any()), //title row
		r.mockPrinter.EXPECT().AddRow([]string{"hive.openshift.io", "v1", "ClusterDeployment", r.cd.Namespace, r.cd.Name}),
		r.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, r.accountClaims),
		r.mockPrinter.EXPECT().AddRow([]string{"aws.managed.openshift.io", "v1alpha1", "AccountClaim", r.cd.Namespace, "fake-account-claim"}),
		r.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, r.accounts),
		r.mockPrinter.EXPECT().AddRow([]string{"aws.managed.openshift.io", "v1alpha1", "Account", "aws-account-operator", "fake-account"}),
		r.mockPrinter.EXPECT().AddRow([]string{`""`, "v1", "Secret", "aws-account-operator", "fake-secret"}),
		r.mockPrinter.EXPECT().Flush(),
	)
	err := r.l.RunListResources()

	g.Expect(err).NotTo(HaveOccurred())
	r.finish()
}
