package policies

import (
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
)

type PolicyDocument struct {
	Version   string
	Statement []cco.StatementEntry
}

func GetAWSProviderSpec(credReq *cco.CredentialsRequest) (*cco.AWSProviderSpec, error) {
	provSpecObject := cco.AWSProviderSpec{}
	err := cco.Codec.DecodeProviderSpec(credReq.Spec.ProviderSpec, &provSpecObject)
	if err != nil {
		return nil, err
	}

	return &provSpecObject, nil
}

func AWSCredentialsRequestToPolicyDocument(credReq *cco.CredentialsRequest) (*PolicyDocument, error) {
	awsSpec, err := GetAWSProviderSpec(credReq)
	if err != nil {
		return nil, err
	}

	out := &PolicyDocument{
		Version:   "2012-10-17",
		Statement: awsSpec.StatementEntries,
	}

	return out, nil

}
