package servicequotas

import (
	"errors"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
)

/*GetSupportedRegions returns a []string of regions supported
return []string of any regions found
	if allRegions: return all found regions, ignore filter
	if !allRegions:
		filter set: return 1 element if region matches
		filter == "": return all found regions
return error if no regions are returned
*/
func GetSupportedRegions(filter string, allRegions bool) ([]string, error) {
	var results []string
	for i := range awsv1alpha1.CoveredRegions {
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
