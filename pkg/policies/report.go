package policies

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// FormatTable writes a human-readable table of simulation results.
func (r *SimulationReport) FormatTable(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)

	fmt.Fprintf(tw, "COMPONENT\tACTION\tRESOURCE\tCONTEXT\tEXPECTED\tDECISION\tRESULT\n")
	fmt.Fprintf(tw, "---------\t------\t--------\t-------\t--------\t--------\t------\n")

	for _, res := range r.Results {
		resource := res.Resource
		if len(resource) > 40 {
			resource = "..." + resource[len(resource)-37:]
		}

		context := res.ContextDesc
		if context == "" {
			context = "(none)"
		}
		if len(context) > 40 {
			context = context[:37] + "..."
		}

		status := "PASS"
		if !res.Pass {
			status = "FAIL"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			res.Component, res.Action, resource, context,
			res.Expected, res.Decision, status)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nSUMMARY: %d/%d passed, %d FAILED\n", r.Passed, r.Total, r.Failed)

	if r.Failed > 0 {
		fmt.Fprintf(w, "\nFAILED ACTIONS:\n")
		for _, res := range r.Results {
			if !res.Pass {
				fmt.Fprintf(w, "  - %s on %s\n", res.Action, res.Resource)
				if res.ContextDesc != "" {
					fmt.Fprintf(w, "    Context: %s\n", res.ContextDesc)
				}
				fmt.Fprintf(w, "    Expected: %s, Got: %s\n", res.Expected, res.Decision)
				if len(res.MissingContext) > 0 {
					fmt.Fprintf(w, "    Missing context keys: %s\n", strings.Join(res.MissingContext, ", "))
				}
			}
		}
	}
}

// FormatJSON writes simulation results as JSON.
func (r *SimulationReport) FormatJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

// JUnitTestSuites is the top-level JUnit XML structure.
type JUnitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents a single test suite in JUnit XML.
type JUnitTestSuite struct {
	XMLName  xml.Name        `xml:"testsuite"`
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Cases    []JUnitTestCase `xml:"testcase"`
}

// JUnitTestCase represents a single test case in JUnit XML.
type JUnitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Failure   *JUnitFailure `xml:"failure,omitempty"`
}

// JUnitFailure represents a test failure in JUnit XML.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// FormatJUnitXML writes simulation results as JUnit XML for CI integration.
func (r *SimulationReport) FormatJUnitXML(w io.Writer) error {
	suite := JUnitTestSuite{
		Name:     r.PolicyName,
		Tests:    r.Total,
		Failures: r.Failed,
	}

	for _, res := range r.Results {
		tc := JUnitTestCase{
			Name:      fmt.Sprintf("%s - %s", res.Action, res.ContextDesc),
			ClassName: res.Component,
		}

		if !res.Pass {
			body := fmt.Sprintf("Action: %s\nResource: %s\nExpected: %s\nGot: %s",
				res.Action, res.Resource, res.Expected, res.Decision)
			if len(res.MissingContext) > 0 {
				body += fmt.Sprintf("\nMissing context keys: %s", strings.Join(res.MissingContext, ", "))
			}

			tc.Failure = &JUnitFailure{
				Message: fmt.Sprintf("Expected %s but got %s", res.Expected, res.Decision),
				Type:    "PolicyMismatch",
				Body:    body,
			}
		}

		suite.Cases = append(suite.Cases, tc)
	}

	suites := JUnitTestSuites{
		Suites: []JUnitTestSuite{suite},
	}

	fmt.Fprint(w, xml.Header)
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	return encoder.Encode(suites)
}

// MergeReports combines multiple simulation reports into one.
func MergeReports(reports ...*SimulationReport) *SimulationReport {
	merged := &SimulationReport{
		PolicyName: "combined",
	}

	for _, r := range reports {
		merged.Results = append(merged.Results, r.Results...)
		merged.Passed += r.Passed
		merged.Failed += r.Failed
		merged.Total += r.Total
	}

	return merged
}
