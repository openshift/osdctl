package rhobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// hcpLogsPageSize is the Loki rows-per-request used for the bulk HCP export. It is far larger than
// the interactive default (500) to keep the number of requests manageable over a 72h window.
const hcpLogsPageSize = 5000

// GatherHCPLogs fetches pod logs for the given namespaces from RHOBS over the [now-since, now]
// window and writes them under destDir, one file per pod. It is the RHOBS replacement for the
// former Dynatrace-based HCP log collection. (Kubernetes Events are collected separately by the
// ACM hypershift dump as core/events.yaml, so they are intentionally not fetched here.)
//
// A namespace with no matching logs is not an error, but a query/transport/auth failure is returned
// so callers can distinguish an empty result from a collection failure. It returns the total number
// of log lines written across all namespaces, so callers can report whether data was actually found.
// maxLines bounds the most-recent lines fetched per namespace (0 = unlimited). A positive cap makes
// the collection fetch newest-first and stop, bounding both runtime and archive size.
func GatherHCPLogs(ctx context.Context, clusterId, hiveOcmUrl, destDir string, namespaces []string, since time.Duration, maxLines int) (totalLogs int, err error) {
	fetcher, err := CreateRhobsFetcher(ctx, clusterId, RhobsFetchForLogs, hiveOcmUrl)
	if err != nil {
		return 0, fmt.Errorf("failed to set up RHOBS access: %w", err)
	}

	// Bulk export: fetch large pages to cut the number of round-trips (and token refreshes) over a
	// 72h window, which is otherwise dominated by paginating 500 rows at a time.
	fetcher.pageSize = hcpLogsPageSize

	// Pre-warm the token provider once so the per-namespace workers below don't race initializing it.
	if _, err := fetcher.getTokenProvider(); err != nil {
		return 0, fmt.Errorf("failed to set up RHOBS access: %w", err)
	}

	endTime := time.Now()
	startTime := endTime.Add(-since)
	clusterExtId := fetcher.logsClusterExtId()

	// De-duplicate namespaces: distinct entries get their own goroutine and output dir, so a
	// repeated namespace (e.g. when the hosted and control-plane namespaces coincide) would
	// otherwise have two workers writing the same pod files concurrently.
	namespaces = dedupeStrings(namespaces)

	type nsResult struct {
		logs int
		err  error
	}
	results := make([]nsResult, len(namespaces))

	// Namespaces are independent (separate queries and output dirs), so collect them concurrently.
	var wg sync.WaitGroup
	for i, namespace := range namespaces {
		wg.Add(1)
		go func(i int, namespace string) {
			defer wg.Done()

			logs, logErr := fetcher.writeNamespaceLogs(ctx, destDir, namespace, clusterExtId, startTime, endTime, maxLines)
			results[i] = nsResult{logs: logs, err: logErr}

			log.Infof("RHOBS %s: %d log lines", namespace, logs)
		}(i, namespace)
	}
	wg.Wait()

	for _, res := range results {
		totalLogs += res.logs
		if res.err != nil && err == nil {
			err = res.err
		}
	}

	return totalLogs, err
}

// dedupeStrings returns the input with duplicates removed, preserving first-seen order.
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// hcpLogsExpr builds the LogQL expression selecting a namespace's logs (excluding events)
// scoped to the cluster's openshift_cluster_uid.
func hcpLogsExpr(namespace, clusterExtId string) string {
	return fmt.Sprintf(`{k8s_namespace_name=%q} | json json_kind="kind" | json_kind != "Event" | openshift_cluster_uid = %q`, namespace, clusterExtId)
}

func (f *RhobsFetcher) writeNamespaceLogs(ctx context.Context, destDir, namespace, clusterExtId string, startTime, endTime time.Time, maxLines int) (int, error) {
	writer := newPodLogWriter(filepath.Join(destDir, namespace, "pods"))
	defer writer.close()

	// A positive cap fetches the most-recent maxLines newest-first and stops paginating;
	// 0 means unlimited, read oldest-first like `kubectl logs`.
	logsCount, forward := -1, true
	if maxLines > 0 {
		logsCount, forward = maxLines, false
	}

	count := 0
	err := f.queryLogs(ctx, hcpLogsExpr(namespace, clusterExtId), startTime, endTime, logsCount, forward, func(result *logResult) {
		count++
		if writeErr := writer.write(result.Stream["k8s_pod_name"], result.getMessage()); writeErr != nil {
			log.Warnf("failed to write log line for namespace %s: %v", namespace, writeErr)
		}
	})
	if err != nil {
		return count, fmt.Errorf("failed to query logs for namespace %s: %w", namespace, err)
	}
	return count, nil
}

// podLogWriter routes streamed log lines to one file per pod under dir, opening each file lazily
// on its first line so namespaces/pods with no logs leave no empty files behind.
type podLogWriter struct {
	dir   string
	files map[string]*os.File
}

func newPodLogWriter(dir string) *podLogWriter {
	return &podLogWriter{dir: dir, files: map[string]*os.File{}}
}

func (w *podLogWriter) write(podName, message string) error {
	if podName == "" {
		podName = "_unknown"
	}
	file, ok := w.files[podName]
	if !ok {
		if err := os.MkdirAll(w.dir, 0o750); err != nil {
			return err
		}
		opened, err := os.OpenFile(filepath.Join(w.dir, podName+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		w.files[podName] = opened
		file = opened
	}
	_, err := file.WriteString(message + "\n")
	return err
}

func (w *podLogWriter) close() {
	for _, file := range w.files {
		_ = file.Close()
	}
}
