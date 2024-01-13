package servicelog

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/utils/strings/slices"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/servicelog"
	"github.com/openshift/osdctl/internal/utils"
	"github.com/openshift/osdctl/pkg/printer"
	ocmutils "github.com/openshift/osdctl/pkg/utils"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type PostCmdOptions struct {
	Message         servicelog.Message
	ClustersFile    servicelog.ClustersFile
	Template        string
	TemplateParams  []string
	filterFiles     []string // Path to filter file
	filtersFromFile string   // Contents of filterFiles
	isDryRun        bool
	skipPrompts     bool
	clustersFile    string
	internalOnly    bool
	ClusterId       string

	// Messaged clusters
	successfulClusters map[string]string
	failedClusters     map[string]string
}

const documentationBaseURL = "https://docs.openshift.com"

func newPostCmd() *cobra.Command {
	var opts = PostCmdOptions{}
	postCmd := &cobra.Command{
		Use:   "post CLUSTER_ID",
		Short: "Send a servicelog message to a given cluster",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				opts.ClusterId = args[0]
			}
			cmdutil.CheckErr(opts.Run())
		},
	}

	// define required flags
	postCmd.Flags().StringVarP(&opts.Template, "template", "t", "", "Message template file or URL")
	postCmd.Flags().StringArrayVarP(&opts.TemplateParams, "param", "p", opts.TemplateParams, "Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.")
	postCmd.Flags().BoolVarP(&opts.isDryRun, "dry-run", "d", false, "Dry-run - print the service log about to be sent but don't send it.")
	postCmd.Flags().StringArrayVarP(&filterParams, "query", "q", filterParams, "Specify a search query (eg. -q \"name like foo\") for a bulk-post to matching clusters.")
	postCmd.Flags().BoolVarP(&opts.skipPrompts, "yes", "y", false, "Skips all prompts.")
	postCmd.Flags().StringArrayVarP(&opts.filterFiles, "query-file", "f", []string{}, "File containing search queries to apply. All lines in the file will be concatenated into a single query. If this flag is called multiple times, every file's search query will be combined with logical AND.")
	postCmd.Flags().StringVarP(&opts.clustersFile, "clusters-file", "c", "", `Read a list of clusters to post the servicelog to. the format of the file is: {"clusters":["$CLUSTERID"]}`)
	postCmd.Flags().BoolVarP(&opts.internalOnly, "internal", "i", false, "Internal only service log. Use MESSAGE for template parameter (eg. -p MESSAGE='My super secret message').")

	return postCmd
}

func (o *PostCmdOptions) Init() error {
	userParameterNames = []string{}
	userParameterValues = []string{}
	filterParams = []string{}
	o.successfulClusters = make(map[string]string)
	o.failedClusters = make(map[string]string)
	return nil
}

func (o *PostCmdOptions) Validate() error {
	if o.ClusterId == "" && len(filterParams) == 0 && o.clustersFile == "" {
		return fmt.Errorf("no cluster identifier has been found")
	}
	return nil
}

func (o *PostCmdOptions) CheckServiceLogsLastHour() bool {
	getAllMessages := false      // we need just manual entries
	getInternalLogsOnly := false // we need all messages
	numberOfHours := 1           // number of hours we need to wait for svc logs
	timeStampToCompare := time.Now().Add(-time.Hour * time.Duration(numberOfHours))
	serviceLogs, err := GetServiceLogsSince(o.ClusterId, timeStampToCompare, getAllMessages, getInternalLogsOnly)
	if err != nil {
		log.Fatalf("failed to fetch service logs: %q", err)
	}
	if len(serviceLogs) > 0 {
		for _, svclog := range serviceLogs {
			fmt.Println("Below service Log has been subitted in last 60 minutes\nDescription: ", svclog.Description())
		}
		return true
	}
	return false
}

