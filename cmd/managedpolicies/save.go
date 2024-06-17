package managedpolicies

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/osdctl/pkg/policies"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/yaml"
)

type saveOptions struct {
  OutFolder string
  ReleaseVersion string
  Cloud policies.CloudSpec
  Force bool
}

func newCmdSave() *cobra.Command{
  ops := &saveOptions{}

  saveCmd := &cobra.Command{
    Use: "save",
    Short: "Save managed policies for use in mcc",
    Args: cobra.ExactArgs(0),
    DisableAutoGenTag: true,
    Run: func(cmd *cobra.Command, _ []string) {
      ops.Cloud = *cmd.Flag(cloudFlagName).Value.(*policies.CloudSpec)
      cmdutil.CheckErr(ops.run())
    },
  }

  saveCmd.Flags().StringVarP(&ops.OutFolder, "dir", "d", "", "Folder where the policy files should be written")
  saveCmd.Flags().StringVarP(&ops.ReleaseVersion, "release-version", "r", "", "ocp version for which the policies should be downloaded")
  saveCmd.Flags().BoolVarP(&ops.Force, "force", "f", false, "Overwrite existing files")

  saveCmd.MarkFlagRequired("out")
  saveCmd.MarkFlagRequired("release-version")

  return saveCmd
}


func (o *saveOptions) run() error {
  err := os.MkdirAll(o.OutFolder, 0755)
  if err != nil {
    return err
  }

	directory, err := policies.DownloadCredentialRequests(o.ReleaseVersion, o.Cloud)
	if err != nil {
		return err
	}
  
  allCredentialsRequests, err := policies.ParseCredentialsRequestsInDir(directory)
  if err != nil {
    return err
  }

  filesToCreate := map[string][]byte{} 
  
  if o.Cloud == policies.AWS {
    for _, credReq := range(allCredentialsRequests) {
      polDoc, err := policies.AWSCredentialsRequestToPolicyDocument(credReq)
      if err != nil {
        return fmt.Errorf("Error parsing CredentialsRequest '%s': %w", credReq.Name, err)
      }

      filename := filepath.Join(o.OutFolder, fmt.Sprintf("%s.json", credReq.Name))
      out, err := json.MarshalIndent(polDoc, "", "    ")
      if err != nil {
        return fmt.Errorf("Coulnd't Marshal sts policy '%s': %w", credReq.Name , err)
      }

      filesToCreate[filename] = out
    }
  } else if o.Cloud == policies.GCP {
    for _, credReq := range(allCredentialsRequests) {
      sa, err := policies.CredentialsRequestToWifServiceAccount(credReq)
      if err != nil {
        return fmt.Errorf("Error parsing CredentialsRequest '%s': %w", credReq.Name, err)
      }
      
      filename := filepath.Join(o.OutFolder, fmt.Sprintf("%s.yaml", sa.Id))
      outJSON, err := json.Marshal(sa)
      if err != nil {
        return fmt.Errorf("Coulnd't Marshal wif ServiceAccount '%s': %w", sa.Id, err)
      }
      out, err := yaml.JSONToYAML(outJSON)
      if err != nil {
        return  fmt.Errorf("Error Converting json to yaml: %w", err)
      }
      filesToCreate[filename] = out
    }
  }

  for path, content := range(filesToCreate) {
    _, err := os.Stat(path)

    if err != nil && !errors.Is(err, os.ErrNotExist) {
      return err
    } 

    if err == nil && !o.Force {
      fmt.Printf("Cowardly refusing to overwrite: '%s'. Append '--force' to overwrite existing files.\n", path)
      continue
    }

    fmt.Printf("Writing %s\n", path)
    if err = os.WriteFile(path, content, 0600); err != nil {
      return err
    }
    
  }

  return nil
}

