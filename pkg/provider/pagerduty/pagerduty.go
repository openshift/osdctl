package pagerduty

// Generate client mocks for testing
//go:generate mockgen -source=pagerduty.go -package=mock -destination=mocks/pagerduty_mock.go

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
)

const (
	PagerDutyUserTokenConfigKey  = "pd_user_token"  // #nosec G101
	PagerDutyOauthTokenConfigKey = "pd_oauth_token" // #nosec G101
	PagerDutyTeamIDsKey          = "team_ids"
)

type IncidentOccurrenceTracker struct {
	IncidentName   string
	Count          int
	LastOccurrence string
}

type pdClientInterface interface {
	ListIncidentsWithContext(context.Context, pd.ListIncidentsOptions) (*pd.ListIncidentsResponse, error)
	ListIncidentAlertsWithContext(context.Context, string, pd.ListIncidentAlertsOptions) (*pd.ListAlertsResponse, error)
	ListServicesWithContext(context.Context, pd.ListServiceOptions) (*pd.ListServiceResponse, error)
}

type client struct {
	pdclient   pdClientInterface
	teamIds    []string
	userToken  string
	oauthToken string
}

func NewClient() *client {
	return &client{}
}

func (c *client) WithTeamIdList(teamIds []string) *client {
	c.teamIds = teamIds
	return c
}

func (c *client) WithUserToken(token string) *client {
	c.userToken = token
	return c
}

func (c *client) WithOauthToken(token string) *client {
	c.oauthToken = token
	return c
}

func (c *client) Init() (*client, error) {
	err := c.buildClient()
	return c, err
}

func (c *client) buildClient() error {
	// Leave both here to keep some backwards compatibility
	// I'm not sure what the difference is, but if both are provided let's just
	// default to using the User Token over the oauth token
	if c.userToken != "" {
		c.pdclient = pd.NewClient(c.userToken)
		return nil
	}

	if c.oauthToken != "" {
		c.pdclient = pd.NewOAuthClient(c.oauthToken)
		return nil
	}

	return fmt.Errorf("Could not build PagerDuty Client - No configured tokens")
}

// GetServiceIDs searches service names within the configured teams. Callers choose
// the query according to cluster routing; Classic services use the DNS base domain.
func (c *client) GetServiceIDs(query string) ([]string, error) {
	lsResponse, err := c.pdclient.ListServicesWithContext(context.TODO(), pd.ListServiceOptions{Query: query, TeamIDs: c.teamIds})
	if err != nil {
		return []string{}, fmt.Errorf("failed to ListServicesWithContext: %w", err)
	}

	var serviceIDS []string
	for _, service := range lsResponse.Services {
		serviceIDS = append(serviceIDS, service.ID)
	}

	return serviceIDS, nil
}

