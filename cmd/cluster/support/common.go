package support

import (
	"errors"
	"fmt"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	ctlutil "github.com/openshift/osdctl/pkg/utils"
	"os"
)

func getLimitedSupportReasons(clusterId string) ([]*cmv1.LimitedSupportReason, error) {
	// Check that the cluster key (name, identifier or external identifier) given by the user
	// is reasonably safe so that there is no risk of SQL injection
	err := ctlutil.IsValidClusterKey(clusterId)
	if err != nil {
		return nil, err
	}

	//create connection to sdk
	connection, err := ctlutil.CreateConnection()
	if err != nil {
		return nil, err
	}
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
