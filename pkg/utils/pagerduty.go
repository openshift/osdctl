package utils

import (
	"context"
	"fmt"
	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

const (
	PagerDutyUserTokenConfigKey  = "pd_user_token"
	PagerDutyOauthTokenConfigKey = "pd_oauth_token"
	PagerDutyTeamIDsKey          = "team_ids"
)

func GetPagerDutyClient(usertoken string, oauthtoken string) (*pd.Client, error) {
	client, err := getPDUserClient(usertoken)
	if client != nil {
		return client, err
	}

	client, err = getPDOauthClient(oauthtoken)
	if err != nil {
		return nil, errors.New("failed to create both user and oauth clients for pd, neither key pd_oauth_token or pd_user_token are set in config file")
	}
	return client, err
}

func GetPDServiceIDs(baseDomain string, usertoken string, oauthtoken string, teamIds []string) ([]string, error) {
	pdClient, err := GetPagerDutyClient(usertoken, oauthtoken)
	if err != nil {
		return nil, fmt.Errorf("failed to GetPagerDutyClient: %w", err)
	}

	// Gets the PD Team IDS
	teams := getPDTeamIDs(teamIds)

	lsResponse, err := pdClient.ListServicesWithContext(context.TODO(), pd.ListServiceOptions{Query: baseDomain, TeamIDs: teams})
	if err != nil {
		return []string{}, fmt.Errorf("failed to ListServicesWithContext: %w", err)
	}

	var serviceIDS []string
	for _, service := range lsResponse.Services {
		serviceIDS = append(serviceIDS, service.ID)
	}

	return serviceIDS, nil
}

func GetCurrentPDAlertsForCluster(pdServiceIDs []string, pdUsertoken string, pdAuthToken string) (map[string][]pd.Incident, error) {
	pdClient, err := GetPagerDutyClient(pdUsertoken, pdAuthToken)
	if err != nil {
		return nil, fmt.Errorf("error getting pd client: %w", err)
	}
	incidents := map[string][]pd.Incident{}

	var incidentLimit uint = 25
	var incidentListOffset uint = 0
	for _, pdServiceID := range pdServiceIDs {
		for {
			listIncidentsResponse, err := pdClient.ListIncidentsWithContext(
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

func getPDUserClient(usertoken string) (*pd.Client, error) {
	if usertoken == "" {
		if !viper.IsSet(PagerDutyUserTokenConfigKey) {
			return nil, fmt.Errorf("key %s is not set in config file", PagerDutyUserTokenConfigKey)
		}
		usertoken = viper.GetString(PagerDutyUserTokenConfigKey)
	}
	return pd.NewClient(usertoken), nil
}

func getPDOauthClient(oauthtoken string) (*pd.Client, error) {
	if oauthtoken == "" {
		if !viper.IsSet(PagerDutyOauthTokenConfigKey) {
			return nil, fmt.Errorf("key %s is not set in config file", PagerDutyOauthTokenConfigKey)
		}
		oauthtoken = viper.GetString(PagerDutyOauthTokenConfigKey)
	}
	return pd.NewOAuthClient(oauthtoken), nil
}

// Returns an empty array if team_ids has not been informed via CLI or config file
// that will make the query show all PD Alerts for all PD services by default
func getPDTeamIDs(teamIds []string) []string {
	if len(teamIds) == 0 {
		if !viper.IsSet(PagerDutyTeamIDsKey) {
			return []string{}
		}
		teamIds = viper.GetStringSlice(PagerDutyTeamIDsKey)
	}
	return teamIds
}
