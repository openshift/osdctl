package access

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	fpath "path/filepath"
	"strings"
	"time"

	clustersmgmtv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	osdctlutil "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	clientcmdapiv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// impersonateUser represents the user SREs are allowed to impersonate in order to retrieve a cluster's kubeconfig.
	impersonateUser     = "backplane-cluster-admin"
	kubeconfigSecretKey = "kubeconfig"

	// PrivateLink "jump pod" configuration
	jumpImage         = "image-registry.openshift-image-registry.svc:5000/openshift/cli:latest"
	jumpContainerName = "jump"
	jumpPodLabelKey   = "automated-break-glass-access/cluster"

	// Lifespan for jump pods in seconds. Currently, PrivateLink jump pods will expire after 8 hours
	jumpPodLifespan = 28800
)

var (
	jumpPodPollInterval = 5 * time.Second
	jumpPodPollTimeout  = 5 * time.Minute
)

// NewCmdCluster implements the 'cluster access' subcommand
func NewCmdAccess(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	accessCmd := &cobra.Command{
		Use:               "break-glass <cluster identifier>",
		Short:             "Emergency access to a cluster",
		Long:              "Obtain emergency credentials to access the given cluster. You must be logged into the cluster's hive shard",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(accessCmdComplete(cmd, args))
			// Prior to creating k8s client, verify the user has elevated permissions
			cmdutil.CheckErr(verifyPermissions(streams, flags))
			client := k8s.NewClient(flags)
			clusterAccess := newClusterAccessOptions(client, streams, flags)
			cmdutil.CheckErr(clusterAccess.Run(cmd, args))
		},
	}
	accessCmd.AddCommand(newCmdCleanup(streams, flags))

	return accessCmd
}

// clusterCmdComplete verifies the command's invocation, returning an error if the usage is invalid
func accessCmdComplete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Exactly one cluster identifier was expected")
	}
	return osdctlutil.IsValidClusterKey(args[0])
}

// verifyPermissions determines if the user has supplied the correct permissions in order to retrieve a KubeConfig secret from hive.
// If the user attempts to impersonate an unexpected account, an error is returned.
// If the user hasn't attempted to impersonate anyone, it prompts whether they would like to do so automatically.
func verifyPermissions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) error {
	if flags.Impersonate != nil && *flags.Impersonate != "" {
		if *flags.Impersonate != impersonateUser {
			osdctlutil.StreamPrintln(streams, "")
			return fmt.Errorf("Unauthorized impersonation as user '%s'. Only requests to impersonate '%s' are allowed.", *flags.Impersonate, impersonateUser)
		}
	} else {
		osdctlutil.StreamPrintln(streams, "")
		osdctlutil.StreamPrintln(streams, fmt.Sprintf("No impersonation request detected. By design, SREs do not have sufficient permission to retrieve a cluster Kubeconfig from hive, and should impersonate '%s' to do so.", impersonateUser))
		osdctlutil.StreamPrint(streams, fmt.Sprintf("Would you like to continue as '%s'? (You can disable this prompt in the future by rerunning this command with '--as %s') [y/N] ", impersonateUser, impersonateUser))

		input, err := osdctlutil.StreamRead(streams, '\n')
		if err != nil {
			return err
		}
		if !isAffirmative(strings.TrimSpace(input)) {
			return fmt.Errorf("Did not impersonate '%s'", impersonateUser)
		}
		*flags.Impersonate = impersonateUser
		osdctlutil.StreamPrintln(streams, fmt.Sprintf("Continuing as '%s'", impersonateUser))
		osdctlutil.StreamPrintln(streams, "")
	}
	return nil
}

// clusterAccessOptions contains the objects and information required to access a cluster
type clusterAccessOptions struct {
	*genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kclient.Client
}

// newAccessOptions creates a clusterAccessOptions object
func newClusterAccessOptions(client kclient.Client, streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) clusterAccessOptions {
	a := clusterAccessOptions{
		IOStreams:   streams,
		ConfigFlags: flags,
		Client:      client,
	}
	return a
}

// Println appends a newline then prints the given msg using the clusterAccessOptions' IOStreams
func (c *clusterAccessOptions) Println(msg string) {
	osdctlutil.StreamPrintln(c.IOStreams, msg)
}

// Print prints the given msg using the clusterAccessOptions' IOStreams
func (c *clusterAccessOptions) Print(msg string) {
	osdctlutil.StreamPrint(c.IOStreams, msg)
}

