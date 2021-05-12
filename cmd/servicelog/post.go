package servicelog

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/internal/servicelog"
	"github.com/openshift/osdctl/internal/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	GoodReply servicelog.GoodReply
	BadReply  servicelog.BadReply
	Message   servicelog.Message
	template  string
)

const (
	defaultTemplate = ""
	modifiedJSON    = "modified-template.json"
)

// postCmd represents the post command
var postCmd = &cobra.Command{
	Use:   "post",
	Short: "Send a servicelog message to a given cluster",
	Run: func(cmd *cobra.Command, args []string) {

		parseUserParameters() // parse all the '-p' user flags

		readTemplate() // parse the given JSON template provided via '-t' flag

		// For every '-p' flag, replace its related placeholder in the template
		for k, v := range templateParams {
			replaceFlags(userParameterValues[k], "", userParameterNames[k], userParameterNames[k], "p", v)
		}

		// Check if there are any remaining placeholders in the template that are not replaced by a parameter
		checkLeftovers()

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
		request := createPostRequest(ocmClient, newData)
		response := sendRequest(request)

		check(response, dir)

		err := dump.Pretty(os.Stdout, response.Bytes())
		if err != nil {
			log.Errorf("cannot print post command: %q", err)
		}
	},
}

func init() {
	// define required flags
	postCmd.Flags().StringVarP(&template, "template", "t", defaultTemplate, "Message template file or URL")
	postCmd.Flags().StringArrayVarP(&templateParams, "param", "p", templateParams, "Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.")
}

// parseUserParameters parse all the '-p FOO=BAR' parameters and checks for syntax errors
func parseUserParameters() {
	for k, v := range templateParams {
		if !strings.Contains(v, "=") {
			log.Fatalf("Wrong syntax of '-p' flag. Please use it like this: '-p FOO=BAR'")
		}

		userParameterNames = append(userParameterNames, fmt.Sprintf("${%v}", strings.Split(v, "=")[0]))
		userParameterValues = append(userParameterValues, strings.Split(v, "=")[1])

		if userParameterValues[k] == "" {
			log.Fatalf("Wrong syntax of '-p' flag. Please use it like this: '-p FOO=BAR'")
		}
	}
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
			HTMLBody, err = utils.CurlThis(urlPage.String())
			if err == nil {
				isURL = true
			}
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
	if err := accessTemplate(template); err == nil { // check if this URL or file and if we can access it
		var file []byte
		if isURL {
			// template is URL on the web
			file = HTMLBody
		} else {
			// template is file on the disk
			file, err = ioutil.ReadFile(template) // this works only for files
			if err != nil {
				log.Fatalf("Cannot not read the file.\nError: %q\n", err)
			}
		}

		if err = parseTemplate(file); err != nil {
			log.Fatalf("Cannot not parse the JSON template.\nError: %q\n", err)
		}
	} else {
		log.Fatal(err)
	}
}

func checkLeftovers() {
	unusedParameters, found := Message.FindLeftovers()
	if found {
		for _, v := range unusedParameters {
			regex := strings.NewReplacer("${", "", "}", "")
			log.Errorf("The selected template is using '%s' parameter, but '--%s' flag is not set for this one. Use '-%s %v=\"FOOBAR\"' to fix this.", v, "param", "p", regex.Replace(v))
		}
		if numberOfMissingParameters := len(unusedParameters); numberOfMissingParameters == 1 {
			log.Fatal("Please define this missing parameter properly.")
		} else {
			log.Fatalf("Please define all %v missing parameters properly.", numberOfMissingParameters)
		}
	}
}

func replaceFlags(flagName, flagDefaultValue, flagParameter, flagLongName, flagShorthand, parameter string) {
	if err := strings.Compare(flagName, flagDefaultValue); err == 0 {
		// The user didn't set the flag. Check if the template is using the flag.
		if found := Message.SearchFlag(flagParameter); found == true {
			log.Fatalf("The selected template is using '%s' parameter, but '%s' flag was not set. Use '-%s' to fix this.", flagParameter, flagLongName, flagShorthand)
		}
	} else {
		// The user set the flag. Check if the template is using the flag.
		if found := Message.SearchFlag(flagParameter); found == false {
			log.Fatalf("The selected template is not using '%s' parameter, but '--%s' flag was set. Do not use '-%s %s' to fix this.", flagParameter, "param", flagShorthand, parameter)
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

func createPostRequest(ocmClient *sdk.Connection, newData string) *sdk.Request {
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
	if clusteruuid != Message.ClusterUUID {
		log.Fatalf("Message sent, but to different cluster (wanted %q, got %q)", Message.ClusterUUID, clusteruuid)
	}
	summary := GoodReply.Summary
	if summary != Message.Summary {
		log.Fatalf("Message sent, but wrong summary information was passed (wanted %q, got %q)", Message.Summary, summary)
	}
	description := GoodReply.Description
	if description != Message.Description {
		log.Fatalf("Message sent, but wrong description information was passed (wanted %q, got %q)", Message.Description, description)
	}
	if ok := json.Valid(body); !ok {
		log.Fatalf("Server returned invalid JSON")
	}
}

func validateBadResponse(body []byte) {
	if ok := json.Valid(body); !ok {
		log.Errorf("Server returned invalid JSON")
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
