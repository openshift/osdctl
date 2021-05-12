package servicelog

import (
	"strings"

	"github.com/openshift-online/ocm-cli/pkg/ocm"
	sdk "github.com/openshift-online/ocm-sdk-go"
	log "github.com/sirupsen/logrus"
)

var (
	templateParams, userParameterNames, userParameterValues []string
	isURL                                                   bool
	HTMLBody                                                []byte
)

const (
	// in case you want to see the swagger code gen, you can look at
	// https://api.openshift.com/?urls.primaryName=Service%20logs#/default/post_api_service_logs_v1_cluster_logs
	targetAPIPath = "/api/service_logs/v1/cluster_logs"
)

func createConnection() *sdk.Connection {
	connection, err := ocm.NewConnection().Build()
	if err != nil {
		if strings.Contains(err.Error(), "Not logged in, run the") {
			log.Fatalf("Failed to create OCM connection: Authetication error, run the 'ocm login' command first.")
		}
		log.Fatalf("Failed to create OCM connection: %v", err)
	}
	return connection
}

func sendRequest(request *sdk.Request) *sdk.Response {
	response, err := request.Send()
	if err != nil {
		log.Fatalf("Can't send request: %v", err)
	}
	return response
}

func check(response *sdk.Response, dir string) {
	status := response.Status()

	body := response.Bytes()

	if status < 400 {
		validateGoodResponse(body)
		log.Info("Message has been successfully sent")

	} else {
		validateBadResponse(body)
		cleanup(dir)
		log.Fatalf("Failed to post message because of %q", BadReply.Reason)

	}
}
