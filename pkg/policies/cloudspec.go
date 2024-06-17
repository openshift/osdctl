package policies

import (
	"fmt"
	"strings"
)

type CloudSpec int

const (
	AWS CloudSpec = iota
	GCP CloudSpec = iota
)

// String is used both by fmt.Print and by Cobra in help text
func (e *CloudSpec) String() string {
	switch *e {
	case AWS:
		return "aws"
	case GCP:
		return "gcp"
	default:
		return "unknown"
	}
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *CloudSpec) Set(v string) error {
	switch strings.ToLower(v) {
	case "aws", "sts":
		*e = AWS
		return nil
  case "gcp", "wif":
		*e = GCP
		return nil
	default:
		return fmt.Errorf(`must be one of "aws", "sts", "gcp", or "wif"`)
	}
}

// Type is only used in help text
func (*CloudSpec) Type() string {
	return "CloudSpec"
}
