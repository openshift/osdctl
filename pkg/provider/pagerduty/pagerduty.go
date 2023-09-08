package pagerduty

import (
	"context"
	"fmt"

	pd "github.com/PagerDuty/go-pagerduty"
)

const (
	PagerDutyUserTokenConfigKey  = "pd_user_token"
	PagerDutyOauthTokenConfigKey = "pd_oauth_token"
	PagerDutyTeamIDsKey          = "team_ids"
)

type pdClientInterface interface {
	ListIncidentsWithContext(context.Context, pd.ListIncidentsOptions) (*pd.ListIncidentsResponse, error)
	ListServicesWithContext(context.Context, pd.ListServiceOptions) (*pd.ListServiceResponse, error)
}

type client struct {
	pdclient   pdClientInterface
	baseDomain string
	teamIds    []string
	userToken  string
	oauthToken string
}

func NewClient() *client {
	return &client{}
}

func (c *client) WithBaseDomain(baseDomain string) *client {
	c.baseDomain = baseDomain
	return c
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

func (c *client) GetPDServiceIDs() ([]string, error) {
	// TODO : do we need this to be an exposed function or could we do this when we build the client?
	lsResponse, err := c.pdclient.ListServicesWithContext(context.TODO(), pd.ListServiceOptions{Query: c.baseDomain, TeamIDs: c.teamIds})
	if err != nil {
		return []string{}, fmt.Errorf("failed to ListServicesWithContext: %w", err)
	}

	var serviceIDS []string
	for _, service := range lsResponse.Services {
		serviceIDS = append(serviceIDS, service.ID)
	}

	return serviceIDS, nil
}

func (c *client) GetFiringAlertsForCluster(pdServiceIDs []string) (map[string][]pd.Incident, error) {
	incidents := map[string][]pd.Incident{}

	var incidentLimit uint = 25
	var incidentListOffset uint = 0
	for _, pdServiceID := range pdServiceIDs {
		for {
			listIncidentsResponse, err := c.pdclient.ListIncidentsWithContext(
				context.TODO(),
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
