package support

import sdk "github.com/openshift-online/ocm-sdk-go"

type SDKConnection interface {
	Post() *sdk.Request
	Delete() *sdk.Request
}

var (
	Client SDKConnection
)

// sdk client structure
type MockClient struct {
	//empty structure to satisfy interface
}

// Mock POST request to the API for unit tests
func (m *MockClient) Post() *sdk.Request {
	return &sdk.Request{}
}

// Mock Delete request to the API for unit tests
func (m *MockClient) Delete() *sdk.Request {
	return &sdk.Request{}
}
