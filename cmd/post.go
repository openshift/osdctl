package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/openshift-online/ocm-cli/pkg/arguments"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	"github.com/openshift-online/ocm-cli/pkg/ocm"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osd-utils-cli/internal/servicelog"
	"github.com/openshift/osd-utils-cli/internal/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var (
	template, clusterUUID, caseID string
	Message                       servicelog.Message
	GoodReply                     servicelog.GoodReply
	BadReply                      servicelog.BadReply
)

const (
	defaultTemplate      = ""
	defaultClusterUUID   = ""
	defaultCaseID        = ""
	targetAPIPath        = "/api/service_logs/v1/cluster_logs" // https://api.openshift.com/?urls.primaryName=Service%20logs#/default/post_api_service_logs_v1_cluster_logs
	modifiedJSON         = "modified-template.json"
	clusterParameter     = "${CLUSTER_UUID}"
	caseIDParameter      = "${CASE_ID}"
	clusterUUIDLongName  = "cluster-external-id"
	caseIDLongName       = "support-case-id"
	clusterUUIDShorthand = "c"
	caseIDShorthand      = "i"
)

// postCmd represents the post command
var postCmd = &cobra.Command{
	Use:   "post",
	Short: "Send a servicelog message to a given cluster",
	Run: func(cmd *cobra.Command, args []string) {
		readTemplate() // verify and parse
		replaceFlags(clusterUUID, defaultClusterUUID, clusterParameter, clusterUUIDLongName, clusterUUIDShorthand)
		replaceFlags(caseID, defaultCaseID, caseIDParameter, caseIDLongName, caseIDShorthand)

		dir := tempDir()
		defer cleanup(dir)

		newData := modifyTemplate(dir)

		// Create an OCM client to talk to the cluster API
		// the user has to be logged in (e.g. 'ocm login')
		ocmClient := createConnection()
		defer func() {
			if err := ocmClient.Close(); err != nil {
				log.Errorf("Cannot close the ocmClient (possible memory leak): %q", err)
			}
		}()

		// Use the OCM client to create the POST request
		// send it as logservice and validate the response
		request := createRequest(ocmClient, newData)
		response := postRequest(request)
		check(response, dir)
	},
}

func init() {
	// define required flags
	postCmd.Flags().StringVarP(&template, "template", "t", defaultTemplate, "Message template file or URL")
	postCmd.Flags().StringVarP(&clusterUUID, clusterUUIDLongName, clusterUUIDShorthand, defaultClusterUUID, "Target cluster UUID")
	postCmd.Flags().StringVarP(&caseID, caseIDLongName, caseIDShorthand, defaultCaseID, "Related ticket (RedHat Support Case ID)")
}

// accessTemplate checks if the provided template is currently accessible and returns an error
func accessTemplate(template string) (err error) {

	if template == "" {
		log.Errorf("Template file is not provided. Use '-t' to fix this.")
		return err
	}

	if utils.FileExists(template) {
		return err
	}

	if utils.FolderExists(template) {
		log.Errorf("the provided template %q is a directory, not a file!", template)
	}

	if utils.IsValidUrl(template) {
		urlPage, _ := url.Parse(template)
		if err := utils.IsOnline(*urlPage); err != nil {
			log.Errorf("host %q is not accessible", template)
		} else {
			return err
		}
	}

	return fmt.Errorf("cannot read the template %q", template)

}

// parseTemplate reads the template file into a JSON struct
func parseTemplate(jsonFile []byte) error {
	return json.Unmarshal(jsonFile, &Message)
}

func parseGoodReply(jsonFile []byte) error {
	return json.Unmarshal(jsonFile, &GoodReply)
}

func parseBadReply(jsonFile []byte) error {
	return json.Unmarshal(jsonFile, &BadReply)
}

func readTemplate() {
	if err := accessTemplate(template); err == nil {
		file, err := ioutil.ReadFile(template)
		if err != nil {
			log.Fatalf("Cannot not read the file.\nError: %q\n", err)
		}

		if err = parseTemplate(file); err != nil {
			log.Fatalf("Cannot not parse the JSON template.\nError: %q\n", err)
		}
	} else {
		log.Fatal(err)
	}
}

