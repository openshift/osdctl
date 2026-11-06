package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestCheckVersion ensures grace is limited to the immediate stable predecessor
// and expires exactly seven days after publication, regardless of API ordering.
func TestCheckVersion(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name       string
		current    string
		age        time.Duration
		grace      bool
		listCalled bool
	}{
		{"previous release", "1.2.0", time.Hour, true, true},
		{"previous release with v prefix", "v1.2.0", time.Hour, true, true},
		{"just under a week", "1.2.0", 7*24*time.Hour - time.Second, true, true},
		{"exactly a week", "1.2.0", 7 * 24 * time.Hour, false, false},
		{"over a week", "1.2.0", 8 * 24 * time.Hour, false, false},
		{"two releases behind", "1.1.0", time.Hour, false, true},
		{"latest release", "1.3.0", time.Hour, false, false},
		{"unreleased build", "1.4.0-dev", time.Hour, false, true},
		{"prerelease", "1.3.0-rc.1", time.Hour, false, true},
		{"future publication", "1.2.0", -time.Hour, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			latest := gitHubResponse{TagName: "v1.3.0", PublishedAt: now.Add(-tt.age)}
			var listCalled atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/releases/latest" {
					_ = json.NewEncoder(w).Encode(latest)
					return
				}
				listCalled.Store(true)
				if r.URL.Path != "/releases" || r.URL.Query().Get("per_page") != "100" {
					t.Errorf("unexpected release list URL: %s", r.URL)
				}
				// Publication order, not response order, determines the predecessor.
				_ = json.NewEncoder(w).Encode([]gitHubResponse{
					{TagName: "v1.1.0", PublishedAt: latest.PublishedAt.Add(-48 * time.Hour)},
					latest,
					{TagName: "v1.3.0-rc.1", PublishedAt: latest.PublishedAt.Add(-time.Hour), Prerelease: true},
					{TagName: "v1.4.0", PublishedAt: latest.PublishedAt.Add(-time.Hour), Draft: true},
					{TagName: "v1.2.0", PublishedAt: latest.PublishedAt.Add(-24 * time.Hour)},
				})
			}))
			defer server.Close()

			latestTag, grace, err := checkVersion(server.Client(), server.URL+"/releases/latest", tt.current, now)
			if err != nil || latestTag != latest.TagName || grace != tt.grace {
				t.Fatalf("checkVersion() = (%q, %v, %v), want (%q, %v, nil)", latestTag, grace, err, latest.TagName, tt.grace)
			}
			if called := listCalled.Load(); called != tt.listCalled {
				t.Errorf("release list called = %v, want %v", called, tt.listCalled)
			}
		})
	}
}

// TestCheckVersionMissingMetadata guards against granting grace when the API
// does not provide enough publication metadata to establish eligibility.
func TestCheckVersionMissingMetadata(t *testing.T) {
	for _, tt := range []struct {
		name   string
		latest string
		list   string
	}{
		{"missing publication date", `{"tag_name":"v1.3.0"}`, `[]`},
		{"no previous release", `{"tag_name":"v1.3.0","published_at":"2026-05-12T11:00:00Z"}`, `[]`},
		{"previous missing date", `{"tag_name":"v1.3.0","published_at":"2026-05-12T11:00:00Z"}`, `[{"tag_name":"v1.2.0"}]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := tt.list
				if r.URL.Path == "/releases/latest" {
					body = tt.latest
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			_, grace, err := checkVersion(server.Client(), server.URL+"/releases/latest", "1.2.0", time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
			if err != nil || grace {
				t.Fatalf("checkVersion() grace = %v, err = %v; want false, nil", grace, err)
			}
		})
	}
}

// TestCheckVersionAPIErrors ensures failed requests never grant grace and retain
// the latest version when only the predecessor lookup fails.
func TestCheckVersionAPIErrors(t *testing.T) {
	for _, path := range []string{"/releases/latest", "/releases"} {
		for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError, http.StatusOK} {
			t.Run(path+"/"+http.StatusText(status), func(t *testing.T) {
				now := time.Now()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == path {
						w.WriteHeader(status)
						_, _ = w.Write([]byte("invalid JSON"))
						return
					}
					_ = json.NewEncoder(w).Encode(gitHubResponse{TagName: "v1.3.0", PublishedAt: now.Add(-time.Hour)})
				}))
				defer server.Close()
				latest, grace, err := checkVersion(server.Client(), server.URL+"/releases/latest", "1.2.0", now)
				if err == nil || grace {
					t.Fatalf("checkVersion() = (%q, %v, %v); expected error and no grace period", latest, grace, err)
				}
				if path == "/releases" && latest != "v1.3.0" {
					t.Errorf("latest version lost on release list error: %q", latest)
				}
			})
		}
	}
}