func (o *PostCmdOptions) Run() error {
	if err := o.Init(); err != nil {
		return err
	}
	if err := o.Validate(); err != nil {
		return err
	}
	if term.IsTerminal(int(os.Stdout.Fd())) && o.CheckServiceLogsLastHour() {
		if !ocmutils.ConfirmPrompt() {
			return nil
		}
	}
	o.parseUserParameters() // parse all the '-p' user flags
	o.readFilterFile()      // parse the ocm filters in file provided via '-f' flag
	o.readTemplate()        // parse the given JSON template provided via '-t' flag

	// For every '-p' flag, replace its related placeholder in the template & filterFiles
	for k := range userParameterNames {
		o.replaceFlags(userParameterNames[k], userParameterValues[k])
	}

	// Check if there are any remaining placeholders in the template that are not replaced by a parameter,
	// excluding '${CLUSTER_UUID}' which will be replaced for each cluster later
	o.checkLeftovers([]string{"${CLUSTER_UUID}"})

	// Create an OCM client to talk to the cluster API
	// the user has to be logged in (e.g. 'ocm login')
	ocmClient, err := ocmutils.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		if err := ocmClient.Close(); err != nil {
			log.Errorf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Retrieve matching clusters
	if o.filtersFromFile != "" {
		if len(filterParams) != 0 {
			log.Warnf("Search queries were passed using both the '-q' and '-f' flags. This will apply logical AND between the queries, potentially resulting in no matches")
		}
		filters := strings.Join(strings.Split(strings.TrimSpace(o.filtersFromFile), "\n"), " ")
		filterParams = append(filterParams, filters)
	}
	var queries []string
	if o.clustersFile != "" {
		contents, err := o.accessFile(o.clustersFile)
		if err != nil {
			log.Fatalf("Cannot read file %s: %q", o.clustersFile, err)
		}
		err = o.parseClustersFile(contents)
		if err != nil {
			log.Fatalf("Cannot parse file %s: %q", o.clustersFile, err)
		}
		for i := range o.ClustersFile.Clusters {
			cluster := o.ClustersFile.Clusters[i]
			queries = append(queries, ocmutils.GenerateQuery(cluster))
		}
	} else {
		queries = append(queries, ocmutils.GenerateQuery(o.ClusterId))
	}
	if len(queries) > 0 {
		if len(filterParams) > 0 {
			log.Warnf("A cluster identifier was passed with the '-q' flag. This will apply logical AND between the search query and the cluster given, potentially resulting in no matches")
		}
	}

	filterParams = append(filterParams, strings.Join(queries, " or "))
	clusters, err := ocmutils.ApplyFilters(ocmClient, filterParams)

	if err != nil {
		log.Fatalf("Cannot retrieve clusters: %q", err)
	} else if len(clusters) < 1 {
		log.Fatalf("No clusters match the given parameters.")
	}

	log.Infoln("The following clusters match the given parameters:")
	if err := o.printClusters(clusters); err != nil {
		log.Fatalf("Could not print matching clusters: %q", err)
	}

	log.Infoln("The following template will be sent:")
	if err := o.printTemplate(); err != nil {
		log.Errorf("Cannot read generated template: %q", err)
	}

	// If this is a dry-run, don't proceed further.
	if o.isDryRun {
		return nil
	}

	if !o.skipPrompts {
		if !ocmutils.ConfirmPrompt() {
			return nil
		}
	}

	// Handler if the program terminates abruptly
	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, os.Interrupt)
		<-sigchan

		// perform final cleanup actions
		log.Error("program abruptly terminated, performing clean-up...")
		o.cleanUp(clusters)
		log.Fatal("servicelog post command terminated")
	}()

	// cluster type for which documentation link is provided in servicelog description
	docClusterType := getDocClusterType(o.Message.Description)

	for _, cluster := range clusters {
		request, err := o.createPostRequest(ocmClient, cluster)
		if err != nil {
			o.failedClusters[cluster.ExternalID()] = err.Error()
			continue
		}

		// if servicelog description contains a documentation link, verify that
		// documentation link matches the cluster product (rosa, dedicated)
		if !o.skipPrompts && docClusterType != "" {
			clusterType := cluster.Product().ID()

			if docClusterType != clusterType {
				log.Warn("The documentation mentioned in the servicelog is for '", docClusterType, "' while the product is '", clusterType, "'.")
				if !ocmutils.ConfirmPrompt() {
					log.Info("Skipping cluster ID: ", cluster.ID(), ", Name: ", cluster.Name())
					continue
				}
			}
		}

		response, err := ocmutils.SendRequest(request)
		if err != nil {
			o.failedClusters[cluster.ExternalID()] = err.Error()
			continue
		}

		o.check(response, o.Message)
	}

	o.printPostOutput()
	return nil
}

