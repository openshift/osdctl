package servicelog

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/servicelog"
	"github.com/openshift/osdctl/internal/utils"
	"github.com/openshift/osdctl/pkg/printer"
	ctlutil "github.com/openshift/osdctl/pkg/utils"
	ocmutils "github.com/openshift/osdctl/pkg/utils"

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
	internalOnly    bool

	// Messaged clusters
	successfulClusters = make(map[string]string)
	failedClusters     = make(map[string]string)
)

const (
	defaultTemplate = ""
)

// postCmd represents the post command
var postCmd = &cobra.Command{
	Use:   "post CLUSTER_ID",
	Short: "Send a servicelog message to a given cluster",
	Run: func(cmd *cobra.Command, args []string) {

		parseUserParameters() // parse all the '-p' user flags
		readFilterFile()      // parse the ocm filters in file provided via '-f' flag
		readTemplate()        // parse the given JSON template provided via '-t' flag

		if len(args) == 0 && len(filterParams) == 0 && clustersFile == "" {
			log.Fatalf("No cluster identifier has been found.")
		}

		var queries []string
		if len(args) != 1 {
			log.Infof("Too many arguments. Expected 1 got %d", len(args))
		}
		for _, clusterIds := range args {
			queries = append(queries, ocmutils.GenerateQuery(clusterIds))
		}

		if len(queries) > 0 {
			if len(filterParams) > 0 {
				log.Warnf("A cluster identifier was passed with the '-q' flag. This will apply logical AND between the search query and the cluster given, potentially resulting in no matches")
			}
			filterParams = append(filterParams, strings.Join(queries, " or "))
		}

		// For every '-p' flag, replace its related placeholder in the template & filterFiles
		for k := range userParameterNames {
			replaceFlags(userParameterNames[k], userParameterValues[k])
		}

		// Check if there are any remaining placeholders in the template that are not replaced by a parameter,
		// excluding '${CLUSTER_UUID}' which will be replaced for each cluster later
		checkLeftovers([]string{"${CLUSTER_UUID}"})

		// Create an OCM client to talk to the cluster API
		// the user has to be logged in (e.g. 'ocm login')
		ocmClient := ocmutils.CreateConnection()
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
				query = append(query, ocmutils.GenerateQuery(cluster))
			}
			filterParams = append(filterParams, strings.Join(query, " or "))
		}

		clusters, err := ocmutils.ApplyFilters(ocmClient, filterParams)

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
			err = ctlutil.ConfirmSend()
			if err != nil {
				log.Fatal(err)
			}
		}

		// Handler if the program terminates abruptly
		go func() {
			sigchan := make(chan os.Signal, 1)
			signal.Notify(sigchan, os.Interrupt)
			<-sigchan

			// perform final cleanup actions
			log.Error("program abruptly terminated, performing clean-up...")
			cleanUp(clusters)
			log.Fatal("servicelog post command terminated")
		}()

		for _, cluster := range clusters {
			request, err := createPostRequest(ocmClient, cluster)
			if err != nil {
				failedClusters[cluster.ExternalID()] = err.Error()
				continue
			}

			response, err := sendRequest(request)
			if err != nil {
				failedClusters[cluster.ExternalID()] = err.Error()
				continue
			}

			check(response, Message)
		}

		printPostOutput()
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
	postCmd.Flags().StringVarP(&clustersFile, "clusters-file", "c", clustersFile, `Read a list of clusters to post the servicelog to. the format of the file is: {"clusters":["$CLUSTERID"]}`)
	postCmd.Flags().BoolVarP(&internalOnly, "internal", "i", false, "Internal only service log. Use MESSAGE for template parameter (eg. -p MESSAGE='My super secret message').")
}

// parseUserParameters parse all the '-p FOO=BAR' parameters and checks for syntax errors
func parseUserParameters() {
	for _, v := range templateParams {
		if !strings.Contains(v, "=") {
			log.Fatalf("Wrong syntax of '-p' flag. Please use it like this: '-p FOO=BAR'")
		}

		param := strings.SplitN(v, "=", 2)
		if param[0] == "" || param[1] == "" {
			log.Fatalf("Wrong syntax of '-p' flag. Please use it like this: '-p FOO=BAR'")
		}

		userParameterNames = append(userParameterNames, fmt.Sprintf("${%v}", param[0]))
		userParameterValues = append(userParameterValues, param[1])
	}
}

