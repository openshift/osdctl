package org

import (
	"fmt"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

/*var (
	templateParams, userParameterNames, userParameterValues, filterParams []string
	HTMLBody                                                              []byte
)*/

func sendRequest(request *sdk.Request) (*sdk.Response, error) {
	response, err := request.Send()
	if err != nil {
		return nil, fmt.Errorf("cannot send request: %q", err)
	}
	return response, nil
}

/*func validateGoodResponse(body []byte, clusterMessage servicelog.Message) (goodReply *servicelog.GoodReply, err error) {
	if !json.Valid(body) {
		return nil, fmt.Errorf("server returned invalid JSON")
	}

	if err = json.Unmarshal(body, &goodReply); err != nil {
		return nil, fmt.Errorf("cannot not parse the JSON template.\nError: %q", err)
	}

	if goodReply.Severity != clusterMessage.Severity {
		return nil, fmt.Errorf("message sent, but wrong severity information was passed (wanted %q, got %q)", clusterMessage.Severity, goodReply.Severity)
	}
	if goodReply.ServiceName != clusterMessage.ServiceName {
		return nil, fmt.Errorf("message sent, but wrong service_name information was passed (wanted %q, got %q)", clusterMessage.ServiceName, goodReply.ServiceName)
	}
	if goodReply.ClusterUUID != clusterMessage.ClusterUUID {
		return nil, fmt.Errorf("message sent, but to different cluster (wanted %q, got %q)", clusterMessage.ClusterUUID, goodReply.ClusterUUID)
	}
	if goodReply.Summary != clusterMessage.Summary {
		return nil, fmt.Errorf("message sent, but wrong summary information was passed (wanted %q, got %q)", clusterMessage.Summary, goodReply.Summary)
	}
	if goodReply.Description != clusterMessage.Description {
		return nil, fmt.Errorf("message sent, but wrong description information was passed (wanted %q, got %q)", clusterMessage.Description, goodReply.Description)
	}

	return goodReply, nil
}

func validateBadResponse(body []byte) (badReply *servicelog.BadReply, err error) {
	if ok := json.Valid(body); !ok {
		return nil, fmt.Errorf("server returned invalid JSON")
	}
	if err = json.Unmarshal(body, &badReply); err != nil {
		return nil, fmt.Errorf("cannot parse the error JSON message %q", err)
	}

	return badReply, nil
}*/