// if servicelog description contains documentation link, parse and return the cluster type from the url
func getDocClusterType(message string) string {

	if strings.Contains(message, documentationBaseURL) {
		pattern := `https://docs.openshift.com/([^/]+)/`
		re := regexp.MustCompile(pattern)
		match := re.FindStringSubmatch(message)
		if len(match) >= 2 {
			productType := match[1]
			if productType == "dedicated" {
				// the documentation urls for osd use "dedicated" as the differentiator
				// e.g. https://docs.openshift.com/dedicated/welcome/index.html
				// for proper comparison with cluster product types, return "osd"
				// where "dedicated" is used in the documentation urls
				productType = "osd"
			}
			return productType
		}
	}
	return ""
}

func (o *PostCmdOptions) check(response *sdk.Response, clusterMessage servicelog.Message) {
	body := response.Bytes()
	if response.Status() < 400 {
		_, err := validateGoodResponse(body, clusterMessage)
		if err != nil {
			o.failedClusters[clusterMessage.ClusterUUID] = err.Error()
		} else {
			o.successfulClusters[clusterMessage.ClusterUUID] = fmt.Sprintf("Message has been successfully sent to %s", clusterMessage.ClusterUUID)
		}
	} else {
		badReply, err := validateBadResponse(body)
		if err != nil {
			o.failedClusters[clusterMessage.ClusterUUID] = err.Error()
		} else {
			o.failedClusters[clusterMessage.ClusterUUID] = badReply.Reason
		}
	}
}

