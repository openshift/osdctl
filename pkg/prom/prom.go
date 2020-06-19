package prom

import (
	"fmt"
	"io"
	"sort"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// DecodeMetrics decodes Prometheus metrics.
func DecodeMetrics(r io.Reader, matchers map[string]string) ([]string, error) {
	dec := expfmt.NewDecoder(r, expfmt.FmtText)
	decoder := expfmt.SampleDecoder{
		Dec:  dec,
		Opts: &expfmt.DecodeOptions{},
	}

	res := make([]string, 0)
	for {
		var vector model.Vector
		if err := decoder.Decode(&vector); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		for _, metric := range vector {
			if matchLabels(metric.Metric, matchers) {
				res = append(res, fmt.Sprintf("%s => %f", metric.Metric, metric.Value))
			}
		}
	}

	sort.Strings(res)

	return res, nil
}

// matchLabels checks whether the metric has the required labels or not
func matchLabels(metric model.Metric, matchers map[string]string) bool {
	for k, v := range matchers {
		if labelValue, ok := metric[model.LabelName(k)]; !ok {
			return false
		} else if string(labelValue) != v {
			return false
		}
	}

	return true
}