// accessFile returns the contents of a local file or url, and any errors encountered
func accessFile(filePath string) ([]byte, error) {

	if utils.IsValidUrl(filePath) {
		urlPage, _ := url.Parse(filePath)
		if err := utils.IsOnline(*urlPage); err != nil {
			return nil, fmt.Errorf("host %q is not accessible", filePath)
		}
		return utils.CurlThis(urlPage.String())
	}

	filePath = filepath.Clean(filePath)
	if utils.FileExists(filePath) {
		// template is file on the disk
		file, err := ioutil.ReadFile(filePath) //#nosec G304 -- Potential file inclusion via variable
		if err != nil {
			return file, fmt.Errorf("cannot read the file.\nError: %q", err)
		}
		return file, nil
	}
	if utils.FolderExists(filePath) {
		return nil, fmt.Errorf("the provided path %q is a directory, not a file", filePath)
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
	if internalOnly {
		// fixed template for internal service logs
		messageTemplate := []byte(`
		{
			"severity": "Info",
			"service_name": "SREManualAction",
			"summary": "INTERNAL",
			"description": "${MESSAGE}",
			"internal_only": true
		}
		`)
		if err := parseTemplate(messageTemplate); err != nil {
			log.Fatalf("Cannot not parse the JSON internal message template.\nError: %q\n", err)
		}
		return
	}

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
		table.AddRow([]string{cluster.Name(), cluster.ID(), string(cluster.State()), cluster.OpenshiftVersion(), cluster.CloudProvider().ID(), cluster.Region().ID()})
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

func createPostRequest(ocmClient *sdk.Connection, cluster *v1.Cluster) (request *sdk.Request, err error) {
	// Create and populate the request:
	request = ocmClient.Post()
	err = arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		return nil, fmt.Errorf("cannot parse API path '%s': %v", targetAPIPath, err)
	}

	Message.ClusterUUID = cluster.ExternalID()
	Message.ClusterID = cluster.ID()
	Message.InternalOnly = internalOnly
	if subscription := cluster.Subscription(); subscription != nil {
		Message.SubscriptionID = cluster.Subscription().ID()
	}

	messageBytes, err := json.Marshal(Message)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal template to json: %v", err)
	}

	request.Bytes(messageBytes)
	return request, nil
}

// listMessagedClusters prints all the clusters a service log was tried to be posted.
func listMessagedClusters(clusters map[string]string) error {
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"ID", "Status"})

	for id, status := range clusters {
		table.AddRow([]string{id, status})
	}

	// New row for better readability
	table.AddRow([]string{})

	return table.Flush()
}

// printPostOutput prints the main servicelog post output.
func printPostOutput() {
	output := fmt.Sprintf("Success: %d, Failed: %d\n", len(successfulClusters), len(failedClusters))
	log.Infoln(output + "\n")

	// Print if any service logs were successfully sent
	if len(successfulClusters) > 0 {
		log.Infoln("Successful clusters:")
		if err := listMessagedClusters(successfulClusters); err != nil {
			log.Fatalf("Cannot list successful clusters: %q", err)
		}
	}

	// Print if there were failures while sending service logs
	if len(failedClusters) > 0 {
		log.Infoln("Failed clusters:")
		if err := listMessagedClusters(failedClusters); err != nil {
			log.Fatalf("Cannot list failed clusters: %q", err)
		}
	}
}

// cleanUp performs final actions in case of program termination.
func cleanUp(clusters []*v1.Cluster) {
	for _, cluster := range clusters {
		if _, ok := successfulClusters[cluster.ExternalID()]; !ok {
			failedClusters[cluster.ExternalID()] = "cannot send message due to program interruption"
		}
	}

	printPostOutput()
}