func (c *client) getServices(ctx context.Context, query string) ([]pd.Service, error) {
	lsResponse, err := c.pdclient.ListServicesWithContext(ctx, pd.ListServiceOptions{Query: query, TeamIDs: c.teamIds, Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("failed to ListServicesWithContext: %w", err)
	}
	return lsResponse.Services, nil
}

// GetHCPServiceIDs selects the dedicated critical and warning services for an
// RHOBS cell. Matching requires HCP, environment, severity, and the cell's
// static region, so an unrelated service cannot match on a bare region string.
func (c *client) GetHCPServiceIDs(ctx context.Context, rhobsCell, environment string) ([]string, error) {
	cellName, err := rhobsCellName(rhobsCell)
	if err != nil {
		return nil, err
	}

	region := regexp.MustCompile(`-\d+$`).ReplaceAllString(cellName, "")
	services, err := c.getServices(ctx, "hcp")
	if err != nil {
		return nil, err
	}

	serviceIDs := make([]string, 0, 2)
	// Keep the paging service first, followed by the warning service. This makes
	// the output deterministic and keeps the critical service as the primary
	// service shown by cluster context.
	for _, severity := range []string{"critical", "warning"} {
		for _, service := range services {
			if hcpServiceNameMatches(service.Name, region, environment, severity) {
				serviceIDs = append(serviceIDs, service.ID)
			}
		}
	}
	// Stage and integration use one non-severity-specific HCP service per
	// region, unlike production's separate critical and warning services.
	for _, service := range services {
		if hcpServiceNameMatches(service.Name, region, environment, "") {
			serviceIDs = append(serviceIDs, service.ID)
		}
	}
	return serviceIDs, nil
}

func hcpServiceNameMatches(name, region, environment, severity string) bool {
	name = strings.ToLower(name)
	if !hcpServiceBaseNameMatches(name, region, environment) {
		return false
	}
	if severity == "" {
		if environment == "production" {
			return false
		}
		return !containsServiceToken(name, "critical") && !containsServiceToken(name, "warning")
	}
	if environment != "production" {
		return false
	}
	return containsServiceToken(name, severity)
}

func hcpServiceBaseNameMatches(name, region, environment string) bool {
	return containsServiceToken(name, "hcp") &&
		(containsServiceToken(name, environment) || environment == "production" && containsServiceToken(name, "prod")) &&
		strings.Contains(compactServiceToken(name), compactServiceToken(region))
}

func compactServiceToken(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsServiceToken(name, token string) bool {
	pattern := `(?:^|[^a-z0-9])` + regexp.QuoteMeta(strings.ToLower(token)) + `(?:$|[^a-z0-9])`
	return regexp.MustCompile(pattern).MatchString(name)
}

func rhobsCellName(rhobsCell string) (string, error) {
	parsed, err := url.Parse(rhobsCell)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", errors.New("invalid RHOBS cell URL")
	}
	return strings.Split(parsed.Hostname(), ".")[0], nil
}

// GetFiringAlertsForServices returns all active incidents for the services.
// Shared services require callers to filter the results for the desired cluster.
func (c *client) GetFiringAlertsForServices(pdServiceIDs []string) (map[string][]pd.Incident, error) {
	return c.GetFiringAlertsForServicesWithContext(context.Background(), pdServiceIDs)
}

// GetFiringAlertsForServicesWithContext is the context-aware variant used by
// commands that need cancellation and deadlines.
func (c *client) GetFiringAlertsForServicesWithContext(ctx context.Context, pdServiceIDs []string) (map[string][]pd.Incident, error) {
	incidents := map[string][]pd.Incident{}

	var incidentLimit uint = 25
	for _, pdServiceID := range pdServiceIDs {
		var incidentListOffset uint = 0
		for {
			listIncidentsResponse, err := c.pdclient.ListIncidentsWithContext(
				ctx,
				pd.ListIncidentsOptions{
					ServiceIDs: []string{pdServiceID},
					Statuses:   []string{"triggered", "acknowledged"},
					SortBy:     "urgency:DESC",
					Limit:      incidentLimit,
					Offset:     incidentListOffset,
				},
			)
			if err != nil {
				return nil, err
			}

			incidents[pdServiceID] = append(incidents[pdServiceID], listIncidentsResponse.Incidents...)

			if !listIncidentsResponse.More {
				break
			}
			incidentListOffset += incidentLimit
		}
	}
	return incidents, nil
}

// GetFiringAlertsForHCPCluster returns active incidents whose alert details
// contain the hosted cluster's external UUID. RHOBS sends this as
// details.cluster_id; custom_details is supported for older integrations.
func (c *client) GetFiringAlertsForHCPCluster(ctx context.Context, pdServiceIDs []string, clusterExternalID string) (map[string][]pd.Incident, error) {
	allIncidents, err := c.GetFiringAlertsForServicesWithContext(ctx, pdServiceIDs)
	if err != nil {
		return nil, err
	}

	matched := make(map[string][]pd.Incident)
	for serviceID, incidents := range allIncidents {
		for _, incident := range incidents {
			alerts, alertErr := c.pdclient.ListIncidentAlertsWithContext(ctx, incident.ID, pd.ListIncidentAlertsOptions{Limit: 100})
			if alertErr != nil {
				return nil, fmt.Errorf("failed to list alerts for incident %s: %w", incident.ID, alertErr)
			}
			for _, alert := range alerts.Alerts {
				if alertDetailsContainClusterID(alert.Body, clusterExternalID) {
					matched[serviceID] = append(matched[serviceID], incident)
					break
				}
			}
		}
	}
	return matched, nil
}

func alertDetailsContainClusterID(value map[string]interface{}, clusterID string) bool {
	var visit func(interface{}, string) bool
	visit = func(current interface{}, key string) bool {
		switch typed := current.(type) {
		case map[string]interface{}:
			for childKey, child := range typed {
				if visit(child, childKey) {
					return true
				}
			}
		case []interface{}:
			for _, child := range typed {
				if visit(child, key) {
					return true
				}
			}
		case string:
			return (strings.EqualFold(key, "cluster_id") ||
				strings.EqualFold(key, "external_cluster_id")) && typed == clusterID
		}
		return false
	}
	return visit(value, "")
}

// GetHistoricalAlertsForServices aggregates incidents across each service without
// filtering by cluster, so callers must select services dedicated to that cluster.
func (c *client) GetHistoricalAlertsForServices(pdServiceIDs []string) (map[string][]*IncidentOccurrenceTracker, error) {
	return c.GetHistoricalAlertsForServicesWithContext(context.Background(), pdServiceIDs)
}

// GetHistoricalAlertsForServicesWithContext is the context-aware variant used
// by commands that need cancellation and deadlines.
func (c *client) GetHistoricalAlertsForServicesWithContext(ctx context.Context, pdServiceIDs []string) (map[string][]*IncidentOccurrenceTracker, error) {

	var currentOffset uint
	var limit uint = 100
	var incidents []pd.Incident
	incidentMap := map[string][]*IncidentOccurrenceTracker{}

	for _, pdServiceID := range pdServiceIDs {
		for currentOffset = 0; true; currentOffset += limit {
			liResponse, err := c.pdclient.ListIncidentsWithContext(
				ctx,
				pd.ListIncidentsOptions{
					ServiceIDs: []string{pdServiceID},
					Statuses:   []string{"resolved", "triggered", "acknowledged"},
					Offset:     currentOffset,
					Limit:      limit,
					SortBy:     "created_at:desc",
				},
			)

			if err != nil {
				return nil, err
			}

			if len(liResponse.Incidents) == 0 {
				break
			}

			incidents = append(incidents, liResponse.Incidents...)
		}

		incidentCounter := make(map[string]*IncidentOccurrenceTracker)

		for _, incident := range incidents {
			title := strings.Split(incident.Title, " ")[0]
			if _, found := incidentCounter[title]; found {
				incidentCounter[title].Count++

				// Compare current incident timestamp vs our previous 'latest occurrence', and save the most recent.
				currentLastOccurrence, err := time.Parse(time.RFC3339, incidentCounter[title].LastOccurrence)
				if err != nil {
					fmt.Printf("Failed to parse time %q\n", err)
					return nil, err
				}

				incidentCreatedAt, err := time.Parse(time.RFC3339, incident.CreatedAt)
				if err != nil {
					fmt.Printf("Failed to parse time %q\n", err)
					return nil, err
				}

				// We want to see when the latest occurrence was
				if incidentCreatedAt.After(currentLastOccurrence) {
					incidentCounter[title].LastOccurrence = incident.CreatedAt
				}

			} else {
				// First time encountering this incident type
				incidentCounter[title] = &IncidentOccurrenceTracker{
					IncidentName:   title,
					Count:          1,
					LastOccurrence: incident.CreatedAt,
				}
			}
		}

		var incidentSlice []*IncidentOccurrenceTracker = make([]*IncidentOccurrenceTracker, 0, len(incidentCounter))
		for _, val := range incidentCounter {
			incidentSlice = append(incidentSlice, val)
		}

		sort.Slice(incidentSlice, func(i int, j int) bool {
			return incidentSlice[i].Count < incidentSlice[j].Count
		})
		incidentMap[pdServiceID] = append(incidentMap[pdServiceID], incidentSlice...)

	}

	return incidentMap, nil

}
