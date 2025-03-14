package network

import (
	"fmt"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var _ = ginkgo.Describe("Network Command", func() {

	var (
		streams genericclioptions.IOStreams
		client  *k8s.LazyClient
		cmd     *cobra.Command
	)

	ginkgo.BeforeEach(func() {
		client = &k8s.LazyClient{}
		streams = genericclioptions.IOStreams{}
		cmd = NewCmdNetwork(streams, client)
	})

	ginkgo.Context("When running the network command", func() {

		ginkgo.It("should create the network command", func() {
			gomega.Expect(cmd.Use).To(gomega.Equal("network"))
			gomega.Expect(cmd.Short).To(gomega.Equal("network related utilities"))
			gomega.Expect(len(cmd.Commands())).To(gomega.BeNumerically(">", 0))
		})

		ginkgo.It("should add the correct subcommands", func() {
			gomega.Expect(len(cmd.Commands())).To(gomega.BeNumerically(">", 1))
			subCmd := cmd.Commands()[0]
			gomega.Expect(subCmd.Use).To(gomega.Equal("packet-capture"))
		})

		ginkgo.It("should execute the network command without errors", func() {
			cmd.SetArgs([]string{})
			err := cmd.Execute()
			gomega.Expect(err).To(gomega.BeNil())
		})
	})

	ginkgo.Context("When invoking the help command", func() {

		ginkgo.It("should call cmd.Help() without errors", func() {
			help(cmd, nil)
		})

		ginkgo.It("should handle cmd.Help() errors", func() {
			mockCmd := &cobra.Command{
				Use: "mock",
				RunE: func(cmd *cobra.Command, args []string) error {
					return fmt.Errorf("mock error")
				},
			}

			help(mockCmd, nil)
		})
	})
})

func TestNetworkCmd(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Network Command Suite")
}
