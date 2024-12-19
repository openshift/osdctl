package servicelog

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/osdctl/internal/servicelog"
)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Setup Suite")
}

var _ = Describe("Test posting service logs", func() {
	var options *PostCmdOptions

	BeforeEach(func() {
		options = &PostCmdOptions{
			Overrides: []string{
				"description=new description",
				"summary=new summary",
			},
			Message: servicelog.Message{
				Summary:      "The original summary",
				InternalOnly: false,
			},
		}
	})

	Context("overriding a field", func() {
		It("overrides string fields successfully", func() {
			overrideString := "Overridden Summary"
			err := options.overrideField("summary", overrideString)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(options.Message.Summary).To(Equal(overrideString))
		})

		It("overrides bool fields correctly", func() {
			Expect(options.Message.InternalOnly).ToNot(Equal(true))

			err := options.overrideField("internal_only", "true")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(options.Message.InternalOnly).To(Equal(true))
		})

		It("errors when overriding a field that does not exist", func() {
			err := options.overrideField("does_not_exist", "")

			Expect(err).Should(HaveOccurred())
		})

		It("errors when overriding a bool with an unparsable string", func() {
			err := options.overrideField("internal_only", "ThisIsNotABool")

			Expect(err).Should(HaveOccurred())
		})

		It("errors when overriding an unsupported data type", func() {
			err := options.overrideField("doc_references", "DoesntMatter")

			Expect(err).Should(HaveOccurred())
		})
	})

	Context("parsing overrides", func() {
		It("parses correctly", func() {
			overrideMap, err := options.parseOverrides()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(overrideMap).To(HaveKey("description"))
			Expect(overrideMap["description"]).To(Equal("new description"))
			Expect(overrideMap).To(HaveKey("summary"))
			Expect(overrideMap["summary"]).To(Equal("new summary"))
		})

		It("fails when an option contains no equals sign", func() {
			options.Overrides = []string{
				"THISDOESNOTHAVEANEQUALS",
			}

			_, err := options.parseOverrides()

			Expect(err).Should(HaveOccurred())
		})

		It("fails when an option has no key", func() {
			options.Overrides = []string{
				"=VALUE",
			}

			_, err := options.parseOverrides()

			Expect(err).Should(HaveOccurred())
		})

		It("fails when an option has no value", func() {
			options.Overrides = []string{
				"KEY=",
			}

			_, err := options.parseOverrides()

			Expect(err).Should(HaveOccurred())
		})
	})

	Context("full override parsing", func() {
		It("correctly applies valid rules", func() {
			overrideMap := map[string]string{
				"internal_only": "true",
				"summary":       "Test Summary",
			}

			err := options.applyOverrides(overrideMap)

			Expect(err).ShouldNot(HaveOccurred())
		})

		It("gives an error when there is an invalid rule", func() {
			overrideMap := map[string]string{
				"internal_only": "notaboolean",
			}

			err := options.applyOverrides(overrideMap)

			Expect(err).Should(HaveOccurred())
		})
	})
})
