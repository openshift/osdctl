package access

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	fpath "path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	clustersmgmtv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	clientcmdapiv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestAccessCmdComplete ensures clusterAccessOptions.Complete allows for only a single cluster to be passed to the 'cluster' subcommand
func TestAccessCmdComplete(t *testing.T) {
	tests := []struct {
		Name          string
		Args          []string
		ErrorExpected bool
	}{
		{
			Name:          "Single cluster provided",
			Args:          []string{"testCluster"},
			ErrorExpected: false,
		},
		{
			Name:          "No cluster provided",
			Args:          []string{},
			ErrorExpected: true,
		},
		{
			Name:          "Multiple clusters provided",
			Args:          []string{"testCluster", "testCluster2"},
			ErrorExpected: true,
		},
		{
			Name:          "Invalid cluster provided",
			Args:          []string{"inv@lid/cluster"},
			ErrorExpected: true,
		},
		{
			Name:          "Multiple invalid clusters provided",
			Args:          []string{"inv@lid/cluster1", "inv@lid/cluster2"},
			ErrorExpected: true,
		},
	}

	for _, test := range tests {
		err := accessCmdComplete(&cobra.Command{}, test.Args)
		if test.ErrorExpected {
			if err == nil {
				t.Fatalf("Test '%s' failed. Expected error, but got none", test.Name)
			}
		} else {
			if err != nil {
				t.Fatalf("Test '%s' failed. Expected no error, but got '%v'", test.Name, err)
			}
		}
	}
}

func TestVerifyPermissions(t *testing.T) {
	privUser := impersonateUser
	unprivUser := "not-backplane-cluster-admin"
	noUser := ""

	tests := []struct {
		Name    string
		Flags   *genericclioptions.ConfigFlags
		Streams genericclioptions.IOStreams
		Allowed bool
	}{
		{
			Name: "Impersonate Privileged User",
			Flags: &genericclioptions.ConfigFlags{
				Impersonate: &privUser,
			},
			Streams: genericclioptions.NewTestIOStreamsDiscard(),
			Allowed: true,
		},
		{
			Name: "Impersonate Unprivileged User",
			Flags: &genericclioptions.ConfigFlags{
				Impersonate: &unprivUser,
			},
			Streams: genericclioptions.NewTestIOStreamsDiscard(),
			Allowed: false,
		},
		{
			Name:  "No Impersonation, default choice",
			Flags: &genericclioptions.ConfigFlags{},
			Streams: genericclioptions.IOStreams{
				Out:    genericclioptions.NewTestIOStreamsDiscard().Out,
				ErrOut: genericclioptions.NewTestIOStreamsDiscard().ErrOut,
				In:     strings.NewReader("\n"),
			},
			Allowed: false,
		},
		{
			Name:  "No Impersonation, explicit deny",
			Flags: &genericclioptions.ConfigFlags{},
			Streams: genericclioptions.IOStreams{
				Out:    genericclioptions.NewTestIOStreamsDiscard().Out,
				ErrOut: genericclioptions.NewTestIOStreamsDiscard().ErrOut,
				In:     strings.NewReader("n\n"),
			},
			Allowed: false,
		},
		{
			Name:  "No Impersonation, nonsense input",
			Flags: &genericclioptions.ConfigFlags{},
			Streams: genericclioptions.IOStreams{
				Out:    genericclioptions.NewTestIOStreamsDiscard().Out,
				ErrOut: genericclioptions.NewTestIOStreamsDiscard().ErrOut,
				In:     strings.NewReader("%\n"),
			},
			Allowed: false,
		},
		{
			Name: "No Impersonation, explicit accept",
			Flags: &genericclioptions.ConfigFlags{
				// Must initialize Impersonate field for acceptance test - else segfault :)
				Impersonate: &noUser,
			},
			Streams: genericclioptions.IOStreams{
				Out:    genericclioptions.NewTestIOStreamsDiscard().Out,
				ErrOut: genericclioptions.NewTestIOStreamsDiscard().ErrOut,
				In:     strings.NewReader("y\n"),
			},
			Allowed: true,
		},
	}
	for _, test := range tests {
		fmt.Printf("Testing '%s'\n", test.Name)
		err := verifyPermissions(test.Streams, test.Flags)
		if test.Allowed {
			if err != nil {
				t.Errorf("Failed '%s': expected permissions to be sufficient, but found that they were not. Err: %v", test.Name, err)
			}
		} else {
			if err == nil {
				t.Errorf("Failed '%s': expected permissions to be insufficient, but found that they were not. Err: %v", test.Name, err)
			}
		}
	}
}

