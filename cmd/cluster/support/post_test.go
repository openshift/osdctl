package support

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

func TestValidateResolutionString(t *testing.T) {
	tests := []struct {
		input         string
		errorExpected bool
	}{
		{"resolution.", true},
		{"no-dot-at-end", false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			err := validateResolutionString(test.input)
			if (err == nil) == test.errorExpected {
				t.Errorf("For input '%s', expected an error: %t, but got: %v", test.input, test.errorExpected, err)
			}
		})
	}
}

func Test_buildLimitedSupport(t *testing.T) {
	tests := []struct {
		name        string
		post        *Post
		wantSummary string
	}{
		{
			name: "Builds a limited support struct for cloud misconfiguration",
			post: &Post{
				Misconfiguration: cloud,
				Problem:          "test problem cloud",
				Resolution:       "test resolution cloud",
			},
			wantSummary: LimitedSupportSummaryCloud,
		},
		{
			name: "Builds a limited support struct for cluster misconfiguration",
			post: &Post{
				Misconfiguration: cluster,
				Problem:          "test problem cluster",
				Resolution:       "test resolution cluster",
			},
			wantSummary: LimitedSupportSummaryCluster,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.post.buildLimitedSupport()
			if err != nil {
				t.Errorf("buildLimitedSupport() error = %v, wantErr %v", err, false)
				return
			}
			if summary := got.Summary(); summary != tt.wantSummary {
				t.Errorf("buildLimitedSupport() got summary = %v, want %v", summary, tt.wantSummary)
			}
			if detectionType := got.DetectionType(); detectionType != cmv1.DetectionTypeManual {
				t.Errorf("buildLimitedSupport() got detectionType = %v, want %v", detectionType, cmv1.DetectionTypeManual)
			}
			if details := got.Details(); details != fmt.Sprintf("%s %s", tt.post.Problem, tt.post.Resolution) {
				t.Errorf("buildLimitedSupport() got details = %s, want %s", details, fmt.Sprintf("%s %s", tt.post.Problem, tt.post.Resolution))
			}
		})
	}
}

func Test_buildInternalServiceLog(t *testing.T) {
	const (
		externalId = "abc-123"
		internalId = "def456"
	)

	type args struct {
		limitedSupportId string
		evidence         string
		subscriptionId   string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Builds a log entry struct with subscription ID",
			args: args{
				limitedSupportId: "test-ls-id",
				evidence:         "this is evidence",
				subscriptionId:   "subid123",
			},
		},
		{
			name: "Builds a log entry struct without subscription ID",
			args: args{
				limitedSupportId: "test-ls-id",
				evidence:         "this is evidence",
				subscriptionId:   "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster, err := cmv1.NewCluster().ExternalID(externalId).ID(internalId).Build()
			if err != nil {
				t.Error(err)
			}

			p := &Post{cluster: cluster, Evidence: tt.args.evidence}

			got, err := p.buildInternalServiceLog(tt.args.limitedSupportId, tt.args.subscriptionId)
			if err != nil {
				t.Errorf("buildInternalServiceLog() error = %v, wantErr %v", err, false)
				return
			}
			if clusterUUID := got.ClusterUUID(); clusterUUID != externalId {
				t.Errorf("buildInternalServiceLog() got clusterUUID = %v, want %v", clusterUUID, externalId)
			}

			if clusterID := got.ClusterID(); clusterID != internalId {
				t.Errorf("buildInternalServiceLog() got clusterUUID = %v, want %v", clusterID, internalId)
			}

			if internalOnly := got.InternalOnly(); internalOnly != true {
				t.Errorf("buildInternalServiceLog() got internalOnly = %v, want %v", internalOnly, true)
			}

			if severity := got.Severity(); severity != InternalServiceLogSeverity {
				t.Errorf("buildInternalServiceLog() got severity = %v, want %v", severity, InternalServiceLogSeverity)
			}

			if serviceName := got.ServiceName(); serviceName != InternalServiceLogServiceName {
				t.Errorf("buildInternalServiceLog() got serviceName = %v, want %v", serviceName, InternalServiceLogServiceName)
			}

			if summary := got.Summary(); summary != InternalServiceLogSummary {
				t.Errorf("buildInternalServiceLog() got summary = %v, want %v", summary, InternalServiceLogSummary)
			}

			if description := got.Description(); description != fmt.Sprintf("%v - %v", tt.args.limitedSupportId, tt.args.evidence) {
				t.Errorf("buildInternalServiceLog() got description = %v, want %v", description, fmt.Sprintf("%v - %v", tt.args.limitedSupportId, tt.args.evidence))
			}

			if subscriptionID := got.SubscriptionID(); subscriptionID != tt.args.subscriptionId {
				t.Errorf("buildInternalServiceLog() got subscriptionID = %v, want %v", subscriptionID, tt.args.subscriptionId)
			}
		})
	}
}

