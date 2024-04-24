package dynatrace

import (
	"fmt"
	"strings"
)

type DTQuery struct {
	fragments  []string
	finalQuery string
}

func (q *DTQuery) Init(hours int) *DTQuery {
	q.fragments = []string{}

	q.fragments = append(q.fragments, fmt.Sprintf("fetch logs, from:now()-%dh \n| filter matchesValue(event.type, \"LOG\")", hours))

	return q
}

func (q *DTQuery) Cluster(mgmtClusterName string) *DTQuery {
	q.fragments = append(q.fragments, fmt.Sprintf(" and matchesPhrase(dt.kubernetes.cluster.name, \"%s\")", mgmtClusterName))

	return q
}

func (q *DTQuery) Namespaces(namespaceList []string) *DTQuery {
	var nsQuery string
	finalQuery := ""
	nsQuery = " and ("

	for i, ns := range namespaceList {
		nsQuery += fmt.Sprintf("matchesValue(k8s.namespace.name, \"%s\")", ns)
		if i < len(namespaceList)-1 {
			nsQuery += " or "
		}
	}
	nsQuery += ")"
	finalQuery += nsQuery

	q.fragments = append(q.fragments, finalQuery)

	return q
}

func (q *DTQuery) Nodes(nodeList []string) *DTQuery {
	var nodeQuery string

	nodeQuery = " and ("
	for i, node := range nodeList {
		nodeQuery += fmt.Sprintf("matchesValue(k8s.node.name, \"%s\")", node)
		if i < len(nodeList)-1 {
			nodeQuery += " or "
		}
	}
	nodeQuery += ")"
	q.fragments = append(q.fragments, nodeQuery)

	return q
}

func (q *DTQuery) Pods(podList []string) *DTQuery {
	var podQuery string

	podQuery = " and ("
	for i, pod := range podList {
		podQuery += fmt.Sprintf("matchesValue(k8s.pod.name, \"%s\")", pod)
		if i < len(podList)-1 {
			podQuery += " or "
		}
	}
	podQuery += ")"
	q.fragments = append(q.fragments, podQuery)

	return q
}

func (q *DTQuery) Containers(containerList []string) *DTQuery {
	var containerQuery string

	containerQuery = " and ("
	for i, container := range containerList {
		containerQuery += fmt.Sprintf("matchesValue(k8s.container.name, \"%s\")", container)
		if i < len(containerList)-1 {
			containerQuery += " or "
		}
	}
	containerQuery += ")"
	q.fragments = append(q.fragments, containerQuery)

	return q
}

func (q *DTQuery) Status(statusList []string) *DTQuery {
	var statusQuery string

	statusQuery = " and ("
	for i, status := range statusList {
		statusQuery += fmt.Sprintf("matchesValue(status, \"%s\")", status)
		if i < len(statusList)-1 {
			statusQuery += " or "
		}
	}
	statusQuery += ")"
	q.fragments = append(q.fragments, statusQuery)

	return q
}

func (q *DTQuery) ContainsPhrase(phrase string) *DTQuery {
	q.fragments = append(q.fragments, " and contains(content,\""+phrase+"\")")

	return q
}

func (q *DTQuery) Sort(order string) (query *DTQuery, error error) {
	validOrders := []string{
		"asc",
		"desc",
	}

	for _, or := range validOrders {
		if or == order {
			q.fragments = append(q.fragments, fmt.Sprintf("\n| sort timestamp %s", order))
			return q, nil
		}
	}

	return q, fmt.Errorf("no valid sorting order specified. valid order are %s. given %v", strings.Join(validOrders, ", "), order)
}

func (q *DTQuery) Limit(limit int) *DTQuery {
	q.fragments = append(q.fragments, "\n| limit "+fmt.Sprint(limit))

	return q
}

func (q *DTQuery) Build() string {
	q.finalQuery = strings.Join(q.fragments[:], "")

	return q.finalQuery
}
