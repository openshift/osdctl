package servicequotas

import (
	"errors"
)

var coveredRegions = map[string]map[string]string{
	"us-east-1": {
		"initializationAMI": "ami-000db10762d0c4c05",
	},
	"us-east-2": {
		"initializationAMI": "ami-094720ddca649952f",
	},
	"us-west-1": {
		"initializationAMI": "ami-04642fc8fca1e8e67",
	},
	"us-west-2": {
		"initializationAMI": "ami-0a7e1ebfee7a4570e",
	},
	"ca-central-1": {
		"initializationAMI": "ami-06ca3c0058d0275b3",
	},
	"eu-central-1": {
		"initializationAMI": "ami-09de4a4c670389e4b",
	},
	"eu-west-1": {
		"initializationAMI": "ami-0202869bdd0fc8c75",
	},
	"eu-west-2": {
		"initializationAMI": "ami-0188c0c5eddd2d032",
	},
	"eu-west-3": {
		"initializationAMI": "ami-0c4224e392ec4e440",
	},
	"ap-northeast-1": {
		"initializationAMI": "ami-00b95502a4d51a07e",
	},
	"ap-northeast-2": {
		"initializationAMI": "ami-041b16ca28f036753",
	},
	"ap-south-1": {
		"initializationAMI": "ami-0963937a03c01ecd4",
	},
	"ap-southeast-1": {
		"initializationAMI": "ami-055c55112e25b1f1f",
	},
	"ap-southeast-2": {
		"initializationAMI": "ami-036b423b657376f5b",
	},
	"sa-east-1": {
		"initializationAMI": "ami-05c1c16cac05a7c0b",
	},
}

// GetSupportedRegions returns a []string of all supported regions if found and an error. Filtering for a single region is also supported.
func GetSupportedRegions(filter string, allRegions bool) ([]string, error) {
	var results []string

	for i := range coveredRegions {
		if (filter != "") && !allRegions {
			if filter == i {
				results = append(results, i)
				return results, nil
			}
			continue
		}

		results = append(results, i)
	}

	if len(results) == 0 {
		return results, errors.New("Cannot find Supported Region")
	}

	return results, nil
}