func TestPostInit(t *testing.T) {
	p := Post{}
	p.Init()

	assert.NotNilf(t, userParameterNames, "userParameterNames should not be nil after calling p.Init()")
	assert.Emptyf(t, userParameterNames, "userParameterNames should have 0 elements after calling p.Init()")
	assert.NotNilf(t, userParameterValues, "userParameterValues should not be nil after calling p.Init()")
	assert.Emptyf(t, userParameterValues, "userParameterValues should have 0 elements after calling p.Init()")
}

func TestPostSetupForProblemValues(t *testing.T) {

	tests := map[string]struct {
		name, problem string
		errorExpected bool
	}{
		"Valid problem string not ending in punctuation": {problem: "testProblem", errorExpected: false},
		"Invalid problem string ending in '.'":           {problem: "testProblem.", errorExpected: true},
		"Invalid problem string ending in '?'":           {problem: "testProblem?", errorExpected: true},
		"Invalid problem string ending in '!'":           {problem: "testProblem!", errorExpected: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p := Post{Problem: tt.problem, Resolution: "testResolution"}
			goterr := (p.setup() != nil)
			assert.Equalf(t, goterr, tt.errorExpected, "Error expected: %v, Got Error: %v", tt.errorExpected, goterr)
		})
	}
}
func TestPostSetupForResolutionValues(t *testing.T) {

	tests := map[string]struct {
		problem, resolution string
		errorExpected       bool
	}{
		"Valid resolution string not ending in punctuation": {problem: "testProblem", resolution: "testResolution", errorExpected: false},
		"Invalid resolution string ending in '.'":           {problem: "testProblem", resolution: "testResolution.", errorExpected: true},
		"Invalid resolution string ending in '?'":           {problem: "testProblem", resolution: "testResolution?", errorExpected: true},
		"Invalid resolution string ending in '!'":           {problem: "testProblem", resolution: "testResolution!", errorExpected: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p := Post{Problem: "testProblem", Resolution: tt.resolution}
			goterr := (p.setup() != nil)
			assert.Equalf(t, goterr, tt.errorExpected, "Error expected: %v, Got Error: %v", tt.errorExpected, goterr)
		})
	}
}

func TestPostCheckWhenTemplateEmpty(t *testing.T) {
	tests := []struct {
		name          string
		post          Post
		errorExpected bool
	}{
		{
			name:          "When template is empty, Problem should not empty",
			post:          Post{Resolution: "test Resolution", Misconfiguration: "test Misconfiguration", Evidence: "test Evidence"},
			errorExpected: true,
		},
		{
			name:          "When template is empty, Resolution should not be empty",
			post:          Post{Problem: "test Problem", Misconfiguration: "test Misconfiguration", Evidence: "test Evidence"},
			errorExpected: true,
		},
		{
			name:          "When template is empty, Misconfiguration should not be empty",
			post:          Post{Problem: "test Problem", Resolution: "test Resolution", Evidence: "test Evidence"},
			errorExpected: true,
		},
		{
			name: "When template is empty, Evidence,Misconfiguration,Resolution,Problem are not empty",
			post: Post{Problem: "test Problem", Resolution: "test Resolution", Misconfiguration: "test Misconfiguration", Evidence: "test Evidence"},
		},
		{
			name:          "When template is empty but Resolution is invalid",
			post:          Post{Problem: "test Problem", Resolution: "test Resolution.", Misconfiguration: "test Misconfiguration", Evidence: "test Evidence"},
			errorExpected: true,
		},
		{
			name:          "When template is empty but setup fails",
			post:          Post{Problem: "test Problem.", Resolution: "test Resolution", Misconfiguration: "test Misconfiguration", Evidence: "test Evidence"},
			errorExpected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goterr := (tt.post.check() != nil)
			assert.Equalf(t, goterr, tt.errorExpected, "Error expected: %v, Got Error: %v", tt.errorExpected, goterr)
		})
	}

}
func TestPostCheckWhenTemplateNotEmpty(t *testing.T) {
	tests := []struct {
		name          string
		post          Post
		errorExpected bool
	}{
		{
			name:          "When template is not empty, Problem should be empty",
			post:          Post{Template: "test", Problem: "test Problem"},
			errorExpected: true,
		},
		{
			name:          "When template is not empty, Resolution should be empty",
			post:          Post{Template: "test", Resolution: "test Resolution"},
			errorExpected: true,
		},
		{
			name:          "When template is not empty, Misconfiguration should be empty",
			post:          Post{Template: "test", Misconfiguration: "test Misconfiguration"},
			errorExpected: true,
		},
		{
			name:          "When template is not empty, Evidence should be empty",
			post:          Post{Template: "test", Evidence: "test Evidence"},
			errorExpected: true,
		},
		{
			name: "When template is not empty, Evidence,Misconfiguration,Resolution,Problem are empty",
			post: Post{Template: "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goterr := (tt.post.check() != nil)
			assert.Equalf(t, goterr, tt.errorExpected, "Error expected: %v, Got Error: %v", tt.errorExpected, goterr)
		})
	}
}

