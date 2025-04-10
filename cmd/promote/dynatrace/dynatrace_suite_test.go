package dynatrace

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDynatraceSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test Dynatrace suite")
}