// Errorln appends a newline then prints the given error msg using the clusterAccessOptions' IOStreams
func (c *clusterAccessOptions) Errorln(msg string) {
	osdctlutil.StreamErrorln(c.IOStreams, msg)
}

// Readln reads a single line of user input using the clusterAccessOptions' IOStreams. User input is returned with all
// procceeding and following whitespace trimmed
func (c *clusterAccessOptions) Readln() (string, error) {
	in, err := osdctlutil.StreamRead(c.IOStreams, '\n')
	return strings.TrimSpace(in), err
}

// Run executes the 'cluster' access subcommand
func (c *clusterAccessOptions) Run(cmd *cobra.Command, args []string) error {
	clusterIdentifier := args[0]
	c.Println(fmt.Sprintf("Retrieving Kubeconfig for cluster '%s'", clusterIdentifier))

	// Connect to ocm
	conn, err := osdctlutil.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		cmdutil.CheckErr(conn.Close())
	}()

	cluster, err := osdctlutil.GetCluster(conn, clusterIdentifier)
	if err != nil {
		return err
	}
	c.Println(fmt.Sprintf("Internal Cluster ID: %s", cluster.ID()))

	// Retrieve the kubeconfig secret from the cluster's namespace on hive
	ns, err := getClusterNamespace(c.Client, cluster.ID())
	if err != nil {
		return err
	}
	c.Println(fmt.Sprintf("Cluster namespace: %s", ns.Name))

	kubeconfigSecret, err := c.getKubeConfigSecret(ns)
	if err != nil {
		return err
	}
	c.Println(fmt.Sprintf("Kubeconfig Secret: %s", kubeconfigSecret.Name))

	// If Cluster is PrivateLink - access via jump pod on hive
	if cluster.AWS().PrivateLink() {
		c.Println("")
		c.Println("Cluster is PrivateLink, and is only accessible via a jump pod on Hive")
		return c.createJumpPodAccess(cluster, kubeconfigSecret)
	}

	// Otherwise, Cluster is not PrivateLink - save kubeconfig locally
	c.Println("")
	c.Println("Cluster is accessible via a local Kubeconfig file")
	return c.createLocalKubeconfigAccess(cluster, kubeconfigSecret)
}

// createJumpPodAccess grants access to a cluster by creating a pod for users to exec into
func (c *clusterAccessOptions) createJumpPodAccess(cluster *clustersmgmtv1.Cluster, kubeconfigSecret corev1.Secret) error {
	c.Println("Attempting to spin up a pod to use for access")

	pod, err := c.createJumpPod(kubeconfigSecret, cluster.ID())
	if err != nil {
		c.Errorln("Failed to create pod")
		return err
	}

	c.Println(fmt.Sprintf("Jump pod created. Waiting for it to start"))
	c.Println("")

	err = c.waitForJumpPod(pod, jumpPodPollInterval, jumpPodPollTimeout)
	if err != nil {
		c.Errorln("Timed out waiting for pod to start.")
		c.Println(fmt.Sprintf("You can check the status of the pod using\n\n    oc describe pods %s -n %s\n", pod.Name, pod.Namespace))
		c.Println("Once the pod is running:")
	} else {
		c.Println("Pod detected as running")
	}
	c.Println(fmt.Sprintf("Use \n\n    oc exec -it --as %s -n %s %s -- /bin/bash\n\nto run commands in the pod. All 'oc' commands run within the pod will be executed against the cluster '%s' (this can be verified by running `oc cluster-info` in the pod)", impersonateUser, pod.Namespace, pod.Name, cluster.Name()))
	return err
}