func TestPostRunCheckFails(t *testing.T) {
	post := Post{
		Template: "test",
		Problem:  "test",
	}

	err := post.Run("testclusterid")

	assert.NotNilf(t, err, "Should get error in post.check()")
}

func TestPostRunClusterKeyInvalidFails(t *testing.T) {
	post := Post{
		Template: "test",
	}

	err := post.Run("testclusterid?")

	assert.NotNilf(t, err, "Should get error in post.check()")
}

func TestPostBuildLimitedSupportTemplate(t *testing.T) {
	templateFile, err := createTemplateFile(t)

	if err != nil {
		t.Fatal("Could not create temperory Template File")
	}

	post := Post{
		Template:       templateFile,
		TemplateParams: []string{"severity=medium"},
	}

	lsr, err := post.buildLimitedSupportTemplate()
	assert.NotNil(t, lsr)
	assert.Nil(t, err)

}

func TestPostParseUserParameters(t *testing.T) {
	post := Post{
		TemplateParams: []string{"severity=medium"},
	}
	post.parseUserParameters()
	assert.Contains(t, userParameterNames, "${severity}")
	assert.Contains(t, userParameterValues, "medium")
}

func TestPostReadTemplate(t *testing.T) {
	templateFile, err := createTemplateFile(t)

	if err != nil {
		t.Fatal("Could not create temperory Template File")
	}

	post := Post{
		Template:       templateFile,
		TemplateParams: []string{"severity=medium"},
	}

	tf := post.readTemplate()
	assert.NotNil(t, tf)
	assert.Equal(t, tf.Details, "Severity is ${severity}")

}

func TestPostAccessFile_Url(t *testing.T) {
	post := Post{}

	res := "Hello, client"
	want := []byte(res)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, res)
	}))
	defer ts.Close()

	got, err := post.accessFile(ts.URL)
	assert.Nil(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, want, got)

}

func TestPostAccessFile_File(t *testing.T) {
	templateFile, err := createTemplateFile(t)

	if err != nil {
		t.Fatal("Could not create temperory Template File")
	}

	post := Post{
		Template: templateFile,
	}

	b, err := post.accessFile(templateFile)
	assert.Nil(t, err)
	assert.NotNil(t, b)

	b, err = post.accessFile(templateFile + ".invalid")
	assert.Nil(t, b)
	assert.NotNil(t, err)

	dir := t.TempDir()
	b, err = post.accessFile(dir)
	assert.Nil(t, b)
	assert.NotNil(t, err)
}

func TestPrintLimitedSupportReason(t *testing.T) {
	post := Post{
		Misconfiguration: cloud,
		Problem:          "test problem cloud",
		Resolution:       "test resolution cloud",
	}

	limitedSupport, err := post.buildLimitedSupport()
	if assert.NoError(t, err) {
		err = printLimitedSupportReason(limitedSupport)
		assert.NoError(t, err)
	}
}

func createTemplateFile(t *testing.T) (path string, err error) {
	dir := t.TempDir()
	path = filepath.Join(dir, "tempFile")
	content := `{"details": "Severity is ${severity}"}`
	a := make([]byte, 10)
	rand.Read(a)
	err = os.WriteFile(path, []byte(content), 0644)
	return
}