// TestClusterAccessOptions_createLocalKubeconfigAccess tests the clusterAccessOptions' createLocalKubeconfigAccess function
func TestClusterAccessOptions_createLocalKubeconfigAccess(t *testing.T) {
	ns := "uhc-production-testfakecluster1234"
	validKubeconfigKey := "kubeconfig"

	type secretCfg struct {
		Name string
		Key  string
	}

	tests := []struct {
		Name          string
		Secret        secretCfg
		UpdateEnvResp string
		PrivateAPI    bool
		ExpectErr     bool
	}{
		{
			Name: "Valid Secret, update env",
			Secret: secretCfg{
				Name: "valid-secret",
				Key:  validKubeconfigKey,
			},
			UpdateEnvResp: "y",
			PrivateAPI:    false,
			ExpectErr:     false,
		}, {
			Name: "Valid secret, don't update env",
			Secret: secretCfg{
				Name: "valid-secret",
				Key:  validKubeconfigKey,
			},
			UpdateEnvResp: "n",
			PrivateAPI:    false,
			ExpectErr:     false,
		},
		{
			Name: "Invalid Secret Key",
			Secret: secretCfg{
				Name: "broken-secret",
				Key:  "broken-kubeconfig",
			},
			UpdateEnvResp: "",
			PrivateAPI:    false,
			ExpectErr:     true,
		},
		{
			Name: "Private API",
			Secret: secretCfg{
				Name: "valid-secret",
				Key:  validKubeconfigKey,
			},
			UpdateEnvResp: "y",
			PrivateAPI:    true,
			ExpectErr:     false,
		},
	}

	for _, test := range tests {
		// Run test as anonymous function in order to cleanup saved files between runs
		func() {
			fmt.Printf("Testing '%s'\n", test.Name)

			// Setup Environment
			updateEnvResponse := fmt.Sprintf("%s\n", test.UpdateEnvResp)
			streams := genericclioptions.IOStreams{In: strings.NewReader(updateEnvResponse), Out: os.Stdout, ErrOut: os.Stderr}
			flags := genericclioptions.ConfigFlags{}
			client := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
			access := newClusterAccessOptions(client, streams, &flags)

			// Generate test objects
			cluster := generateClusterObjectForTesting("test-cluster", "test-cluster-id", false, test.PrivateAPI)
			serverURL := "https://api.test-cluster.fakedomain.devshift.org:6443"
			secretName := fmt.Sprintf("test-osdctl-access-cluster-%s-%d-secret-%s", time.Now().Format("20060102-150405-"), (time.Now().Nanosecond() / 1000000), test.Secret.Name)
			secret, expectedKubeconfig := generateKubeconfigSecretObjectForTesting(secretName, ns, test.Secret.Key, serverURL)
			defer func(secretName string) {
				kubeconfigFilePath := fpath.Join(os.TempDir(), secretName)
				err := os.Remove(kubeconfigFilePath)
				if err != nil {
					t.Fatalf("Failed to cleanup file '%s': %v", kubeconfigFilePath, err)
				}
			}(secretName)

			// Run test
			_, found := os.LookupEnv("KUBECONFIG")
			if found && isAffirmative(test.UpdateEnvResp) {
				t.Skipf("Skipping '%s': test would overwrite currently set environment variable '$KUBECONFIG'. Unset $KUBECONFIG to run this test in the future.", test.Name)
			}
			err := access.createLocalKubeconfigAccess(&cluster, secret)
			defer func() {
				if isAffirmative(test.UpdateEnvResp) {
					err = os.Unsetenv("KUBECONFIG")
					if err != nil {
						t.Errorf("Failed '%s': could not unset environment variable $KUBECONFIG: %v", test.Name, err)
					}
				}
			}()

			// Validate results
			// Verify err
			if test.ExpectErr {
				if err == nil {
					t.Errorf("Failed '%s': expected to receive err, but got %v. ", test.Name, err)
				}
			} else {
				if err != nil {
					t.Errorf("Failed '%s': did not expect to recieve err, but got %v. ", test.Name, err)
				}
			}

			// Verify local file
			expectedFilePath := fpath.Join(os.TempDir(), secret.Name)
			metadata, err := os.Stat(expectedFilePath)
			if os.IsNotExist(err) {
				t.Errorf("Failed '%s': local file not written to expected location: %s", test.Name, expectedFilePath)
			} else if err != nil {
				t.Errorf("Failed '%s': could not retrieve local file: %v", test.Name, err)
			}

			if metadata.Mode() != os.FileMode(0600) {
				t.Errorf("Failed '%s': local file not written with expected mode 0600", test.Name)
			}

			file, err := os.ReadFile(expectedFilePath)
			if err != nil {
				t.Errorf("Failed '%s': unable to open local file: %v", test.Name, err)
			}
			if test.ExpectErr {
				// Since we expect the createLocalKubeconfigAccess func to throw an error in this test, we also expect the local file to be the raw secret
				localSecret := corev1.Secret{}
				err = json.Unmarshal(file, &localSecret)
				if err != nil {
					t.Errorf("Failed '%s': could not unmarshal local file to Secret: %v", test.Name, err)
				}
				if !reflect.DeepEqual(secret, localSecret) {
					t.Errorf("Failed '%s': Secrets are not equivalent. Original Secret: %v\n\nWritten Secret: %v", test.Name, secret, localSecret)
				}
			} else {
				// Since we don't expect the createLocalKubeconfigAccess func to throw an error in this test, we also expect the local file to be a valid kubeconfig
				localKubeconfig := clientcmdapiv1.Config{}
				d := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(file), len(file))
				if err := d.Decode(&localKubeconfig); err != nil {
					t.Errorf("Failed '%s': could not unmarshal local file to Config: %v", test.Name, err)
				}
				if test.PrivateAPI {
					// Update expected kubeconfig with the private API URL
					expectedServerURL := "https://rh-api.test-cluster.fakedomain.devshift.org:6443"
					expectedKubeconfig.Clusters[0].Cluster.Server = expectedServerURL
					// After updating the kubeconfig, we have to re- Marshal & Unmarshal the yaml for the following DeepEqual call to succeed
					rawNestedKubeconfig, err := json.Marshal(expectedKubeconfig)
					if err != nil {
						t.Errorf("Failed '%s': could not marshal kubeconfig after updating for private API: %v", test.Name, err)
					}
					rawKubeconfig, err1 := yaml.JSONToYAML(rawNestedKubeconfig)
					if err1 != nil {
						t.Errorf("Failed '%s': could not marshal kubeconfig after updating for private API: %v", test.Name, err)
					}
					err = yaml.Unmarshal(rawKubeconfig, &expectedKubeconfig)
					if err != nil {
						t.Errorf("Failed '%s': could not unmarshal kubeconfig after updating for private API: %v", test.Name, err)
					}
				}

				if !reflect.DeepEqual(expectedKubeconfig, localKubeconfig) {
					t.Errorf("Failed '%s': Kubeconfigs are not equivalent.\nOriginal kubeconfig:%v\n\nWritten kubeconfig:%v", test.Name, expectedKubeconfig, localKubeconfig)
				}
			}

			// Verify environment
			// Environment should never be updated for clusters with a PrivateAPI, since they must be accessed via bastion
			kubeconfigEnvVar, found := os.LookupEnv("KUBECONFIG")
			if isAffirmative(test.UpdateEnvResp) && !test.PrivateAPI {
				if !found {
					t.Errorf("Failed '%s': KUBECONFIG environment variable not set, but expected to be", test.Name)
				}
				if kubeconfigEnvVar != expectedFilePath {
					t.Errorf("Failed '%s': KUBECONFIG environment variable not set to expected value. Found: %s, expected: %s", test.Name, kubeconfigEnvVar, expectedFilePath)
				}
			} else {
				if found {
					t.Errorf("Failed '%s': KUBECONFIG environment variable set, expected it to be unset", test.Name)
				}
			}
		}()
	}
}

