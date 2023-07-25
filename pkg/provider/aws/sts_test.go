package aws

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"
)

const (
	AwsPartitionID      = "aws"        // AWS Standard partition.
	AwsUsGovPartitionID = "aws-us-gov" // AWS GovCloud (US) partition.
)

type mockSuite struct {
	mockCtrl      *gomock.Controller
	mockAWSClient *mock.MockClient
}

// setupDefaultMocks is an easy way to setup all of the default mocks
func setupDefaultMocks(t *testing.T) *mockSuite {
	mocks := &mockSuite{
		mockCtrl: gomock.NewController(t),
	}

	mocks.mockAWSClient = mock.NewMockClient(mocks.mockCtrl)
	return mocks
}

func TestGetAwsPartition(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		title        string
		setupAWSMock func(r *mock.MockClientMockRecorder)
		errExpected  bool
		expected     string
	}{
		{
			title: "AWS partition",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{
					Arn: awsSdk.String("arn:aws:iam::123456789012:user/username"),
				}, nil)
			},
			errExpected: false,
			expected:    AwsPartitionID,
		},
		{
			title: "AWS GovCloud partition",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{
					Arn: awsSdk.String("arn:aws-us-gov:iam::123456789012:user/username"),
				}, nil)
			},
			errExpected: false,
			expected:    AwsUsGovPartitionID,
		},
		{
			title: "Invalid arn",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{
					Arn: awsSdk.String("hello"),
				}, nil)
			},
			errExpected: true,
			expected:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			tc.setupAWSMock(mocks.mockAWSClient.EXPECT())

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			// after mocks is defined
			defer mocks.mockCtrl.Finish()

			partition, err := GetAwsPartition(mocks.mockAWSClient)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(partition).Should(Equal(tc.expected))
			}
		})
	}
}

func TestGetFederationEndpointUrl(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		title       string
		partition   string
		errExpected bool
	}{
		{
			title:       "AWS partition",
			partition:   AwsPartitionID,
			errExpected: false,
		},
		{
			title:       "Invalid partition",
			partition:   "hello",
			errExpected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			_, err := GetFederationEndpointUrl(tc.partition)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			}
		})
	}
}

func TestGetConsoleUrl(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		title       string
		partition   string
		errExpected bool
	}{
		{
			title:       "AWS GovCloud partition",
			partition:   AwsUsGovPartitionID,
			errExpected: false,
		},
		{
			title:       "Invalid partition",
			partition:   "hello",
			errExpected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			_, err := GetConsoleUrl(tc.partition)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			}
		})
	}
}

func TestGetAssumeRoleCredentials(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title           string
		setupAWSMock    func(r *mock.MockClientMockRecorder)
		duration        int64
		roleSessionName string
		roleArn         string
		credentials     *types.Credentials
		errExpected     bool
	}{
		{
			title: "Error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.AssumeRole(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1)
			},
			credentials:     nil,
			duration:        0,
			roleSessionName: "",
			roleArn:         "",
			errExpected:     true,
		},
		{
			title: "No output and no error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.AssumeRole(gomock.Any()).Return(nil, nil).Times(1)
			},
			credentials:     nil,
			duration:        0,
			roleSessionName: "",
			roleArn:         "",
			errExpected:     true,
		},
		{
			title: "Normal",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.AssumeRole(gomock.Any()).Return(&sts.AssumeRoleOutput{
					Credentials: &types.Credentials{
						AccessKeyId:     awsSdk.String("foo/bar"),
						SecretAccessKey: awsSdk.String("foo/bar"),
					},
				}, nil).Times(1)
			},
			credentials: &types.Credentials{
				AccessKeyId:     awsSdk.String("foo/bar"),
				SecretAccessKey: awsSdk.String("foo/bar"),
			},
			roleSessionName: "foo",
			roleArn:         "bar",
			errExpected:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			tc.setupAWSMock(mocks.mockAWSClient.EXPECT())

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			// after mocks is defined
			defer mocks.mockCtrl.Finish()

			creds, err := GetAssumeRoleCredentials(mocks.mockAWSClient, awsSdk.Int32(0), &tc.roleSessionName, &tc.roleArn)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(creds).Should(Equal(tc.credentials))
			}
		})
	}
}

