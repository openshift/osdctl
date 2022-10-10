package support

import (
	"fmt"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

func sendRequest(request *sdk.Request) (*sdk.Response, error) {

	response, err := request.Send()
	if err != nil {
		return nil, fmt.Errorf("cannot send request: %q", err)
	}
	return response, nil
}