func TestClusterAccessOptions_createJumpPod(t *testing.T) {
	tests := []struct {
		Name string
	}{
		{
			Name: "createJumpPod",
		},
	}

	for _, test := range tests {
		fmt.Printf("Testing '%s'\n", test.Name)

		// Setup Environment
		scheme := runtime.NewScheme()
		err := corev1.AddToScheme(scheme)
		if err != nil {
			t.Fatalf("Failed to add corev1 to scheme: %v", err)
		}
		client := fake.NewClientBuilder().WithScheme(scheme).Build()

		flags := genericclioptions.ConfigFlags{}
		streams := genericclioptions.IOStreams{In: genericclioptions.NewTestIOStreamsDiscard().In, Out: os.Stdout, ErrOut: os.Stderr}
		access := newClusterAccessOptions(client, streams, &flags)

		// Generate test objects
		serverURL := "https://api.test-cluster.fakedomain.devshift.org:6443"
		secretName := fmt.Sprintf("test-osdctl-access-cluster-%s-%d-secret-%s", time.Now().Format("20060102-150405-"), (time.Now().Nanosecond() / 1000000), "test-createJumpPod")
		secretNS := "uhc-production-testclusterns"
		secret, _ := generateKubeconfigSecretObjectForTesting(secretName, secretNS, "kubeconfig", serverURL)

		// Run test
		pod, err := access.createJumpPod(secret, "fake-cluster-uuid-123456")
		if err != nil {
			t.Errorf("Failed %s: error while creating pod: %v", test.Name, err)
		}

		// Verify pod was built correctly
		// Verify volume
		if len(pod.Spec.Volumes) != 1 {
			t.Errorf("Unexpected number of volumes: expected 1, got %d", len(pod.Spec.Volumes))
		} else if pod.Spec.Volumes[0].VolumeSource.Secret.SecretName != secretName {
			t.Errorf("Pod's volume does not reference the kubeconfig secret")
		}

		// Verify container - confirm that the mount path and the environment variables align so users needn't set anything manually for the pod to function
		if len(pod.Spec.Containers) != 1 {
			t.Errorf("Unexpected number of containers in pod: expected 1, got %d", len(pod.Spec.Containers))
		}

		container := pod.Spec.Containers[0]
		expectedMountPath := "/tmp"
		if len(container.VolumeMounts) != 1 {
			t.Errorf("Unexpected number of volumeMounts: expected 1, got %d", len(container.VolumeMounts))
		} else if container.VolumeMounts[0].Name != kubeconfigSecretKey {
			t.Errorf("Unexpected mount name for kubeconfig secret: expected '%s', got '%s'", kubeconfigSecretKey, container.VolumeMounts[0].Name)
		} else if container.VolumeMounts[0].MountPath != expectedMountPath {
			t.Errorf("Unexpected mount path for kubeconfig secret: expected '%s', got '%s'", expectedMountPath, container.VolumeMounts[0].MountPath)
		}

		expectedEnvValue := fmt.Sprintf("/tmp/%s", kubeconfigSecretKey)
		if len(container.Env) != 1 {
			t.Errorf("Unexpected number of environment variables: expected 1, got %d", len(container.Env))
		} else if container.Env[0].Name != "KUBECONFIG" {
			t.Errorf("Unexpected environment variable set: expected 'KUBECONFIG', got '%s'", container.Env[0].Name)
		} else if container.Env[0].Value != expectedEnvValue {
			t.Errorf("Unexpected environment variable set: expected 'KUBECONFIG', got '%s'", container.Env[0].Name)
		}
	}
}

