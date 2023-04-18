package support

import (
	"errors"
	"fmt"
	ctlutil "github.com/openshift/osdctl/pkg/utils"
	"os"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

func sendRequest(request *sdk.Request) (*sdk.Response, error) {

	response, err := request.Send()
	if err != nil {
		return nil, fmt.Errorf("cannot send request: %q", err)
	}
	return response, nil
}

func getLimitedSupportReasons(clusterId string) ([]*ctlutil.LimitedSupportReasonItem, error) {
	// Check that the cluster key (name, identifier or external identifier) given by the user
	// is reasonably safe so that there is no risk of SQL injection
	err := ctlutil.IsValidClusterKey(clusterId)
	if err != nil {
		return nil, err
	}

	//create connection to sdk
	connection := ctlutil.CreateConnection()
	defer func() {
		if err := connection.Close(); err != nil {
			fmt.Printf("Cannot close the connection: %q\n", err)
			os.Exit(1)
		}
	}()

	//getting the cluster
	cluster, err := ctlutil.GetCluster(connection, clusterId)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Can't retrieve cluster: %v\n", err))
	}

	//getting the limited support reasons for the cluster
	clusterLimitedSupportReasons, err := ctlutil.GetClusterLimitedSupportReasons(connection, cluster.ID())
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Can't retrieve cluster limited support reasons: %v\n", err))
	}

	return clusterLimitedSupportReasons, nil
}
