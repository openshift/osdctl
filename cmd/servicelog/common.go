package servicelog

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/openshift-online/ocm-cli/pkg/dump"
	"github.com/openshift-online/ocm-cli/pkg/ocm"
	ocm_servicelog "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"

	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/servicelog"
	log "github.com/sirupsen/logrus"
)

var (
	templateParams, userParameterNames, userParameterValues, filterParams []string
	HTMLBody                                                              []byte
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

// generateQuery returns an OCM search query to retrieve all clusters matching an expression (ie- "foo%")
func generateQuery(clusterIdentifier string) string {
	return strings.TrimSpace(fmt.Sprintf("(id like '%[1]s' or external_id like '%[1]s' or display_name like '%[1]s')", clusterIdentifier))
}

// getFilteredClusters retrieves clusters in OCM which match the filters given
func applyFilters(ocmClient *sdk.Connection, filters []string) ([]*v1.Cluster, error) {
	if len(filters) < 1 {
		return nil, nil
	}

	for k, v := range filters {
		filters[k] = fmt.Sprintf("(%s)", v)
	}

	requestSize := 50
	full_filters := strings.Join(filters, " and ")

	log.Infof(`running the command: 'ocm list clusters --parameter=search="%s"'`, full_filters)

	request := ocmClient.ClustersMgmt().V1().Clusters().List().Search(full_filters).Size(requestSize)
	response, err := request.Send()
	if err != nil {
		return nil, err
	}

	items := response.Items().Slice()
	for response.Size() >= requestSize {
		request.Page(response.Page() + 1)
		response, err = request.Send()
		if err != nil {
			return nil, err
		}
		items = append(items, response.Items().Slice()...)
	}

	return items, err
}

func check(response *ocm_servicelog.ClusterLogsAddResponse, clusterMessage servicelog.Message) {
	uuid := clusterMessage.ClusterUUID()
	if response.Error() != nil {
		failedClusters[uuid] = response.Error().Error()
		return
	}
	if response.Status() < 400 {
		err := validateResponse(response.Body(), clusterMessage.LogEntry)
		if err != nil {
			failedClusters[uuid] = err.Error()
			return
		}
		successfulClusters[uuid] = fmt.Sprintf("Message has been successfully sent to %s", uuid)
		return
	}
	failedClusters[uuid] = fmt.Sprintf("the servicelog did not error but the status is %d, failing", response.Status())
}

func validateResponse(got, wanted *ocm_servicelog.LogEntry) error {
	if got.Severity() != wanted.Severity() {
		return fmt.Errorf("message sent, but wrong severity information was passed (wanted %q, got %q)", wanted.Severity(), got.Severity())
	}
	if got.ServiceName() != wanted.ServiceName() {
		return fmt.Errorf("message sent, but wrong service_name information was passed (wanted %q, got %q)", wanted.ServiceName(), got.ServiceName())
	}
	if got.ClusterUUID() != wanted.ClusterUUID() {
		return fmt.Errorf("message sent, but to different cluster (wanted %q, got %q)", wanted.ClusterUUID(), got.ClusterUUID())
	}
	if got.Summary() != wanted.Summary() {
		return fmt.Errorf("message sent, but wrong summary information was passed (wanted %q, got %q)", wanted.Summary(), got.Summary())
	}
	if got.Description() != wanted.Description() {
		return fmt.Errorf("message sent, but wrong description information was passed (wanted %q, got %q)", wanted.Description(), got.Description())
	}

	return nil
}

func printEntry(entry *ocm_servicelog.LogEntry) error {
	buf := bytes.NewBuffer(nil)

	err := ocm_servicelog.MarshalLogEntry(entry, buf)
	if err != nil {
		return err
	}

	return dump.Pretty(os.Stdout, buf.Bytes())
}
