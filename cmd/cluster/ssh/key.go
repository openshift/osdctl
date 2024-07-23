package ssh

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// sshSecretName defines the name of the ssh secret in each hive namespace
	sshSecretName = "ssh"
	// privateKeyFilename defines the map key used to identify the private ssh key in the hive "ssh" secret's data
	privateKeyFilename = "ssh-privatekey"
)

var (
	skipConfirmation bool
)

func NewCmdKey() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key [cluster identifier]",
		Short: "Retrieve a cluster's SSH key from Hive",
		Long:  "Retrieve a cluster's SSH key from Hive. If a cluster identifier (internal ID, UUID, name, etc) is provided, then the key retrieved will be for that cluster. If no identifier is provided, then the key for the cluster backplane is currently logged into will be used instead. This command should only be used as a last resort, when all other means of accessing a node are lost.",
		Example: `$ osdctl cluster ssh key $CLUSTER_ID
INFO[0005] Backplane URL retrieved via OCM environment: https://api.backplane.openshift.com
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----

Providing a $CLUSTER_ID allows you to specify the cluster who's private ssh key you want to view, regardless if you're logged in or not.


$ osdctl cluster ssh key
INFO[0005] Backplane URL retrieved via OCM environment: https://api.backplane.openshift.com
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----

Omitting the $CLUSTER_ID will print the ssh key for the cluster you're currently logged into.


$ osdctl cluster ssh key -y > /tmp/ssh.key
INFO[0005] Backplane URL retrieved via OCM environment: https://api.backplane.openshift.com
$ cat /tmp/ssh.key
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----

Despite the logs from backplane, the ssh key is the only output channelled through stdout. This means you can safely redirect the output to a file for greater convienence.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {

			// If user provides an argument: use it to identify the cluster's hive shard,
			// otherwise use the current cluster's ID
			clusterID := ""
			var err error
			if len(args) == 0 {
				clusterID, err = k8s.GetCurrentCluster()
				if err != nil {
					return fmt.Errorf("failed to retrieve ID for current cluster")
				}
			} else {
				clusterID = args[0]
			}

			err = PrintKey(clusterID, skipConfirmation)
			if err != nil {
				return fmt.Errorf("failed to retrieve ssh key for cluster %s: %w", clusterID, err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&skipConfirmation, "yes", "y", false, "Skip any confirmation prompts and print the key automatically. Useful for redirects and scripting.")

	return cmd
}

// PrintKey retrieves the cluster's private ssh key from hive and prints it to stdout.
func PrintKey(identifier string, skipConfirmation bool) error {
	// Login to the provided cluster's hive shard
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("failed to establish connection to OCM: %w", err)
	}

	cluster, err := utils.GetCluster(ocmClient, identifier)
	if err != nil {
		return fmt.Errorf("failed to retrieve cluster from OCM: %w", err)
	}

	// Print summary and confirm this is the intended cluster before proceeding
	if !skipConfirmation {
		fmt.Println("Cluster:")
		fmt.Printf("\tName:\t%s\n", cluster.Name())
		fmt.Printf("\tID:\t%s\n", cluster.ID())
		fmt.Printf("\tUUID:\t%s\n", cluster.ExternalID())
		fmt.Println()
		if !utils.ConfirmPrompt() {
			return nil
		}
		fmt.Println()
	}

	clusterID := cluster.ID()
	hive, err := utils.GetHiveCluster(clusterID)
	if err != nil {
		return fmt.Errorf("failed to retrieve hive shard for cluster: %w", err)
	}

	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	hiveClient, err := k8s.NewAsBackplaneClusterAdmin(hive.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create privileged client: %w", err)
	}

	// Determine the cluster's hive namespace via cluster ID
	namespaces := corev1.NamespaceList{}
	err = hiveClient.List(context.TODO(), &namespaces)
	if err != nil {
		return fmt.Errorf("failed to list hive namespaces: %w", err)
	}

	namespace, err := findClusterNamespace(namespaces, clusterID)
	if err != nil {
		return fmt.Errorf("failed to locate cluster namespace in hive: %w", err)
	}

	// Grab secret from the cluster's hive NS
	secret := corev1.Secret{}
	err = hiveClient.Get(context.TODO(), types.NamespacedName{Namespace: namespace.Name, Name: sshSecretName}, &secret)
	if err != nil {
		return fmt.Errorf("failed to retrieve secret from hive: %w", err)
	}

	// Grab the correct file out of the secret & decode
	encodedPrivateKey, found := secret.Data[privateKeyFilename]
	if !found {
		return fmt.Errorf("failed to locate the private ssh key in the '%s/%s' secret from hive shard '%s'", secret.Namespace, secret.Name, hive.Name())
	}

	fmt.Println(string(encodedPrivateKey))

	return nil
}

func findClusterNamespace(namespaces corev1.NamespaceList, clusterID string) (corev1.Namespace, error) {
	for _, namespace := range namespaces.Items {
		if strings.Contains(namespace.Name, clusterID) {
			return namespace, nil
		}
	}
	return corev1.Namespace{}, fmt.Errorf("no namespace containing the identifier '%s' found", clusterID)
}
