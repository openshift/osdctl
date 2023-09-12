package support

// BadReply is the template for bad reply
type BadReply struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Href    string `json:"href"`
	Code    string `json:"code"`
	Reason  string `json:"reason"`
	Details []struct {
		Description string `json:"description"`
	} `json:"details"`
}
