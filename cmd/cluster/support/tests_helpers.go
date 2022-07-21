package support

import sdk "github.com/openshift-online/ocm-sdk-go"

//interface that must be met to be a SDK connection
type SDKConnectionClient interface {
	Post() *sdk.Request
	Delete() *sdk.Request
}

//sdk client structure
type Client struct {
	name string
}

func (c *Client) Post() *sdk.Request {
	return &sdk.Request{}
}
func (c *Client) Delete() *sdk.Request {
	return &sdk.Request{}
}
