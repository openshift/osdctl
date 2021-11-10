package servicelog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/servicelog"
	"github.com/openshift/osdctl/internal/utils"
	"github.com/openshift/osdctl/pkg/printer"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	Message         servicelog.Message
	ClustersFile    servicelog.ClustersFile
	template        string
	filterFiles     []string // Path to filter file
	filtersFromFile string   // Contents of filterFiles
	isDryRun        bool
	skipPrompts     bool
	clustersFile    string
)

const (
	defaultTemplate = ""
)

// postCmd represents the post command
var postCmd = &cobra.Command{
	Use:   "post",
	Short: "Send a servicelog message to a given cluster",
	Run: func(cmd *cobra.Command, args []string) {

		parseUserParameters() // parse all the '-p' user flags
		readFilterFile()      // parse the ocm filters in file provided via '-f' flag
		readTemplate()        // parse the given JSON template provided via '-t' flag

		// For every '-p' flag, replace its related placeholder in the template & filterFiles
		for k := range userParameterNames {
			replaceFlags(userParameterNames[k], userParameterValues[k])
		}

		// Check if there are any remaining placeholders in the template that are not replaced by a parameter,
		// excluding '${CLUSTER_UUID}' which will be replaced for each cluster later
		checkLeftovers([]string{"${CLUSTER_UUID}"})

		// Create an OCM client to talk to the cluster API
		// the user has to be logged in (e.g. 'ocm login')
		ocmClient := createConnection()
		defer func() {
			if err := ocmClient.Close(); err != nil {
				log.Errorf("Cannot close the ocmClient (possible memory leak): %q", err)
			}
		}()

		// Retrieve matching clusters
		if filtersFromFile != "" {
			if len(filterParams) != 0 {
				log.Warnf("Search queries were passed using both the '-q' and '-f' flags. This will apply logical AND between the queries, potentially resulting in no matches")
			}
			filters := strings.Join(strings.Split(strings.TrimSpace(filtersFromFile), "\n"), " ")
			filterParams = append(filterParams, filters)
		}

		if clustersFile != "" {
			contents, err := accessFile(clustersFile)
			if err != nil {
				log.Fatalf("Cannot read file %s: %q", clustersFile, err)
			}
			err = parseClustersFile(contents)
			if err != nil {
				log.Fatalf("Cannot parse file %s: %q", clustersFile, err)
			}
			query := []string{}
			for i := range ClustersFile.Clusters {
				cluster := ClustersFile.Clusters[i]
				query = append(query, generateQuery(cluster))
			}
			filterParams = query
		}

		clusters, err := applyFilters(ocmClient, filterParams)

		if err != nil {
			log.Fatalf("Cannot retrieve clusters: %q", err)
		} else if len(clusters) < 1 {
			log.Fatalf("No clusters match the given parameters.")
		}

		log.Infoln("The following clusters match the given parameters:")
		if err := printClusters(clusters); err != nil {
			log.Fatalf("Could not print matching clusters: %q", err)
		}

		log.Infoln("The following template will be sent:")
		if err := printTemplate(); err != nil {
			log.Errorf("Cannot read generated template: %q", err)
		}

		// If this is a dry-run, don't proceed further.
		if isDryRun {
			return
		}

		if !skipPrompts {
			if err = confirmSend(); err != nil {
				log.Errorf("Error confirming message: %q", err)
			}
		}

		for _, cluster := range clusters {
			request, clusterMessage := createPostRequest(ocmClient, cluster.ExternalID())
			response := sendRequest(request)
			check(response, clusterMessage)
		}
	},
}

