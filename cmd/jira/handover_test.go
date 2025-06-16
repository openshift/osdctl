package jira

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMapProducts(t *testing.T) {
	input := []string{"Product A", "Product B"}
	expected := []map[string]string{
		{"value": "Product A"},
		{"value": "Product B"},
	}
	result := mapProducts(input)
	assert.Equal(t, expected, result)
}

func TestContainsIgnoreCase(t *testing.T) {
	list := []string{"OpenShift Dedicated",
		"OpenShift Dedicated on AWS",
		"OpenShift Dedicated on GCP",
		"Red Hat Openshift on AWS",
		"Red Hat Openshift on AWS with Hosted Control Planes",
	}
	assert.True(t, containsIgnoreCase(list, "OpenShift Dedicated"))
	assert.True(t, containsIgnoreCase(list, "red hat openshift on aws"))
	assert.False(t, containsIgnoreCase(list, "EKS"))
}

func TestGetFieldInput_FromFlag(t *testing.T) {
	viper.Set("customer", "ACME Corp")
	val := promptInput("customer", "Enter Customer Name:")
	assert.Equal(t, "ACME Corp", val)
	viper.Set("customer", "")
}

func TestGetProducts_FromFlag(t *testing.T) {
	viper.Set("products", "OpenShift Dedicated on GCP")
	products, _ := getProducts()
	assert.ElementsMatch(t, []string{"OpenShift Dedicated on GCP"}, products)
	viper.Set("products", "")
}

func TestGetProducts_InvalidProduct(t *testing.T) {
	viper.Set("products", "InvalidProduct")
	_, err := getProducts()
	assert.Error(t, err)
	viper.Set("products", "")
}

func TestGetProducts_TrimmedAndDeduplicated(t *testing.T) {
	viper.Set("products", "  OpenShift Dedicated, OpenShift Dedicated on GCP ")
	products, _ := getProducts()
	assert.ElementsMatch(t, []string{"OpenShift Dedicated", "OpenShift Dedicated on GCP"}, products)
	viper.Set("products", "")
}

func TestMapProducts_EmptyInput(t *testing.T) {
	result := mapProducts([]string{})
	assert.Empty(t, result)
}

func TestContainsIgnoreCase_EmptyList(t *testing.T) {
	assert.False(t, containsIgnoreCase([]string{}, "Anything"))
}