func TestFormatSignInURL(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title       string
		partition   string
		signInToken string
		base        string
	}{
		{
			title:       "signInToken foo",
			partition:   AwsPartitionID,
			signInToken: "foo",
			base:        "https://signin.aws.amazon.com/federation?Action=login&Destination=https%3A%2F%2Fconsole.aws.amazon.com%2F&Issuer=Red+Hat+SRE&SigninToken=",
		},
		{
			title:       "signInToken bar",
			partition:   AwsUsGovPartitionID,
			signInToken: "bar",
			base:        "https://signin.amazonaws-us-gov.com/federation?Action=login&Destination=https%3A%2F%2Fconsole.amazonaws-us-gov.com%2F&Issuer=Red+Hat+SRE&SigninToken=",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			u, err := formatSignInURL(tc.partition, tc.signInToken)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(u.String()).Should(Equal(tc.base + tc.signInToken))
		})
	}
}

func TestGetSignInToken(t *testing.T) {
	g := NewGomegaWithT(t)

	testCreds := &types.Credentials{
		AccessKeyId:     awsSdk.String("foo"),
		SecretAccessKey: awsSdk.String("bar"),
		SessionToken:    awsSdk.String("buz"),
	}

	testCases := []struct {
		title       string
		handler     func(w http.ResponseWriter, r *http.Request)
		creds       *types.Credentials
		token       string
		errExpected bool
		errContent  string
	}{
		{
			title: "Server returns 200 but empty body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			},
			creds:       testCreds,
			token:       "",
			errExpected: true,
			errContent:  "unexpected end of JSON input",
		},
		{
			title: "Server returns 404",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(404)
			},
			creds:       testCreds,
			token:       "",
			errExpected: true,
			errContent:  "bad response code 404",
		},
		{
			title: "Server returns 500",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(500)
			},
			creds:       testCreds,
			token:       "",
			errExpected: true,
			errContent:  "bad response code 500",
		},
		{
			title: "Server returns 200 but bad body format",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.Write([]byte("foo")) //#nosec G104 -- Errors unhandled, just a test
			},
			creds:       testCreds,
			token:       "",
			errExpected: true,
			errContent:  "invalid character 'o' in literal false (expecting 'a')",
		},
		{
			title: "Success",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				session := r.FormValue("Session")
				var creds sessionPayload
				_ = json.Unmarshal([]byte(session), &creds)
				w.WriteHeader(200)

				resp := awsSignInTokenResponse{SigninToken: creds.SessionID + creds.SessionKey + creds.SessionToken}
				data, _ := json.Marshal(resp)
				w.Write(data) //#nosec G104 -- Errors unhandled, just a test
			},
			creds:       testCreds,
			token:       "foobarbuz",
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tc.handler))
			token, err := getSignInToken(srv.URL, tc.creds)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
				g.Expect(err.Error()).Should(Equal(tc.errContent))
			} else {
				g.Expect(token).Should(Equal(tc.token))
			}
		})
	}
}

func TestRequestSignedURL(t *testing.T) {
	g := NewGomegaWithT(t)

	testCreds := &types.Credentials{
		AccessKeyId:     awsSdk.String("foo"),
		SecretAccessKey: awsSdk.String("bar"),
		SessionToken:    awsSdk.String("buz"),
	}

	testCases := []struct {
		title       string
		handler     func(w http.ResponseWriter, r *http.Request)
		creds       *types.Credentials
		token       string
		errExpected bool
		errContent  string
	}{
		{
			title: "Server returns 200 but empty body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			},
			creds:       testCreds,
			token:       "",
			errExpected: true,
			errContent:  "unexpected end of JSON input",
		},
		{
			title: "Server returns 404",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(404)
			},
			creds:       testCreds,
			token:       "",
			errExpected: true,
			errContent:  "bad response code 404",
		},
		{
			title: "Server returns 500",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(500)
			},
			creds:       testCreds,
			token:       "",
			errExpected: true,
			errContent:  "bad response code 500",
		},
		{
			title: "Server returns 200 but bad body format",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.Write([]byte("foo")) //#nosec G104 -- Errors unhandled, just a test
			},
			creds:       testCreds,
			token:       "",
			errExpected: true,
			errContent:  "invalid character 'o' in literal false (expecting 'a')",
		},
		{
			title: "Success",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				session := r.FormValue("Session")
				var creds types.Credentials
				_ = json.Unmarshal([]byte(session), &creds)
				w.WriteHeader(200)

				resp := awsSignInTokenResponse{SigninToken: *creds.AccessKeyId + *creds.SecretAccessKey + *creds.SessionToken}
				data, _ := json.Marshal(resp)
				w.Write(data) //#nosec G104 -- just a test file
			},
			creds:       testCreds,
			token:       "foobarbuz",
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tc.handler))
			data, err := json.Marshal(tc.creds)
			g.Expect(err).ShouldNot(HaveOccurred())
			token, err := requestSignedURL(srv.URL, data)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
				g.Expect(err.Error()).Should(Equal(tc.errContent))
			} else {
				g.Expect(token).Should(Equal(tc.token))
			}
		})
	}
}
