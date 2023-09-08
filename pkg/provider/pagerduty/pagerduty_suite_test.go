package pagerduty_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPagerduty(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pagerduty Suite")
}
