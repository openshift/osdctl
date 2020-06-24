package aws

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/golang/mock/gomock"
	"github.com/openshift/osd-utils-cli/pkg/provider/aws/mock"
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

func TestGetAssumeRoleCredentials(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title           string
		setupAWSMock    func(r *mock.MockClientMockRecorder)
		duration        int64
		roleSessionName string
		roleArn         string
		credentials     *sts.Credentials
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
					Credentials: &sts.Credentials{
						AccessKeyId:     aws.String("foo/bar"),
						SecretAccessKey: aws.String("foo/bar"),
					},
				}, nil).Times(1)
			},
			credentials: &sts.Credentials{
				AccessKeyId:     aws.String("foo/bar"),
				SecretAccessKey: aws.String("foo/bar"),
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

			creds, err := getAssumeRoleCredentials(mocks.mockAWSClient, aws.Int64(0), &tc.roleSessionName, &tc.roleArn)
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
	base := "https://signin.aws.amazon.com/federation?Action=login&Destination=https%3A%2F%2Fconsole.aws.amazon.com%2F&Issuer=Red+Hat+SRE&SigninToken="
	testCases := []struct {
		title       string
		signInToken string
		output      string
	}{
		{
			title:       "signInToken foo",
			signInToken: "foo",
			output:      base + "foo",
		},
		{
			title:       "signInToken bar",
			signInToken: "bar",
			output:      base + "bar",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			u, err := formatSignInURL(tc.signInToken)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(u.String()).Should(Equal(tc.output))
		})
	}
}

func TestGetSignInToken(t *testing.T) {
	g := NewGomegaWithT(t)

	testCreds := &sts.Credentials{
		AccessKeyId:     aws.String("foo"),
		SecretAccessKey: aws.String("bar"),
		SessionToken:    aws.String("buz"),
	}

	testCases := []struct {
		title       string
		handler     func(w http.ResponseWriter, r *http.Request)
		creds       *sts.Credentials
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
				w.Write([]byte("foo"))
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
				w.Write(data)
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

	testCreds := &sts.Credentials{
		AccessKeyId:     aws.String("foo"),
		SecretAccessKey: aws.String("bar"),
		SessionToken:    aws.String("buz"),
	}

	testCases := []struct {
		title       string
		handler     func(w http.ResponseWriter, r *http.Request)
		creds       *sts.Credentials
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
				w.Write([]byte("foo"))
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
				var creds sts.Credentials
				_ = json.Unmarshal([]byte(session), &creds)
				w.WriteHeader(200)

				resp := awsSignInTokenResponse{SigninToken: *creds.AccessKeyId + *creds.SecretAccessKey + *creds.SessionToken}
				data, _ := json.Marshal(resp)
				w.Write(data)
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
