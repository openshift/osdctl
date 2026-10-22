package mustgather

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift-online/ocm-sdk-go/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type setupFunc func() (*rest.Config, *kubernetes.Clientset, error)

func okSetup() (*rest.Config, *kubernetes.Clientset, error) {
	return &rest.Config{}, nil, nil
}

func failSetup(err error) setupFunc {
	return func() (*rest.Config, *kubernetes.Clientset, error) { return nil, nil, err }
}

// newTestMustGather returns a mustGather whose external side effects (the oc exec and the RHOBS
// export) are replaced by the provided fakes, so outcomes can be driven without a cluster.
func newTestMustGather(runErr, logsErr error) *mustGather {
	return &mustGather{
		reason:             "test",
		acmMustGatherImage: "test-image",
		hiveOcmUrl:         "production",
		runMustGather: func(*rest.Config, *kubernetes.Clientset, []string) error {
			return runErr
		},
		gatherHCPLogs: func(context.Context, string, string, string, []string, time.Duration, int) (int, error) {
			return 0, logsErr
		},
	}
}

func mockOCMConn(t *testing.T, routes map[string]string) *sdk.Connection {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range routes {
		body := body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ": "Bearer",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
	})
	signed, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	logger, err := logging.NewGoLoggerBuilder().Debug(false).Build()
	require.NoError(t, err)

	conn, err := sdk.NewConnectionBuilder().Logger(logger).Tokens(signed).URL(server.URL).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func TestRunTarget_InfraTargets(t *testing.T) {
	cluster := mustCluster(t, "mc-id", false)

	tests := []struct {
		name        string
		target      string
		getMC       setupFunc
		getSC       setupFunc
		runErr      error
		wantStatus  gatherStatus
		wantFailure string
	}{
		{name: "sc success", target: "sc", getMC: okSetup, getSC: okSetup, wantStatus: statusSucceeded},
		{name: "sc resolve fails", target: "sc", getMC: okSetup, getSC: failSetup(errors.New("boom")), wantStatus: statusFailed, wantFailure: "resolve cluster"},
		{name: "sc gather fails", target: "sc", getMC: okSetup, getSC: okSetup, runErr: errors.New("oc failed"), wantStatus: statusFailed, wantFailure: "must-gather"},
		{name: "sc_acm success", target: "sc_acm", getMC: okSetup, getSC: okSetup, wantStatus: statusSucceeded},
		{name: "mc success", target: "mc", getMC: okSetup, getSC: okSetup, wantStatus: statusSucceeded},
		{name: "mc resolve fails", target: "mc", getMC: failSetup(errors.New("boom")), getSC: okSetup, wantStatus: statusFailed, wantFailure: "resolve cluster"},
		{name: "unknown target", target: "bogus", getMC: okSetup, getSC: okSetup, wantStatus: statusFailed, wantFailure: "unknown gather target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mg := newTestMustGather(tt.runErr, nil)
			// ocmClient is unused for non-hcp targets.
			res := mg.runTarget(tt.target, t.TempDir(), cluster, nil, tt.getMC, tt.getSC)

			assert.Equal(t, tt.wantStatus, res.status)
			if tt.wantFailure != "" {
				require.NotEmpty(t, res.failures)
				assert.Contains(t, res.failures[0], tt.wantFailure)
			} else {
				assert.Empty(t, res.failures)
			}
		})
	}
}

func mustHCPCluster(t *testing.T, id, domainPrefix string) *cmv1.Cluster {
	t.Helper()
	cluster, err := cmv1.NewCluster().
		ID(id).
		DomainPrefix(domainPrefix).
		Hypershift(cmv1.NewHypershift().Enabled(true)).
		Build()
	require.NoError(t, err)
	return cluster
}

func TestRunTarget_HCP(t *testing.T) {
	const clusterID = "hcp-id"
	hypershiftPath := "/api/clusters_mgmt/v1/clusters/" + clusterID + "/hypershift"
	withNamespace := map[string]string{hypershiftPath: `{"hcp_namespace":"ocm-stg-hcp-id-myprefix"}`}
	withoutNamespace := map[string]string{hypershiftPath: `{}`}

	tests := []struct {
		name          string
		routes        map[string]string
		getMC         setupFunc
		logsErr       error
		dumpErr       error
		wantStatus    gatherStatus
		wantFailures  []string
		wantNoFailure bool
	}{
		{name: "both substeps succeed", routes: withNamespace, getMC: okSetup, wantStatus: statusSucceeded, wantNoFailure: true},
		{name: "logs fail, dump ok", routes: withNamespace, getMC: okSetup, logsErr: errors.New("rhobs down"), wantStatus: statusPartial, wantFailures: []string{"RHOBS log export"}},
		{name: "logs ok, MC resolve fails", routes: withNamespace, getMC: failSetup(errors.New("mc down")), wantStatus: statusPartial, wantFailures: []string{"resolve MC for ACM dump"}},
		{name: "logs ok, dump fails", routes: withNamespace, getMC: okSetup, dumpErr: errors.New("oc boom"), wantStatus: statusPartial, wantFailures: []string{"ACM hypershift dump"}},
		{name: "both substeps fail", routes: withNamespace, getMC: okSetup, logsErr: errors.New("rhobs down"), dumpErr: errors.New("oc boom"), wantStatus: statusFailed, wantFailures: []string{"RHOBS log export", "ACM hypershift dump"}},
		{name: "missing HCP namespace", routes: withoutNamespace, getMC: okSetup, wantStatus: statusFailed, wantFailures: []string{"get HCP namespace"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := mustHCPCluster(t, clusterID, "myprefix")
			conn := mockOCMConn(t, tt.routes)
			mg := newTestMustGather(tt.dumpErr, tt.logsErr)
			mg.clusterId = clusterID

			res := mg.runTarget("hcp", t.TempDir(), cluster, conn, tt.getMC, okSetup)

			assert.Equal(t, tt.wantStatus, res.status)
			if tt.wantNoFailure {
				assert.Empty(t, res.failures)
			}
			for _, want := range tt.wantFailures {
				assert.Truef(t, hasFailureContaining(res.failures, want),
					"expected a failure containing %q, got %v", want, res.failures)
			}
		})
	}
}

func hasFailureContaining(failures []string, want string) bool {
	for _, failure := range failures {
		if strings.Contains(failure, want) {
			return true
		}
	}
	return false
}
