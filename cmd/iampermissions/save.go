package iampermissions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"github.com/openshift/osdctl/pkg/policies"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/yaml"
)

type saveOptions struct {
	OutFolder      string
	ReleaseVersion string
	Cloud          policies.CloudSpec
	Force          bool
	DownloadCRs    func(string, policies.CloudSpec) (string, error)
	ParseCRsInDir  func(string) ([]*cco.CredentialsRequest, error)
	AWSConverter   func(*cco.CredentialsRequest) (*policies.PolicyDocument, error)
	GCPConverter   func(*cco.CredentialsRequest) (*policies.ServiceAccount, error)
	MkdirAll       func(string, os.FileMode) error
	Stat           func(string) (os.FileInfo, error)
	WriteFile      func(string, []byte, os.FileMode) error
	Print          func(format string, a ...interface{}) (n int, err error)
}

type saveCmdBuilder struct{}

func newCmdSave() *cobra.Command {
	op := &saveOptions{
		DownloadCRs:   policies.DownloadCredentialRequests,
		ParseCRsInDir: policies.ParseCredentialsRequestsInDir,
		AWSConverter:  policies.AWSCredentialsRequestToPolicyDocument,
		GCPConverter:  policies.CredentialsRequestToWifServiceAccount,
		MkdirAll:      os.MkdirAll,
		Stat:          os.Stat,
		WriteFile:     os.WriteFile,
		Print:         fmt.Printf,
	}

	saveCmd := &cobra.Command{
		Use:               "save",
		Short:             "Save iam permissions for use in mcc",
		Args:              cobra.ExactArgs(0),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, _ []string) {
			op.Cloud = *cmd.Flag(cloudFlagName).Value.(*policies.CloudSpec)
			cmdutil.CheckErr(op.run())
		},
	}

	saveCmd.Flags().StringVarP(&op.OutFolder, "dir", "d", "", "Folder where the policy files should be written")
	saveCmd.Flags().StringVarP(&op.ReleaseVersion, "release-version", "r", "", "ocp version for which the policies should be downloaded")
	saveCmd.Flags().BoolVarP(&op.Force, "force", "f", false, "Overwrite existing files")

	saveCmd.MarkFlagRequired("dir")
	saveCmd.MarkFlagRequired("release-version")

	return saveCmd
}

func (o *saveOptions) run() error {
	if err := o.MkdirAll(o.OutFolder, 0755); err != nil {
		return err
	}

	dir, err := o.DownloadCRs(o.ReleaseVersion, o.Cloud)
	if err != nil {
		return err
	}

	crs, err := o.ParseCRsInDir(dir)
	if err != nil {
		return err
	}

	files := make(map[string][]byte)
	switch o.Cloud {
	case policies.AWS:
		for _, cr := range crs {
			doc, err := o.AWSConverter(cr)
			if err != nil {
				return fmt.Errorf("error parsing CredentialsRequest '%s': %w", cr.Name, err)
			}

			path := filepath.Join(o.OutFolder, fmt.Sprintf("%s.json", cr.Name))
			out, err := json.MarshalIndent(doc, "", "    ")
			if err != nil {
				return fmt.Errorf("couldn't marshal sts policy '%s': %w", cr.Name, err)
			}
			files[path] = out
		}

	case policies.GCP:
		for _, cr := range crs {
			sa, err := o.GCPConverter(cr)
			if err != nil {
				return fmt.Errorf("error parsing CredentialsRequest '%s': %w", cr.Name, err)
			}

			path := filepath.Join(o.OutFolder, fmt.Sprintf("%s.yaml", sa.Id))
			jsonData, err := json.Marshal(sa)
			if err != nil {
				return fmt.Errorf("couldn't marshal wif ServiceAccount '%s': %w", sa.Id, err)
			}
			out, err := yaml.JSONToYAML(jsonData)
			if err != nil {
				return fmt.Errorf("error converting json to yaml: %w", err)
			}
			files[path] = out
		}
	}

	for path, content := range files {
		_, err := o.Stat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && !o.Force {
			o.Print("Cowardly refusing to overwrite: '%s'. Append '--force' to overwrite existing files.\n", path)
			continue
		}

		o.Print("Writing %s\n", path)
		if err := o.WriteFile(path, content, 0600); err != nil {
			return err
		}
	}

	return nil
}
