package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift-online/ocm-sdk-go/logging"
)

const (
	hypershiftPath = "/api/clusters_mgmt/v1/clusters/hcp-id/hypershift"
	clustersPath   = "/api/clusters_mgmt/v1/clusters"
	fleetMgmtPath  = "/api/osd_fleet_mgmt/v1/management_clusters"
)

// mockOCMServer builds an httptest server whose routes return the given JSON bodies, keyed
// by request path. A route is only registered when a body is supplied.
func mockOCMServer(t *testing.T, bodies map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range bodies {
		body := body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
	return httptest.NewServer(mux)
}

// newMockOCMConn builds a *sdk.Connection pointed at the given server, using a short-lived,
// self-signed Bearer token so the SDK can parse an expiry without attempting a refresh.
func newMockOCMConn(t *testing.T, serverURL string) *sdk.Connection {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ": "Bearer",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
	})
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	logger, err := logging.NewGoLoggerBuilder().Debug(false).Build()
	if err != nil {
		t.Fatalf("failed to build logger: %v", err)
	}

	conn, err := sdk.NewConnectionBuilder().
		Logger(logger).
		Tokens(signed).
		URL(serverURL).
		Build()
	if err != nil {
		t.Fatalf("failed to build connection: %v", err)
	}
	return conn
}

// assertConnOpen proves the connection was not closed by the function under test: a
// caller-owned connection must remain usable after the helper returns. Every test registers
// the hypershift route, so this trivially succeeds unless the connection was closed.
func assertConnOpen(t *testing.T, conn *sdk.Connection) {
	t.Helper()
	if _, err := conn.ClustersMgmt().V1().Clusters().Cluster("hcp-id").Hypershift().Get().Send(); err != nil {
		t.Fatalf("caller-owned connection must stay open, but a follow-up request failed: %v", err)
	}
}

// Note: TestGetManagementClusterWithConn lives in ocm_test.go. This file covers
// GetServiceClusterWithConn (no coverage elsewhere) and the borrowed-connection contract.
func TestGetServiceClusterWithConn(t *testing.T) {
	t.Run("resolves the service cluster", func(t *testing.T) {
		server := mockOCMServer(t, map[string]string{
			hypershiftPath: `{"management_cluster":"mc-name"}`,
			fleetMgmtPath:  `{"kind":"ManagementClusterList","page":1,"size":1,"total":1,"items":[{"kind":"ManagementCluster","id":"mc-id","name":"mc-name","parent":{"kind":"ServiceCluster","name":"sc-name"}}]}`,
			clustersPath:   `{"kind":"ClusterList","page":1,"size":1,"total":1,"items":[{"kind":"Cluster","id":"sc-id","name":"sc-name"}]}`,
		})
		defer server.Close()
		conn := newMockOCMConn(t, server.URL)
		defer conn.Close()

		sc, err := GetServiceClusterWithConn(context.Background(), "hcp-id", conn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sc.ID() != "sc-id" {
			t.Errorf("expected sc-id, got %q", sc.ID())
		}
		assertConnOpen(t, conn)
	})

	t.Run("missing management cluster yields a consistent checked error", func(t *testing.T) {
		server := mockOCMServer(t, map[string]string{hypershiftPath: `{}`})
		defer server.Close()
		conn := newMockOCMConn(t, server.URL)
		defer conn.Close()

		_, err := GetServiceClusterWithConn(context.Background(), "hcp-id", conn)
		if err == nil || !strings.Contains(err.Error(), "no management cluster found for hcp-id") {
			t.Fatalf("expected 'no management cluster found' error, got: %v", err)
		}
	})

	t.Run("empty fleet management result yields an error", func(t *testing.T) {
		server := mockOCMServer(t, map[string]string{
			hypershiftPath: `{"management_cluster":"mc-name"}`,
			fleetMgmtPath:  `{"kind":"ManagementClusterList","page":1,"size":0,"total":0,"items":[]}`,
		})
		defer server.Close()
		conn := newMockOCMConn(t, server.URL)
		defer conn.Close()

		_, err := GetServiceClusterWithConn(context.Background(), "hcp-id", conn)
		if err == nil || !strings.Contains(err.Error(), "no fleet management record found") {
			t.Fatalf("expected 'no fleet management record found' error, got: %v", err)
		}
	})

	t.Run("non-service-cluster parent yields an error", func(t *testing.T) {
		server := mockOCMServer(t, map[string]string{
			hypershiftPath: `{"management_cluster":"mc-name"}`,
			fleetMgmtPath:  `{"kind":"ManagementClusterList","page":1,"size":1,"total":1,"items":[{"kind":"ManagementCluster","id":"mc-id","name":"mc-name","parent":{"kind":"Hive","name":"hive-name"}}]}`,
		})
		defer server.Close()
		conn := newMockOCMConn(t, server.URL)
		defer conn.Close()

		_, err := GetServiceClusterWithConn(context.Background(), "hcp-id", conn)
		if err == nil || !strings.Contains(err.Error(), "no service cluster found for management cluster mc-name") {
			t.Fatalf("expected 'no service cluster found' error, got: %v", err)
		}
	})
}
