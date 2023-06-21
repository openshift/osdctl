package mgmt

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	jiracontext "github.com/openshift/osdctl/cmd/cluster"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/go-github/v43/github"
	"golang.org/x/oauth2"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	GithubTokenConfigKey = "github_token"
)

type exportJiraToSheetOptions struct {
}

// newCmdExportJiraToSheet searches JIRA based on provided jql and provides the card info + linked PRs in Google Sheets formating
func newCmdExportJiraToSheet() *cobra.Command {
	ops := newExportJiraToSheetOptions()
	exportCmd := &cobra.Command{
		Use:               "ExportJiraToSheet",
		Short:             "Exports JIRA to Spreadsheet",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run())
		},
	}

	return exportCmd
}

func newExportJiraToSheetOptions() *exportJiraToSheetOptions {
	return &exportJiraToSheetOptions{}
}

func (o *exportJiraToSheetOptions) run() error {

	hsBugs := HSBugs{
		issues: []jira.Issue{},
		prs:    make(map[string][]string),
	}

	jql := `labels in (ServiceDeliveryBlocker, ServiceDeliveryImpact) AND labels in (sd-hypershift) ORDER BY created ASC`
	issues, err := getJIRACards(jql)
	if err != nil {
		return err
	}
	hsBugs.issues = issues

	// Now I have all the issues, time to build up the PRs
	fmt.Println("Grabbing GH PRs via webscrapper")
	for _, issue := range hsBugs.issues {
		prs, err := scrapePRInfoFromJIRA(issue.Key)
		if err != nil {
			return err
		}
		if len(prs) > 0 {
			hsBugs.prs[issue.Key] = prs
		}
	}

	fmt.Println("Printing Hyperlinks")
	// Print out the hyperlinks to JIRA first as the formatting breaks if we try paste the entire sheet at once
	for _, issue := range hsBugs.issues {
		jiraKeyURL := fmt.Sprintf("=HYPERLINK(%q, %q)\n", "https://issues.redhat.com/browse/"+issue.Key, issue.Key)
		// We want to have a row per PR, so if a card has multiple PRs, we'll print the key multiple times.
		if vals, ok := hsBugs.prs[issue.Key]; ok {
			for i := 0; i < len(vals); i++ {
				fmt.Print(jiraKeyURL)
			}
		} else {
			//fmt.Printf("=HYPERLINK(%q, %q)\n", "https://issues.redhat.com/browse/"+issue.Key, issue.Key)
			fmt.Print(jiraKeyURL)
		}
	}

	printHSBugs(hsBugs)

	return nil
}

type HSBugs struct {
	issues []jira.Issue
	prs    map[string][]string
}

func getJIRACards(jql string) ([]jira.Issue, error) {
	jiraClient, err := jiracontext.GetJiraClient()
	if err != nil {
		return nil, fmt.Errorf("Error connecting to jira: %v", err)
	}

	lastIssue := 0
	fmt.Println("Searching for JIRA cards")
	var result []jira.Issue
	for {
		// Add a Search option which accepts maximum amount (1000)
		opt := &jira.SearchOptions{
			MaxResults: 1000,      // Max amount
			StartAt:    lastIssue, // Make sure we start grabbing issues from last checkpoint
		}
		issues, resp, err := jiraClient.Issue.Search(jql, opt)
		if err != nil {
			return nil, err
		}
		// Grab total amount from response
		total := resp.Total
		if issues == nil {
			// init the issues array with the correct amount of length
			result = make([]jira.Issue, 0, total)
		}

		// Append found issues to result
		result = append(result, issues...)
		// Update checkpoint index by using the response StartAt variable
		lastIssue = resp.StartAt + len(issues)
		// Check if we have reached the end of the issues
		if lastIssue >= total {
			break
		}
	}

	fmt.Println("Getting each JIRA card individually as client needs to get metadata")
	var issues []jira.Issue
	for _, issue := range result {
		issue, _, _ := jiraClient.Issue.Get(issue.Key, nil)
		issues = append(issues, *issue)
	}
	return issues, nil
}

