package graphviz

import (
	"fmt"
	"strings"
)

type Node struct {
	Id                    string
	AdditionalInformation string
	Subgraph              string
}

func (n *Node) Render() string {
	return fmt.Sprintf("%s\\n%s", n.AdditionalInformation, n.Id)
}

func RenderGraphViz(connections map[Node][]Node) {
	subgraphs := make(map[string]bool)
	sb := strings.Builder{}
	sb.WriteString("strict graph {\n")
	sb.WriteString("node [shape=box]\n")
	for node := range connections {
		subgraphs[node.Subgraph] = true
	}
	for subgraph := range subgraphs {
		sb.WriteString(fmt.Sprintf("subgraph cluster_%s {\n", subgraph))
		for node, nodes := range connections {
			if node.Subgraph == subgraph {
				sb.WriteString(fmt.Sprintf("\"%s\"\n", node.Render()))
				for _, v := range nodes {
					sb.WriteString(fmt.Sprintf("  \"%s\" -- \"%s\"\n", node.Render(), v.Render()))
				}
			}
		}
		sb.WriteString("}\n")
	}
	for node, nodes := range connections {
		if node.Subgraph == "" {
			sb.WriteString(fmt.Sprintf("\"%s\"\n", node.Render()))
			for _, v := range nodes {
				sb.WriteString(fmt.Sprintf("  \"%s\" -- \"%s\"\n", node.Render(), v.Render()))
			}
		}
	}
	sb.WriteString("}")
	fmt.Println(sb.String())
}