// createLocalKubeconfigAccess grants access to a cluster by writing the cluster's kubeconfig file to the local filesystem and (optionally) updating the user's cli environment
func (c *clusterAccessOptions) createLocalKubeconfigAccess(cluster *clustersmgmtv1.Cluster, kubeconfigSecret corev1.Secret) error {
	c.Println("Retrieving kubeconfig secret from Hive")

	kubeconfigFilePath := fpath.Join(os.TempDir(), kubeconfigSecret.Name)
	rawKubeconfig, found := kubeconfigSecret.Data[kubeconfigSecretKey]
	if !found {
		// Kubeconfig secret doesn't contain the expected key - write the obtained secret to a temp location so the user can troubleshoot or manually parse
		c.Errorln(fmt.Sprintf("\nExpected key '%s' not found in Secret", kubeconfigSecretKey))
		c.Println("Attempting to save Secret locally")

		rawData, err := json.Marshal(kubeconfigSecret)
		if err != nil {
			c.Errorln("Failed to marshal secret to raw data")
			return err
		}

		err = saveAsLocalFile(rawData, kubeconfigFilePath)
		if err != nil {
			c.Errorln("Failed to write Secret to file")
			return err
		}

		c.Println(fmt.Sprintf("File has been written to '%s' for manual use", kubeconfigFilePath))
		return fmt.Errorf("Could not parse cluster's kubeconfig Secret")
	}

	// Determine if cluster utilizes a Private API
	listening, listeningOK := cluster.API().GetListening()
	if !listeningOK {
		// Do not return if we can't determine the listening status of the apiserver - in both cases (private or non-private), the kubeconfig is needed locally, so we
		// should pull it anyway, but give clear warning that additional manual action may be required if the kubeconfig fails to work.
		c.Errorln("\nFailed to determine if the cluster is private.\nIf you're not able to access the cluster, try modifying the resulting kubeconfig according to the SOP: https://github.com/openshift/ops-sop/blob/master/v4/howto/break-glass-kubeadmin.md#for-clusters-with-private-api")
	} else if listening == clustersmgmtv1.ListeningMethodInternal {
		// If the cluster has a private API, it must be accessed using a special API url from one of the bastions
		return c.createPrivateAPIAccess(rawKubeconfig, kubeconfigFilePath)
	}

	// Write the kubeconfig to the temp filesystem
	c.Println("Saving kubeconfig")
	err := saveAsLocalFile(rawKubeconfig, kubeconfigFilePath)
	if err != nil {
		c.Errorln("Failed to save kubeconfig")
		return err
	}

	c.Println("")
	c.Println(fmt.Sprintf("Kubeconfig successfully written to '%s'", kubeconfigFilePath))
	c.Println("")

	c.Print(fmt.Sprintf("Would you like to open a new shell that uses 'KUBECONFIG=%s'? [y/N] ", kubeconfigFilePath))
	input, err := c.Readln()
	if err != nil {
		c.Errorln("Failed to read user input")
		return err
	}

	if isAffirmative(input) {
		err = os.Setenv("KUBECONFIG", kubeconfigFilePath)
		if err != nil {
			c.Errorln("Failed to set $KUBECONFIG")
			return err
		}

		shell, found := os.LookupEnv("SHELL")
		if !found {
			c.Println("$SHELL appears to be unset - defaulting to '/bin/bash'")
			shell = "/bin/bash"
		}

		c.Println("")
		c.Println(fmt.Sprintf("A new shell will be spawned, with $KUBECONFIG set to '%s'.", kubeconfigFilePath))
		c.Println("")
		c.Println(fmt.Sprintf("`oc` commands should therefore execute against the cluster '%s' (you can verify this by running `oc cluster-info`)", cluster.Name()))
		c.Println(fmt.Sprintf("To add this capability to other terminals, run\n\n    export KUBECONFIG=%s\n\nwherever you'd like to execute commands against this cluster", kubeconfigFilePath))
		c.Println("When you are done, type 'exit' (or use ctl-D) to return to the original terminal")

		// Spawn a new shell
		cmd := exec.Command(shell)
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			c.Errorln(fmt.Sprintf("Error while running in shell: %v", err))
		}
		c.Println(fmt.Sprintf("Finished executing against cluster '%s'", cluster.Name()))
	} else {
		c.Println("Shell not updated")
		c.Println(fmt.Sprintf("Run\n\n    export KUBECONFIG=%s\n\nin the terminal you would like to use for executing commands against '%s'", kubeconfigFilePath, cluster.Name()))
	}

	return nil
}