func init() {
	// define required flags
	postCmd.Flags().StringVarP(&template, "template", "t", defaultTemplate, "Message template file or URL")
	postCmd.Flags().StringArrayVarP(&templateParams, "param", "p", templateParams, "Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.")
	postCmd.Flags().BoolVarP(&isDryRun, "dry-run", "d", false, "Dry-run - print the service log about to be sent but don't send it.")
	postCmd.Flags().StringArrayVarP(&filterParams, "query", "q", filterParams, "Specify a search query (eg. -q \"name like foo\") for a bulk-post to matching clusters.")
	postCmd.Flags().BoolVarP(&skipPrompts, "yes", "y", false, "Skips all prompts.")
	postCmd.Flags().StringArrayVarP(&filterFiles, "query-file", "f", filterFiles, "File containing search queries to apply. All lines in the file will be concatenated into a single query. If this flag is called multiple times, every file's search query will be combined with logical AND.")
	postCmd.Flags().StringVarP(&clustersFile, "clusters-file", "c", clustersFile, "Read a list of clusters to post the servicelog to")
}

// parseUserParameters parse all the '-p FOO=BAR' parameters and checks for syntax errors
func parseUserParameters() {
	var queries []string // interpret all '-p CLUSTER_UUID' parameters as queries to be made to the ocmClient
	for _, v := range templateParams {
		if !strings.Contains(v, "=") {
			log.Fatalf("Wrong syntax of '-p' flag. Please use it like this: '-p FOO=BAR'")
		}

		param := strings.Split(v, "=")
		if param[0] == "" || param[1] == "" {
			log.Fatalf("Wrong syntax of '-p' flag. Please use it like this: '-p FOO=BAR'")
		}

		if param[0] != "CLUSTER_UUID" {
			userParameterNames = append(userParameterNames, fmt.Sprintf("${%v}", param[0]))
			userParameterValues = append(userParameterValues, param[1])
		} else {
			queries = append(queries, generateQuery(param[1]))
		}
	}

	if len(queries) != 0 {
		if len(filterParams) != 0 {
			log.Warnf("At least one $CLUSTER_UUID parameter was passed with the '-q' flag. This will apply logical AND between the search query and the cluster(s) given, potentially resulting in no matches")
		}
		filterParams = append(filterParams, strings.Join(queries, " or "))
	}
}