// parseUserParameters parse all the '-p FOO=BAR' parameters and checks for syntax errors
func (o *PostCmdOptions) parseUserParameters() {
	for _, v := range o.TemplateParams {
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
func (o *PostCmdOptions) accessFile(filePath string) ([]byte, error) {

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
		file, err := os.ReadFile(filePath) //#nosec G304 -- Potential file inclusion via variable
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
func (o *PostCmdOptions) parseClustersFile(jsonFile []byte) error {
	return json.Unmarshal(jsonFile, &o.ClustersFile)
}

// parseTemplate reads the template file into a JSON struct
func (o *PostCmdOptions) parseTemplate(jsonFile []byte) error {
	return json.Unmarshal(jsonFile, &o.Message)
}

// readTemplate loads the template into the Message variable
func (o *PostCmdOptions) readTemplate() {
	if o.internalOnly {
		// fixed template for internal service logs
		messageTemplate := []byte(`
		{
			"severity": "Info",
			"service_name": "SREManualAction",
			"summary": "INTERNAL ONLY, DO NOT SHARE WITH CUSTOMER",
			"description": "${MESSAGE}",
			"internal_only": true
		}
		`)
		if err := o.parseTemplate(messageTemplate); err != nil {
			log.Fatalf("Cannot not parse the JSON internal message template.\nError: %q\n", err)
		}
		return
	}

	if o.Template == "" {
		log.Fatalf("Template file is not provided. Use '-t' to fix this.")
	}

	file, err := o.accessFile(o.Template)
	if err != nil { // check if this URL or file and if we can access it
		log.Fatal(err)
	}

	if err = o.parseTemplate(file); err != nil {
		log.Fatalf("Cannot not parse the JSON template.\nError: %q\n", err)
	}
}

func (o *PostCmdOptions) readFilterFile() {
	if len(o.filterFiles) < 1 {
		// No filterFiles specified in args
		return
	}

	for _, filterFile := range o.filterFiles {
		fileContents, err := o.accessFile(filterFile)
		if err != nil {
			log.Fatal(err)
		}

		if o.filtersFromFile == "" {
			o.filtersFromFile = "(" + strings.TrimSpace(string(fileContents)) + ")"
		} else {
			o.filtersFromFile = o.filtersFromFile + " and (" + strings.TrimSpace(string(fileContents)) + ")"
		}
	}
}

func (o *PostCmdOptions) FindLeftovers(s string) (matches []string) {
	r := regexp.MustCompile(`\${[^{}]*}`)
	matches = r.FindAllString(s, -1)
	return matches
}

func (o *PostCmdOptions) checkLeftovers(excludes []string) {
	unusedParameters, _ := o.Message.FindLeftovers()
	unusedParameters = append(unusedParameters, o.FindLeftovers(o.filtersFromFile)...)

	var numberOfMissingParameters int
	for _, v := range unusedParameters {
		// Ignore parameters in the exclude list, ie ${CLUSTER_UUID}, which will be replaced later for each cluster a servicelog is sent to

		if !slices.Contains(excludes, v) {
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

func (o *PostCmdOptions) replaceFlags(flagName string, flagValue string) {
	if flagValue == "" {
		log.Fatalf("The selected template is using '%[1]s' parameter, but '%[1]s' flag was not set. Use '-p %[1]s=\"FOOBAR\"' to fix this.", flagName)
	}

	found := false
	if o.Message.SearchFlag(flagName) {
		found = true
		o.Message.ReplaceWithFlag(flagName, flagValue)
	}
	if strings.Contains(o.filtersFromFile, flagName) {
		found = true
		o.filtersFromFile = strings.ReplaceAll(o.filtersFromFile, flagName, flagValue)
	}

	if !found {
		log.Fatalf("The selected template is not using '%s' parameter, but '--param' flag was set. Do not use '-p %s=%s' to fix this.", flagName, flagName, flagValue)
	}
}

func (o *PostCmdOptions) printClusters(clusters []*v1.Cluster) (err error) {
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Name", "ID", "State", "Version", "Cloud Provider", "Region"})
	for _, cluster := range clusters {
		table.AddRow([]string{cluster.Name(), cluster.ID(), string(cluster.State()), cluster.OpenshiftVersion(), cluster.CloudProvider().ID(), cluster.Region().ID()})
	}

	// Add empty row for readability
	table.AddRow([]string{})
	return table.Flush()
}

func (o *PostCmdOptions) printTemplate() (err error) {
	exampleMessage, err := json.Marshal(o.Message)
	if err != nil {
		return err
	}
	return dump.Pretty(os.Stdout, exampleMessage)
}

func (o *PostCmdOptions) createPostRequest(ocmClient *sdk.Connection, cluster *v1.Cluster) (request *sdk.Request, err error) {
	// Create and populate the request:
	request = ocmClient.Post()
	err = arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		return nil, fmt.Errorf("cannot parse API path '%s': %v", targetAPIPath, err)
	}

	o.Message.ClusterUUID = cluster.ExternalID()
	o.Message.ClusterID = cluster.ID()
	o.Message.InternalOnly = o.internalOnly
	if subscription := cluster.Subscription(); subscription != nil {
		o.Message.SubscriptionID = cluster.Subscription().ID()
	}

	messageBytes, err := json.Marshal(o.Message)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal template to json: %v", err)
	}

	request.Bytes(messageBytes)
	return request, nil
}

// listMessagedClusters prints all the clusters a service log was tried to be posted.
func (o *PostCmdOptions) listMessagedClusters(clusters map[string]string) error {
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
func (o *PostCmdOptions) printPostOutput() {
	output := fmt.Sprintf("Success: %d, Failed: %d\n", len(o.successfulClusters), len(o.failedClusters))
	log.Infoln(output + "\n")

	// Print if any service logs were successfully sent
	if len(o.successfulClusters) > 0 {
		log.Infoln("Successful clusters:")
		if err := o.listMessagedClusters(o.successfulClusters); err != nil {
			log.Fatalf("Cannot list successful clusters: %q", err)
		}
	}

	// Print if there were failures while sending service logs
	if len(o.failedClusters) > 0 {
		log.Infoln("Failed clusters:")
		if err := o.listMessagedClusters(o.failedClusters); err != nil {
			log.Fatalf("Cannot list failed clusters: %q", err)
		}
	}
}

// cleanUp performs final actions in case of program termination.
func (o *PostCmdOptions) cleanUp(clusters []*v1.Cluster) {
	for _, cluster := range clusters {
		if _, ok := o.successfulClusters[cluster.ExternalID()]; !ok {
			o.failedClusters[cluster.ExternalID()] = "cannot send message due to program interruption"
		}
	}

	o.printPostOutput()
}
