package utils

import (
	"os"
	"strings"
	"testing"
)

func resetEnvVars(t *testing.T) {
	errToken := os.Unsetenv("OCM_TOKEN")
	errUrl := os.Unsetenv("OCM_URL")
	errRefreshToken := os.Unsetenv("OCM_REFRESH_TOKEN")
	if errToken != nil || errUrl != nil || errRefreshToken != nil {
		t.Fatal("Error setting environment variables")
	}
}

func assertConfigValues(t *testing.T, config *Config, err error, expectedUrl string, expectedToken string, expectedRefreshToken string) {
	if err != nil {
		t.Errorf("Count not read configuration %q", err)
	}
	if config.URL != expectedUrl {
		t.Fatalf(
			"Expected config URL, %s, does not match the actual, %s.",
			expectedUrl,
			config.URL,
		)
	}
	if config.AccessToken != expectedToken {
		t.Errorf(
			"Expected config access token, %s, does not match the actual, %s.",
			expectedToken,
			config.AccessToken,
		)
	}
	if config.RefreshToken != expectedRefreshToken {
		t.Errorf(
			"Expected config refresh token, %s, does not match the actual, %s.",
			expectedRefreshToken,
			config.RefreshToken,
		)
	}
}

func TestGetOCMConfigLocationWithEnvSet(t *testing.T) {
	resetEnvVars(t)
	defer resetEnvVars(t)

	expectedConfigLocation := "~/.config/ocm/ocm.test.json"
	envConfigErr := os.Setenv("OCM_CONFIG", expectedConfigLocation)
	if envConfigErr != nil {
		t.Fatal("Error setting OCM_CONFIG")
	}
	actualConfigLocation, err := getOCMConfigLocation()
	if err != nil {
		t.Errorf("Error getting OCM config location %q", err)
	}
	if actualConfigLocation != expectedConfigLocation {
		t.Errorf(
			"Expected location, %s, did not match actual location, %s.",
			expectedConfigLocation,
			actualConfigLocation,
		)
	}
}

func TestGetOCMConfigurationWithNoEnvVarsSet(t *testing.T) {
	resetEnvVars(t)
	defer resetEnvVars(t)

	expectedToken := "asdf"
	expectedUrl := "https://example.com"
	expectedRefreshToken := "fdsa"
	config, err := getOcmConfiguration(func() (*Config, error) {
		return &Config{
			URL:          expectedUrl,
			AccessToken:  expectedToken,
			RefreshToken: expectedRefreshToken,
		}, nil
	})
	if err != nil {
		t.Errorf("Count not read configuration %q", err)
	}

	assertConfigValues(t, config, err, expectedUrl, expectedToken, expectedRefreshToken)
}

func TestGetOCMConfigurationTokenAndUrlEnvVarsSet(t *testing.T) {
	resetEnvVars(t)
	defer resetEnvVars(t)

	expectedToken := "asdf"
	expectedUrl := "https://example.com"
	expectedRefreshToken := "fdsa"
	errToken := os.Setenv("OCM_TOKEN", expectedToken)
	errUrl := os.Setenv("OCM_URL", expectedUrl)
	if errToken != nil || errUrl != nil {
		t.Error("Error setting environment variables")
	}
	config, err := getOcmConfiguration(func() (*Config, error) {
		return &Config{
			URL:          "https://fail.example.com",
			AccessToken:  "fail",
			RefreshToken: expectedRefreshToken,
		}, nil
	})
	if err != nil {
		t.Errorf("Count not read configuration %q", err)
	}

	assertConfigValues(t, config, err, expectedUrl, expectedToken, expectedRefreshToken)
}

func TestGetOCMConfigurationTokenAndUrlAndRefreshTokenEnvVarsSet(t *testing.T) {
	resetEnvVars(t)
	defer resetEnvVars(t)

	expectedToken := "asdf"
	expectedUrl := "https://example.com"
	expectedRefreshToken := "fdsa"
	errToken := os.Setenv("OCM_TOKEN", expectedToken)
	errUrl := os.Setenv("OCM_URL", expectedUrl)
	errRefreshToken := os.Setenv("OCM_REFRESH_TOKEN", expectedRefreshToken)
	if errToken != nil || errUrl != nil || errRefreshToken != nil {
		t.Error("Error setting environment variables")
	}
	config, err := getOcmConfiguration(func() (*Config, error) {
		return &Config{
			URL:          "https://fail.example.com",
			AccessToken:  "fail",
			RefreshToken: "fail",
		}, nil
	})

	assertConfigValues(t, config, err, expectedUrl, expectedToken, expectedRefreshToken)
}

func testGenerateQuery(t *testing.T, clusterIdentifier string, expectedType string) {
	detectedType := strings.Fields(GenerateQuery(clusterIdentifier))[0][1:]
	if expectedType != detectedType {
		t.Errorf("identifier %s of type %s is detected as %s", clusterIdentifier, expectedType, detectedType)
	}
}

func TestGenerateQueryInternalId(t *testing.T) {
	testGenerateQuery(t, "261kalm3uob0vegg1c7h9o7r5k9t64ji", "id")
	testGenerateQuery(t, "261kalm3uob0vegg1c7h9o7r5k9t64j", "display_name")
	testGenerateQuery(t, "261kalm3uob0vegg1c7h9o7r5k9t64jix", "display_name")
	testGenerateQuery(t, "261kalm3uob0vegg1c7h9o7r5k9t64jI", "display_name")
}

func TestGenerateQueryExternalId(t *testing.T) {
	testGenerateQuery(t, "c1f562af-fb22-42c5-aa07-6848e1eeee9c", "external_id")
	testGenerateQuery(t, "c1f562af-fb22-42c5-aa07-6848e1eeee9cc", "display_name")
	testGenerateQuery(t, "c1f562af-fb22-42c5-aa07-6848e1eeee9", "display_name")
	testGenerateQuery(t, "C1f562af-fb22-42c5-aa07-6848e1eeee9c", "display_name")
}

func TestGenerateQueryDisplayName(t *testing.T) {
	testGenerateQuery(t, "hs-mc-773jpgko0", "display_name")
}
