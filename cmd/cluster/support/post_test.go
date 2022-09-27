package support

import (
	"testing"

	"github.com/openshift/osdctl/internal/support"
)

func TestValidateBadResponse(t *testing.T) {

	//good json format
	gMsg := "{\"field1\":\"data1\",\"field2\":\"data2\"}"

	//bad json format
	bMsg := "{\"field1\":\"data1\",\"field2\"\"data2\"}"

	testCases := []struct {
		title       string
		body        []byte
		errExpected bool
		errReason   string
	}{
		{
			title:       "Validate good Json Response",
			body:        []byte(gMsg),
			errExpected: false,
		},
		{
			title:       "Validate bad Json Response",
			body:        []byte(bMsg),
			errExpected: true,
			errReason:   "Server returned invalid JSON",
		},
	}
	for _, tc := range testCases {
		_, result := validateBadResponse(tc.body)
		if tc.errExpected {
			if result == nil {
				t.Fatalf("Test %s failed. Expected error %s, but got none", tc.title, tc.errReason)
			}
			if result.Error() != tc.errReason {
				t.Fatalf("Test %s failed. Expected error %s, but got %s", tc.title, tc.errReason, result.Error())
			}
		}
		if !tc.errExpected && result != nil {
			t.Fatalf("Test %s failed. Expected no errors, but got %s", tc.title, result.Error())
		}
	}
}

func TestValidateGoodResponse(t *testing.T) {

	//good json format
	gMsg := "{\"field1\":\"data1\",\"field2\":\"data2\"}"
	//bad json format
	bMsg := "{\"field1\":\"data1\",\"field2\"\"data2\"}"

	testCases := []struct {
		title        string
		body         []byte
		lmtSprReason support.LimitedSupport
		errExpected  bool
		errReason    string
	}{
		{
			title:        "Validate good Json Response",
			body:         []byte(gMsg),
			lmtSprReason: support.LimitedSupport{},
			errExpected:  false,
		},
		{
			title:        "Validate bad Json Response",
			body:         []byte(bMsg),
			lmtSprReason: support.LimitedSupport{},
			errExpected:  true,
			errReason:    "Server returned invalid JSON",
		},
	}
	for _, tc := range testCases {
		_, result := validateGoodResponse(tc.body, tc.lmtSprReason)
		if tc.errExpected {
			if result == nil {
				t.Fatalf("Test %s failed. Expected error %s, but got none", tc.title, tc.errReason)
			}
			if result.Error() != tc.errReason {
				t.Fatalf("Test %s failed. Expected error %s, but got %s", tc.title, tc.errReason, result.Error())
			}
		}
		if !tc.errExpected && result != nil {
			t.Fatalf("Test %s failed. Expected no errors, but got %s", tc.title, result.Error())
		}
	}
}