func confirmSend() error {
	fmt.Print("Continue? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	responseBytes, _, err := reader.ReadLine()
	if err != nil {
		return err
	}
	response := strings.ToUpper(string(responseBytes))

	if response != "Y" && response != "YES" {
		if response != "N" && response != "NO" && response != "" {
			log.Fatal("Invalid response, expected 'YES' or 'Y' (case-insensitive). ")
		}
		log.Fatalf("Exiting...")
	}
	return nil
}

// accessTemplate returns the contents of a local file or url, and any errors encountered
func accessFile(filePath string) ([]byte, error) {
	if utils.FileExists(filePath) {
		file, err := ioutil.ReadFile(filePath) // template is file on the disk
		if err != nil {
			return file, fmt.Errorf("Cannot read the file.\nError: %q\n", err)
		}
		return file, nil
	}
	if utils.FolderExists(filePath) {
		return nil, fmt.Errorf("the provided path %q is a directory, not a file!", filePath)
	}
	if utils.IsValidUrl(filePath) {
		urlPage, _ := url.Parse(filePath)
		if err := utils.IsOnline(*urlPage); err != nil {
			return nil, fmt.Errorf("host %q is not accessible", filePath)
		} else {
			return utils.CurlThis(urlPage.String())
		}
	}
	return nil, fmt.Errorf("cannot read the file %q", filePath)
}

// parseClustersFile reads the clustrs file into a JSON struct
func parseClustersFile(jsonFile []byte) error {
	return json.Unmarshal(jsonFile, &ClustersFile)
}

// parseTemplate reads the template file into a JSON struct
func parseTemplate(jsonFile []byte) error {
	return json.Unmarshal(jsonFile, &Message)
}

// readTemplate loads the template into the Message variable
func readTemplate() {
	if template == defaultTemplate {
		log.Fatalf("Template file is not provided. Use '-t' to fix this.")
	}

	file, err := accessFile(template)
	if err != nil { // check if this URL or file and if we can access it
		log.Fatal(err)
	}

	if err = parseTemplate(file); err != nil {
		log.Fatalf("Cannot not parse the JSON template.\nError: %q\n", err)
	}
}

func readFilterFile() {
	if len(filterFiles) < 1 {
		// No filterFiles specified in args
		return
	}

	for _, filterFile := range filterFiles {
		fileContents, err := accessFile(filterFile)
		if err != nil {
			log.Fatal(err)
		}

		if filtersFromFile == "" {
			filtersFromFile = "(" + strings.TrimSpace(string(fileContents)) + ")"
		} else {
			filtersFromFile = filtersFromFile + " and (" + strings.TrimSpace(string(fileContents)) + ")"
		}
	}
}

// Simple helper to determine if a string is present in a slice
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

func FindLeftovers(s string) (matches []string) {
	r := regexp.MustCompile(`\${[^{}]*}`)
	matches = r.FindAllString(s, -1)
	return matches
}

func checkLeftovers(excludes []string) {
	unusedParameters, _ := Message.FindLeftovers()
	unusedParameters = append(unusedParameters, FindLeftovers(filtersFromFile)...)

	var numberOfMissingParameters int
	for _, v := range unusedParameters {
		// Ignore parameters in the exclude list, ie ${CLUSTER_UUID}, which will be replaced later for each cluster a servicelog is sent to
		if !contains(excludes, v) {
			numberOfMissingParameters++
			regex := strings.NewReplacer("${", "", "}", "")
			log.Errorf("The one of the template files is using '%s' parameter, but '--param' flag is not set for this one. Use '-p %v=\"FOOBAR\"' to fix this.", v, regex.Replace(v))
		}
	}
	if numberOfMissingParameters == 1 {
		log.Fatal("Please define this missing parameter properly.")
	} else if numberOfMissingParameters > 1 {
		log.Fatalf("Please define all %v missing parameters properly.", numberOfMissingParameters)
	}
}

func replaceFlags(flagName string, flagValue string) {
	if flagValue == "" {
		log.Fatalf("The selected template is using '%[1]s' parameter, but '%[1]s' flag was not set. Use '-p %[1]s=\"FOOBAR\"' to fix this.", flagName)
	}

	found := false
	if Message.SearchFlag(flagName) {
		found = true
		Message.ReplaceWithFlag(flagName, flagValue)
	}
	if strings.Contains(filtersFromFile, flagName) {
		found = true
		filtersFromFile = strings.ReplaceAll(filtersFromFile, flagName, flagValue)
	}

	if !found {
		log.Fatalf("The selected template is not using '%s' parameter, but '--param' flag was set. Do not use '-p %s=%s' to fix this.", flagName, flagName, flagValue)
	}
}

func printClusters(clusters []*v1.Cluster) (err error) {
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Name", "ID", "State", "Version", "Cloud Provider", "Region"})
	for _, cluster := range clusters {
		table.AddRow([]string{cluster.DisplayName(), cluster.ID(), string(cluster.State()), cluster.OpenshiftVersion(), cluster.CloudProvider().ID(), cluster.Region().ID()})
	}

	// Add empty row for readability
	table.AddRow([]string{})
	return table.Flush()
}

func printTemplate() (err error) {
	exampleMessage, err := json.Marshal(Message)
	if err != nil {
		return err
	}
	return dump.Pretty(os.Stdout, exampleMessage)
}

func createPostRequest(ocmClient *sdk.Connection, clusterId string) (request *sdk.Request, clusterMessage servicelog.Message) {
	// Create and populate the request:
	request = ocmClient.Post()
	err := arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", targetAPIPath, err)
	}

	clusterMessage = Message
	clusterMessage.ReplaceWithFlag("${CLUSTER_UUID}", clusterId)
	messageBytes, err := json.Marshal(clusterMessage)
	if err != nil {
		log.Fatalf("Cannot marshal template to json: %s", err)
	}

	request.Bytes(messageBytes)
	return request, clusterMessage
}