// TestUpdateEnv ensures the updateEnv function is operating correctly
func TestUpdateEnv(t *testing.T) {
	tests := []struct {
		Name   string
		Input  string
		Expect bool
	}{
		{
			Name:   "Input Y",
			Input:  "Y",
			Expect: true,
		},
		{
			Name:   "Input y",
			Input:  "y",
			Expect: true,
		},
		{
			Name:   "Input nothing",
			Input:  "",
			Expect: false,
		},
		{
			Name:   "Input garbage",
			Input:  "%23dslkjd",
			Expect: false,
		},
	}
	for _, test := range tests {
		fmt.Printf("Testing '%s'\n", test.Name)
		result := isAffirmative(test.Input)
		if result != test.Expect {
			t.Errorf("Failed '%s': expected %t, got %t", test.Name, test.Expect, result)
		}
	}
}

// generateClusterObjectForTesting creates a non-functional cluster object solely for testing purposes
func generateClusterObjectForTesting(name string, id string, privateLink bool, private bool) clustersmgmtv1.Cluster {
	var listen clustersmgmtv1.ListeningMethod
	if private {
		listen = clustersmgmtv1.ListeningMethodInternal
	} else {
		listen = clustersmgmtv1.ListeningMethodExternal
	}
	cluster, err := clustersmgmtv1.NewCluster().
		Name(name).
		ID(id).
		AWS(clustersmgmtv1.NewAWS().PrivateLink(privateLink)).
		API(clustersmgmtv1.NewClusterAPI().Listening(listen)).
		Build()

	if err != nil {
		panic(fmt.Sprintf("Failed to build cluster: %v", err))
	}
	return *cluster
}

// generateKubeconfigSecretObjectForTesting creates a Secret containing a kubeconfig file for testing purposes
func generateKubeconfigSecretObjectForTesting(name, namespace, key, serverURL string) (corev1.Secret, clientcmdapiv1.Config) {
	kubeconfig := clientcmdapiv1.Config{
		Clusters: []clientcmdapiv1.NamedCluster{
			{
				Cluster: clientcmdapiv1.Cluster{
					Server: serverURL,
				},
			},
		},
	}
	rawKubeconfig, err := json.Marshal(kubeconfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal kubeconfig: %v", err))
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{key: rawKubeconfig},
	}
	return secret, kubeconfig
}
