package policies

import (
	"strings"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
)

const GCPRoleIDPrefix = "roles/"

func GetGcpProviderSpec(credReq *cco.CredentialsRequest) (*cco.GCPProviderSpec, error) {
    provSpecObject := cco.GCPProviderSpec{}
    err := cco.Codec.DecodeProviderSpec(credReq.Spec.ProviderSpec, &provSpecObject)
    if err != nil {
      return nil, err
    }

  return &provSpecObject, nil
}

func CredentialsRequestToWifServiceAccount(credReq *cco.CredentialsRequest) (*ServiceAccount, error) {

  gcpSpec, err := GetGcpProviderSpec(credReq)

  if err != nil {
    return nil, err
  }

  sa := &ServiceAccount{} 
  sa.AccessMethod = "wif"
  sa.CredentialRequest = CredentialRequest{
  	SecretRef:           SecretRef{
  		Name:      credReq.Spec.SecretRef.Name,
  		Namespace: credReq.Spec.SecretRef.Namespace,
  	},
  	ServiceAccountNames: credReq.Spec.ServiceAccountNames,
  }
  
  sa.Id = credReq.Name
  sa.Kind = "ServiceAccount"
  sa.OsdRole = strings.Replace(credReq.Name, "openshift", "operator", 1)
  
  sa.Roles = []Role{}

  for _ , predefinedRole  := range gcpSpec.PredefinedRoles {
    sa.Roles = append(sa.Roles,Role{
    	Id:          strings.TrimPrefix(predefinedRole, GCPRoleIDPrefix),
    	Kind:        "Role",
    	Predefined:  true,
    })
  }

  if len(gcpSpec.Permissions) > 0 {
     sa.Roles = append(sa.Roles,Role{
    	Id:          credReq.Name,
    	Kind:        "Role",
    	Permissions: gcpSpec.Permissions,
    	Predefined:  false,
    })
  }
  return sa, nil
}