func replaceFlags(flagName, flagDefaultValue, flagParameter, flagLongName, flagShorthand string) {
	if err := strings.Compare(flagName, flagDefaultValue); err == 0 {
		// The user didn't set the flag. Check if the template is using the flag.
		if found := Message.SearchFlag(flagParameter); found == true {
			log.Fatalf("The selected template is using '%s' parameter, but '%s' flag is not set. Use '-%s' to fix this.", flagParameter, flagLongName, flagShorthand)
		}
	} else {
		// The user set the flag. Check if the template is using the flag.
		if found := Message.SearchFlag(flagParameter); found == false {
			log.Fatalf("The selected template is not using '%s' parameter, but '%s' flag is set. Do not use '-%s' to fix this.", flagParameter, flagLongName, flagShorthand)
		}
		Message.ReplaceWithFlag(flagParameter, flagName)
	}
}

func tempDir() (dir string) {
	if dirPath, err := os.Getwd(); err != nil {
		log.Error(err)
	} else {
		dir, err = ioutil.TempDir(dirPath, "servicelog-")
		if err != nil {
			log.Fatal(err)
		}
	}
	return dir
}

func modifyTemplate(dir string) (newData string) {
	// Write the modified file
	newData = filepath.Join(dir, modifiedJSON)
	if err := utils.CreateFile(newData); err == nil {
		file, err := os.Create(newData)
		if err != nil {
			log.Fatalf("Cannot overwrite file %q", err)
		}
		defer file.Close()

		// Create the corrected JSON
		s, _ := json.MarshalIndent(Message, "", "\t")
		if _, err := file.WriteString(string(s)); err != nil {
			log.Fatalf("Cannot write the new modified template %q", err)
		}
	} else {
		log.Fatalf("Cannot create file %q", err)
	}
	return newData
}

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

func createRequest(ocmClient *sdk.Connection, newData string) *sdk.Request {
	// Create and populate the request:
	request := ocmClient.Post()
	err := arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", targetAPIPath, err)
	}
	var empty []string
	arguments.ApplyParameterFlag(request, empty)
	arguments.ApplyHeaderFlag(request, empty)
	err = arguments.ApplyBodyFlag(request, newData)
	if err != nil {
		log.Fatalf("Can't read body: %v", err)
	}
	return request
}

func postRequest(request *sdk.Request) *sdk.Response {
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

func validateGoodResponse(body []byte) {
	if err := parseGoodReply(body); err != nil {
		log.Fatalf("Cannot not parse the JSON template.\nError: %q\n", err)
	}

	severity := GoodReply.Severity
	if severity != Message.Severity {
		log.Fatalf("Message sent, but wrong severity information was passed (wanted %q, got %q)", Message.Severity, severity)
	}
	serviceName := GoodReply.ServiceName
	if serviceName != Message.ServiceName {
		log.Fatalf("Message sent, but wrong service_name information was passed (wanted %q, got %q)", Message.ServiceName, serviceName)
	}
	clusteruuid := GoodReply.ClusterUUID
	if clusterUUID != clusteruuid {
		log.Fatalf("Message sent, but to different cluster (wanted %q, got %q)", clusterUUID, clusteruuid)
	}
	summary := GoodReply.Summary
	if summary != Message.Summary {
		log.Fatalf("Message sent, but wrong summary information was passed (wanted %q, got %q)", Message.Summary, summary)
	}
	description := GoodReply.Description
	if description != Message.Description {
		log.Fatalf("Message sent, but wrong description information was passed (wanted %q, got %q)", Message.Description, description)
	}

	if err := dump.Pretty(os.Stdout, body); err != nil {
		log.Fatalf("Server returned invalid JSON reply %q", err)
	}
}

func validateBadResponse(body []byte) {
	if err := dump.Pretty(os.Stderr, body); err != nil {
		log.Errorf("Server returned invalid JSON reply %q", err)
	}

	if err := parseBadReply(body); err != nil {
		log.Fatalf("Cannot parse the error JSON message %q", err)
	}
}

func cleanup(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		log.Errorf("Cannot clean up %q", err)
	}
}
