package network

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osd-utils-cli/pkg/k8s"
)

const (
	packetCaptureImage       = "quay.io/jharrington22/network-toolbox:latest"
	packetCaptureName        = "packet-capture"
	packetCaptureNamespace   = "default"
	outputDir                = "capture-output"
	nodeLabelKey             = "node-role.kubernetes.io/worker"
	nodeLabelValue           = ""
	packetCaptureDurationSec = 60
)

// newCmdPacketCapture implements the packet-capture command to run a packet capture
func newCmdPacketCapture(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newPacketCaptureOptions(streams, flags)
	packetCaptureCmd := &cobra.Command{
		Use:               "packet-capture",
		Aliases:           []string{"pcap"},
		Short:             "Start packet capture",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	packetCaptureCmd.Flags().IntVarP(&ops.duration, "duration", "d", packetCaptureDurationSec, "Duration (in seconds) of packet capture")
	packetCaptureCmd.Flags().StringVarP(&ops.name, "name", "", packetCaptureName, "Name of Daemonset")
	packetCaptureCmd.Flags().StringVarP(&ops.namespace, "namespace", "n", packetCaptureNamespace, "Namespace to deploy Daemonset")
	packetCaptureCmd.Flags().StringVarP(&ops.nodeLabelKey, "node-label-key", "", nodeLabelKey, "Node label key")
	packetCaptureCmd.Flags().StringVarP(&ops.nodeLabelValue, "node-label-value", "", nodeLabelValue, "Node label value")

	ops.startTime = time.Now()
	return packetCaptureCmd
}

// packetCaptureOptions defines the struct for running packet-capture command
type packetCaptureOptions struct {
	name           string
	namespace      string
	nodeLabelKey   string
	nodeLabelValue string
	duration       int

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli   client.Client
	startTime time.Time
}

func newPacketCaptureOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *packetCaptureOptions {
	return &packetCaptureOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *packetCaptureOptions) complete(cmd *cobra.Command, _ []string) error {
	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}
	return nil
}

func (o *packetCaptureOptions) run() error {
	log.Println("Ensuring Packet Capture Daemonset")
	ds, err := ensurePacketCaptureDaemonSet(o)
	if err != nil {
		log.Fatalf("Error ensuring packet capture daemonset %v", err)
		return err
	}
	log.Println("Waiting For Packet Capture Daemonset")
	err = waitForPacketCaptureDaemonset(o, ds)
	if err != nil {
		log.Fatalf("Error Waiting for daemonset %v", err)
		return err
	}
	log.Println("Copying Files From Packet Capture Pods")
	err = copyFilesFromPacketCapturePods(o)
	if err != nil {
		log.Fatalf("Error copying files %v", err)
		return err
	}
	log.Println("Deleting Packet Capture Daemonset")
	err = deletePacketCaptureDaemonSet(o, ds)
	if err != nil {
		log.Fatalf("Error deleting packet capture daemonset %v", err)
		return err
	}
	return nil
}

// ensurePacketCaptureDaemonSet ensures the daemonset exists
func ensurePacketCaptureDaemonSet(o *packetCaptureOptions) (*appsv1.DaemonSet, error) {
	key := types.NamespacedName{Name: o.name, Namespace: o.namespace}
	desired := desiredPacketCaptureDaemonSet(o, key)
	haveDs, current, err := currentPacketCaptureDaemonSet(o, key)
	if err != nil {
		log.Fatalf("Error getting current daemonset %v", err)
		return nil, err
	}

	if haveDs {
		log.Println("Already have packet-capture daemonset")
		return current, nil
	}

	err = createPacketCaptureDaemonSet(o, desired)
	if err != nil {
		log.Fatalf("Error creating packet capture daemonset %v", err)
		return nil, err
	}

	log.Println("Successfully ensured packet capture daemonset")
	return desired, nil
}

// currentPacketCaptureDaemonSet returns the current daemonset
func currentPacketCaptureDaemonSet(o *packetCaptureOptions, key types.NamespacedName) (bool, *appsv1.DaemonSet, error) {
	ds := &appsv1.DaemonSet{}

	if err := o.kubeCli.Get(context.TODO(), key, ds); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, ds, nil
}

// createPacketCaptureDaemonSet creates the given daemonset resource
func createPacketCaptureDaemonSet(o *packetCaptureOptions, ds *appsv1.DaemonSet) error {
	if err := o.kubeCli.Create(context.TODO(), ds); err != nil {
		return fmt.Errorf("failed to create daemonset %s/%s: %v", ds.Namespace, ds.Name, err)
	}
	return nil
}

// deletePacketCaptureDaemonSet creates the given daemonset resource
func deletePacketCaptureDaemonSet(o *packetCaptureOptions, ds *appsv1.DaemonSet) error {
	if err := o.kubeCli.Delete(context.TODO(), ds); err != nil {
		return fmt.Errorf("failed to delete daemonset %s/%s: %v", ds.Namespace, ds.Name, err)
	}
	return nil
}