// createPrivateAPIAccess provides the necessary changes to access clusters with Private APIs
func (c *clusterAccessOptions) createPrivateAPIAccess(rawKubeconfig []byte, kubeconfigFilePath string) error {
	c.Println("Cluster is private. Updating kubeconfig to execute commands against the rh-api")

	formattedKubeconfig := clientcmdapiv1.Config{}

	d := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(rawKubeconfig), len(rawKubeconfig))
	if err := d.Decode(&formattedKubeconfig); err != nil {
		c.Errorln("Failed to unmarshal kubeconfig")
		return err
	}

	// Replace the server URL w/ the URL for the RH-api
	for i := range formattedKubeconfig.Clusters {
		originalServerURL := formattedKubeconfig.Clusters[i].Cluster.Server
		formattedKubeconfig.Clusters[i].Cluster.Server = strings.Replace(originalServerURL, "api.", "rh-api.", 1)
	}

	var err error
	jsonRawKubeConfig, err1 := json.Marshal(formattedKubeconfig)
	if err1 != nil {
		c.Errorln("Failed to re-marshal json kubeconfig")
		return err
	}

	rawKubeconfig, err = yaml.JSONToYAML(jsonRawKubeConfig)
	if err != nil {
		c.Errorln("Failed to re-marshal yaml kubeconfig")
		return err
	}

	// Write the kubeconfig to the temp filesystem
	c.Println("Saving kubeconfig")
	err = saveAsLocalFile(rawKubeconfig, kubeconfigFilePath)
	if err != nil {
		c.Errorln("Failed to save kubeconfig")
		return err
	}

	c.Println("")
	c.Println(fmt.Sprintf("Kubeconfig successfully written to '%s'", kubeconfigFilePath))
	c.Println("")
	c.Println("Next steps are detailed in the Private API SOP: https://github.com/openshift/ops-sop/blob/master/v4/howto/break-glass-kubeadmin.md#for-clusters-with-private-api")
	c.Println("")
	c.Println(fmt.Sprintf("    scp %s bastion:.private/", kubeconfigFilePath))
	c.Println("")
	c.Println("    ssh bastion")
	c.Println("")
	c.Println(fmt.Sprintf("    export KUBECONFIG=$HOME/.private/%s", fpath.Base(kubeconfigFilePath)))

	return nil
}

// getKubeConfigSecret returns the first secret in the given namespace which contains the "hive.openshift.io/secret-type: kubeconfig" label
func (c *clusterAccessOptions) getKubeConfigSecret(ns corev1.Namespace) (corev1.Secret, error) {
	secretList := corev1.SecretList{}
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"hive.openshift.io/secret-type": "kubeconfig"}}
	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		return corev1.Secret{}, err
	}
	err = c.Client.List(context.TODO(), &secretList, &kclient.ListOptions{Namespace: ns.Name, LabelSelector: selector})
	if err != nil {
		return corev1.Secret{}, err
	}

	if len(secretList.Items) == 0 {
		return corev1.Secret{}, fmt.Errorf("Kubeconfig secret not found in namespace '%s'", ns.Name)
	}

	// Just return the first item in list
	// TODO: What do if we have >1 secret?
	return secretList.Items[0], nil
}

// saveAsLocalFile writes data as a file on the local filesystem with mode 0600
func saveAsLocalFile(data []byte, path string) error {
	return os.WriteFile(path, data, os.FileMode(0600))
}

// createJumpPod creates a deployment on hive to access a PrivateLink cluster from.
func (c *clusterAccessOptions) createJumpPod(kubeconfigSecret corev1.Secret, clusterid string) (corev1.Pod, error) {
	name := fmt.Sprintf("jumphost-%s-%d", time.Now().Format("20060102-150405-"), (time.Now().Nanosecond() / 1000000))
	ns := kubeconfigSecret.Namespace
	label := map[string]string{jumpPodLabelKey: clusterid}

	deploy := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    label,
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: kubeconfigSecretKey,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: kubeconfigSecret.Name,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Containers: []corev1.Container{
				{
					Name:    jumpContainerName,
					Image:   jumpImage,
					Command: []string{"/bin/sh"},
					Args:    []string{"-c", fmt.Sprintf("sleep %d", jumpPodLifespan)},
					Env: []corev1.EnvVar{
						{
							Name:  "KUBECONFIG",
							Value: fmt.Sprintf("/tmp/%s", kubeconfigSecretKey),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      kubeconfigSecretKey,
							MountPath: "/tmp",
						},
					},
				},
			},
		},
	}
	err := c.Client.Create(context.TODO(), &deploy)
	return deploy, err
}

// waitForPod polls until the given pod is ready
func (c *clusterAccessOptions) waitForJumpPod(pod corev1.Pod, interval time.Duration, timeout time.Duration) error {
	key := types.NamespacedName{
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}
	return wait.PollImmediate(interval, timeout, func() (done bool, err error) {
		err = c.Client.Get(context.TODO(), key, &pod)
		if kerr.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name == jumpContainerName && *container.Started {
				return true, nil
			}
		}
		return false, nil
	})
}