func scrapePRInfoFromJIRA(jiraKey string) ([]string, error) {
	url := "https://issues.redhat.com/browse/" + jiraKey
	cardResponse, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer cardResponse.Body.Close()

	if cardResponse.StatusCode != http.StatusOK {
		log.Fatalf("Request failed with status code: %d", cardResponse.StatusCode)
		return nil, err // TODO fix
	}

	cardDoc, err := goquery.NewDocumentFromReader(cardResponse.Body)
	if err != nil {
		return nil, err
	}

	// Scrape GitHub PRs
	var prs []string
	cardDoc.Find(".link-content").Each(func(i int, s *goquery.Selection) {
		prURL, _ := s.Find(".link-title").Attr("href")
		if strings.Contains(prURL, "github") { //|| strings.Contains(prURL, "gitlab") {
			prs = append(prs, prURL)
		}
	})
	return prs, nil
}

func getGithubPrInfo(ghClient *github.Client, prURL string) (string, string) {
	parsedURL, _ := url.Parse(prURL)

	// Extract the repository path
	repoPath := path.Dir(parsedURL.Path)
	// Extract the repository name
	repoParts := strings.Split(repoPath, "/")
	owner := repoParts[1]
	repo := repoParts[2]

	prNumber := atoi(path.Base(parsedURL.Path))

	pr, _, err := ghClient.PullRequests.Get(context.TODO(), owner, repo, prNumber)
	if err != nil {
		return "", ""
	}

	prCreationDate := pr.GetCreatedAt().Format("2006-01-02")
	var prMergeDate string
	if pr.GetMergedAt().IsZero() {
		prMergeDate = "TBD"
	} else {
		prMergeDate = pr.GetMergedAt().Format("2006-01-02")
	}

	return prCreationDate, prMergeDate
}

func printHSBugs(hsBugs HSBugs) {
	ghClient := CreateGitHubClient(context.TODO())

	for _, i := range hsBugs.issues {
		var email string
		if i.Fields.Assignee == nil {
			email = "unassigned"
		} else {
			email = i.Fields.Assignee.EmailAddress
		}

		createdTime := time.Time(i.Fields.Created)
		formattedcreatedTime := createdTime.Format("2006-01-02")

		if prs, ok := hsBugs.prs[i.Key]; ok {
			for _, prURL := range prs {
				if strings.Contains(prURL, "github") {
					prCreationDate, prMergeDate := getGithubPrInfo(ghClient, prURL)
					fmt.Printf(
						"%s,%s,%s,%q,%s,TBD,%s,%s,%s,%s\n",
						i.Fields.Type.Name,
						strings.ToUpper(i.Fields.Priority.Name),
						strings.ToUpper(i.Fields.Status.Name),
						i.Fields.Summary,
						email,
						prURL,
						formattedcreatedTime,
						prCreationDate,
						prMergeDate,
					)
				}
				// TODO gitlab
			}
		} else {
			// No PRs
			fmt.Printf(
				"%s,%s,%s,%q,%s,TBD,None,%s,None,None\n",
				i.Fields.Type.Name,
				strings.ToUpper(i.Fields.Priority.Name),
				strings.ToUpper(i.Fields.Status.Name),
				i.Fields.Summary,
				email,
				formattedcreatedTime,
			)
		}
	}
}

func CreateGitHubClient(ctx context.Context) *github.Client {
	if !viper.IsSet(GithubTokenConfigKey) {
		return nil // fmt.Errorf("key %s is not set in config file", GithubTokenConfigKey)
	}

	token := viper.GetString(GithubTokenConfigKey)

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}

func atoi(s string) int {
	n := 0
	for _, ch := range []byte(s) {
		n = n*10 + int(ch-'0')
	}
	return n
}