// desiredPacketCaptureDaemonSet returns the desired daemonset read in from manifests
func desiredPacketCaptureDaemonSet(o *packetCaptureOptions, key types.NamespacedName) *appsv1.DaemonSet {
	ds := &appsv1.DaemonSet{}
	t := true
	ls := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": key.Name,
		},
	}
	ds.Name = key.Name
	ds.Namespace = key.Namespace

	ds.Spec.Selector = ls
	ds.Spec.Template.Spec.NodeSelector = map[string]string{
		o.nodeLabelKey: o.nodeLabelValue,
	}
	ds.Spec.Template.Labels = ls.MatchLabels
	ds.Spec.Template.Spec.Tolerations = []corev1.Toleration{
		{
			Effect:   "NoSchedule",
			Key:      o.nodeLabelKey,
			Operator: "Exists",
		},
	}
	ds.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "capture-output",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	ds.Spec.Template.Spec.HostNetwork = true
	ds.Spec.Template.Spec.InitContainers = []corev1.Container{
		{
			Name:            "init-capture",
			Image:           packetCaptureImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"/bin/bash", "-c", "tcpdump -G " + strconv.Itoa(o.duration) + " -W 1 -w /tmp/capture-output/capture.pcap -i vxlan_sys_4789 -nn -s0; sync"},
			SecurityContext: &corev1.SecurityContext{Privileged: &t},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "capture-output",
					MountPath: "/tmp/capture-output",
					ReadOnly:  false,
				},
			},
		},
	}
	ds.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:            "copy",
			Image:           packetCaptureImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"/bin/bash", "-c", "trap : TERM INT; sleep infinity & wait"},
			SecurityContext: &corev1.SecurityContext{Privileged: &t},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "capture-output",
					MountPath: "/tmp/capture-output",
					ReadOnly:  false,
				},
			},
		},
	}

	return ds
}

func copyFilesFromPod(o *packetCaptureOptions, pod *corev1.Pod) error {
	os.MkdirAll(outputDir, 0755)
	fileName := fmt.Sprintf("%s-%s.pcap", pod.Spec.NodeName, o.startTime.UTC().Format("20060102T150405"))
	cmd := exec.Command("oc", "cp", pod.Namespace+"/"+pod.Name+":/tmp/capture-output/capture.pcap", outputDir+"/"+fileName)
	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)

	cmd.Stdout = mw
	cmd.Stderr = mw

	err := cmd.Run()

	if err != nil {
		log.Println(stdBuffer.String())
	}

	return err
}

func waitForPacketCaptureDaemonset(o *packetCaptureOptions, ds *appsv1.DaemonSet) error {
	pollErr := wait.PollImmediate(10*time.Second, time.Duration(600)*time.Second, func() (bool, error) {
		var err error
		tmp := &appsv1.DaemonSet{}
		key := types.NamespacedName{Name: ds.Name, Namespace: ds.Namespace}
		if err = o.kubeCli.Get(context.TODO(), key, tmp); err == nil {
			ready := (tmp.Status.NumberReady > 0 &&
				tmp.Status.NumberAvailable == tmp.Status.NumberReady &&
				tmp.Status.NumberReady == tmp.Status.DesiredNumberScheduled)
			return ready, nil
		}
		return false, err
	})
	return pollErr
}

func waitForPacketCaptureContainerRunning(o *packetCaptureOptions, pod *corev1.Pod) error {
	pollErr := wait.PollImmediate(10*time.Second, time.Duration(600)*time.Second, func() (bool, error) {
		var err error
		tmp := &corev1.Pod{}
		key := types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}
		if err = o.kubeCli.Get(context.TODO(), key, tmp); err == nil {
			if len(tmp.Status.ContainerStatuses) == 0 {
				return false, nil
			}
			state := tmp.Status.ContainerStatuses[0].State
			running := state.Running != nil
			return running, nil
		}
		return false, err
	})
	return pollErr
}

func copyFilesFromPacketCapturePods(o *packetCaptureOptions) error {
	var pods corev1.PodList

	if err := o.kubeCli.List(context.TODO(), &pods, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{"app": o.name}),
		Namespace:     o.namespace,
	}); err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if len(pod.Status.ContainerStatuses) == 0 {
			continue
		}
		err := waitForPacketCaptureContainerRunning(o, &pod)
		if err != nil {
			log.Fatalf("Error waiting for pods %v", err)
			return err
		}
		log.Printf("Copying files from %s\n", pod.Name)
		err = copyFilesFromPod(o, &pod)
		if err != nil {
			log.Fatalf("error copying files %v", err)
			return err
		}
	}

	return nil
}
