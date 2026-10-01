package servicelog

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/osdctl/pkg/link_validator"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
)

const targetAPIPath = "/api/service_logs/v1/cluster_logs"

var ErrDeclined = errors.New("service log not sent: user declined")

// PostRequest describes what to send and how to interact with the user.
type PostRequest struct {
	Template       string
	TemplateParams []string
	InternalOnly   bool
	SkipLinkCheck  bool
}

// Post sends a service log to a cluster from a template. It handles the
// full pipeline: template loading, parameter substitution, link validation,
// user confirmation, and HTTP POST. It always runs an interactive
// confirmation flow. For non-interactive sends (e.g. batch operations
// that confirm upfront), use Prepare + PostMessage directly.
func Post(ocmClient *sdk.Connection, cluster *cmv1.Cluster, req PostRequest) error {
	msg, err := Prepare(req)
	if err != nil {
		return err
	}

	if CheckServiceLogsLastHour(ocmClient, cluster.ID()) {
		if !ocmutils.ConfirmPrompt() {
			fmt.Println("Service log not sent (user declined).")
			return ErrDeclined
		}
	}
	log.Infof("Sending service log to cluster %s (%s)", cluster.Name(), cluster.ID())
	log.Infoln("The following service log will be sent:")
	templateBytes, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal service log for preview: %w", err)
	}
	fmt.Println(string(templateBytes))
	if !ocmutils.ConfirmPrompt() {
		fmt.Println("Service log not sent (user declined).")
		return ErrDeclined
	}

	if err := PostMessage(ocmClient, cluster, msg); err != nil {
		return err
	}

	log.Infof("Successfully sent service log to cluster %s (%s)", cluster.Name(), cluster.ID())
	return nil
}

// Prepare loads a template, substitutes parameters, and validates links,
// returning a Message ready for PostMessage. Use this to prepare once
// and send to multiple clusters without re-fetching the template.
func Prepare(req PostRequest) (Message, error) {
	msg, err := loadTemplate(req.Template, req.InternalOnly)
	if err != nil {
		return msg, fmt.Errorf("failed to load template: %w", err)
	}

	params, err := parseParams(req.TemplateParams)
	if err != nil {
		return msg, fmt.Errorf("failed to parse parameters: %w", err)
	}

	if err := substituteParams(&msg, params); err != nil {
		return msg, fmt.Errorf("failed to substitute parameters: %w", err)
	}

	if req.InternalOnly {
		msg.InternalOnly = true
	}

	if !req.SkipLinkCheck {
		if err := validateLinks(msg); err != nil {
			return msg, err
		}
	}

	return msg, nil
}

// PostMessage posts a pre-built Message to a cluster. It sets
// cluster-specific fields (ClusterUUID, ClusterID, SubscriptionID)
// and sends the HTTP request. No template loading or prompting.
func PostMessage(ocmClient *sdk.Connection, cluster *cmv1.Cluster, msg Message) error {
	msg.DocReferences = slices.Clone(msg.DocReferences)
	msg.ClusterUUID = cluster.ExternalID()
	msg.ClusterID = cluster.ID()
	if subscription := cluster.Subscription(); subscription != nil {
		msg.SubscriptionID = subscription.ID()
	}

	messageBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal service log message: %w", err)
	}

	request := ocmClient.Post()
	if err := arguments.ApplyPathArg(request, targetAPIPath); err != nil {
		return fmt.Errorf("failed to parse API path %q: %w", targetAPIPath, err)
	}
	request.Bytes(messageBytes)

	response, err := ocmutils.SendRequest(request)
	if err != nil {
		return fmt.Errorf("failed to send service log: %w", err)
	}

	if response.Status() >= 200 && response.Status() < 300 {
		return nil
	}

	var errBody struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(response.Bytes(), &errBody); err == nil && errBody.Reason != "" {
		return fmt.Errorf("service log failed: %s", errBody.Reason)
	}
	return fmt.Errorf("service log failed with status %d", response.Status())
}

func validateLinks(msg Message) error {
	lv := link_validator.NewLinkValidator()
	messageText := msg.Summary + " " + msg.Description
	warnings, err := lv.ValidateLinks(messageText)
	if err != nil {
		return fmt.Errorf("service log contains dead links: %w", err)
	}
	for _, warning := range warnings {
		log.Warnf("link warning: %s (%v)", warning.URL, warning.Warning)
	}
	return nil
}
