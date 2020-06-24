package prom

import (
	"bytes"
	"io"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
)

func TestMatchLabels(t *testing.T) {
	g := NewGomegaWithT(t)

	testCases := []struct {
		title    string
		metric   model.Metric
		matchers map[string]string
		match    bool
	}{
		{
			title: "one label-value pair match",
			metric: map[model.LabelName]model.LabelValue{
				"label1": "value1",
			},
			matchers: map[string]string{
				"label1": "value1",
			},
			match: true,
		},
		{
			title: "multiple label sets match",
			metric: map[model.LabelName]model.LabelValue{
				"label1": "value1",
				"label2": "value2",
			},
			matchers: map[string]string{
				"label1": "value1",
			},
			match: true,
		},
		{
			title: "no matchers",
			metric: map[model.LabelName]model.LabelValue{
				"label1": "value1",
			},
			matchers: map[string]string{},
			match:    true,
		},
		{
			title:  "no metric",
			metric: map[model.LabelName]model.LabelValue{},
			matchers: map[string]string{
				"label1": "value1",
			},
			match: false,
		},
		{
			title: "metric don't have all required matchers",
			metric: map[model.LabelName]model.LabelValue{
				"label1": "value1",
			},
			matchers: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
			match: false,
		},
		{
			title: "same label, different value",
			metric: map[model.LabelName]model.LabelValue{
				"label1": "value1",
			},
			matchers: map[string]string{
				"label1": "foo",
			},
			match: false,
		},
		{
			title: "different label set",
			metric: map[model.LabelName]model.LabelValue{
				"label1": "value1",
			},
			matchers: map[string]string{
				"foo": "bar",
			},
			match: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			matched := matchLabels(tc.metric, tc.matchers)
			g.Expect(matched).Should(Equal(tc.match))
		})
	}
}

func TestDecodeMetrics(t *testing.T) {
	g := NewGomegaWithT(t)

	exampleMetrics := `# HELP go_info Information about the Go environment.
# TYPE go_info gauge
go_info{foo="bar"} 1
go_info{foo="buz"} 0
`

	testCases := []struct {
		title    string
		input    io.Reader
		matchers map[string]string
		hasError bool
		metrics  []string
	}{
		{
			title: "invalid metrics format",
			input: bytes.NewReader([]byte(`123`)),
			matchers: map[string]string{
				"foo": "bar",
			},
			hasError: true,
			metrics:  []string{},
		},
		// This case doesn't throw error, but return empty string slice
		{
			title: "empty input metrics",
			input: bytes.NewReader([]byte("")),
			matchers: map[string]string{
				"foo": "bar",
			},
			hasError: false,
			metrics:  []string{},
		},
		{
			title: "match one metric",
			input: bytes.NewReader([]byte(exampleMetrics)),
			matchers: map[string]string{
				"foo": "bar",
			},
			hasError: false,
			metrics:  []string{`go_info{foo="bar"} => 1.000000`},
		},
		{
			title: "match another metric",
			input: bytes.NewReader([]byte(exampleMetrics)),
			matchers: map[string]string{
				"foo": "buz",
			},
			hasError: false,
			metrics:  []string{`go_info{foo="buz"} => 0.000000`},
		},
		{
			title: "match all metrics with name go_info",
			input: bytes.NewReader([]byte(exampleMetrics)),
			matchers: map[string]string{
				"__name__": "go_info",
			},
			hasError: false,
			metrics: []string{
				`go_info{foo="bar"} => 1.000000`,
				`go_info{foo="buz"} => 0.000000`,
			},
		},
		{
			title: "no match",
			input: bytes.NewReader([]byte(exampleMetrics)),
			matchers: map[string]string{
				"foo": "aaa",
			},
			hasError: false,
			metrics:  []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			metrics, err := DecodeMetrics(tc.input, tc.matchers)
			if !tc.hasError {
				g.Expect(metrics).Should(Equal(tc.metrics))
			} else {
				g.Expect(err).Should(HaveOccurred())
			}
		})
	}
}
