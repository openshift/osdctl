package cluster

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

func createGraphViz(ai *aggregateClusterInfo) map[Node][]Node {
	connections := make(map[Node][]Node)
	for _, hz := range ai.privatelinkInfo.HostedZones {
		hzn := Node{
			Id:                    *hz.Id,
			AdditionalInformation: fmt.Sprintf("Hosted Zone (P)\\n%s", *hz.Name),
			Subgraph:              "privatelink",
		}
		connections[hzn] = make([]Node, 0)
	}
	var privatelinkVpce string
	for _, rrs := range ai.privatelinkInfo.ResourceRecords {
		for _, rr := range rrs.ResourceRecords {
			if strings.Contains(*rr.Value, "vpce") {
				privatelinkVpce = *rr.Value
			}
		}
	}
	var mgmntService string
	for _, svcs := range ai.managementClusterInfo.EndpointServices {
		for _, dns := range svcs.BaseEndpointDnsNames {
			if strings.Contains(privatelinkVpce, dns) {
				mgmntService = *svcs.ServiceId
			}
		}
	}
	plvpce := Node{
		Id:                    privatelinkVpce,
		AdditionalInformation: "VPC Endpoint (P)",
		Subgraph:              "privatelink",
	}
	mgmntsvc := Node{
		Id:                    mgmntService,
		AdditionalInformation: "Endpoint Service (M)",
		Subgraph:              "management",
	}
	connections[plvpce] = append(connections[plvpce], mgmntsvc)
	for _, conn := range ai.managementClusterInfo.EndpointConnections {
		node := Node{
			Id:                    *conn.VpcEndpointConnectionId,
			AdditionalInformation: "Endpoint Connection (M)",
			Subgraph:              "management",
		}
		connections[mgmntsvc] = append(connections[mgmntsvc], node)
		for _, lb := range conn.NetworkLoadBalancerArns {
			lb := Node{
				Id:                    lb,
				AdditionalInformation: "Load Balancer (M)",
				Subgraph:              "management",
			}
			connections[node] = append(connections[node], lb)
		}
	}
	for _, hz := range ai.clusterInfo.HostedZones {
		hzn := Node{
			Id:                    *hz.Id,
			AdditionalInformation: fmt.Sprintf("Hosted Zone (C)\\n%s", *hz.Name),
			Subgraph:              "customer",
		}
		connections[hzn] = make([]Node, 0)
	}
	for _, rrs := range ai.clusterInfo.ResourceRecords {
		for _, hz := range ai.privatelinkInfo.HostedZones {
			var hzNode Node
			for connection := range connections {
				if connection.Id == *hz.Id {
					hzNode = connection
				}
			}
			for _, rr := range rrs.ResourceRecords {
				if *rr.Value+"." == *hz.Name {
					node := Node{
						Id:                    *rr.Value,
						AdditionalInformation: "Resource Record (C)",
						Subgraph:              "customer",
					}
					connections[node] = make([]Node, 0)
					connections[hzNode] = append(connections[hzNode], node)
				}
			}
		}
	}
	return connections
}

func renderGraphViz(connections map[Node][]Node) {
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
	verboseLog("Graphviz Input - please run this: 'echo <output> | dot -Tpng -o/tmp/example.png'")
	fmt.Println(sb.String())
}
