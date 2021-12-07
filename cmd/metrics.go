package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/osdctl/pkg/prom"
	"github.com/spf13/cobra"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
)

const (
	operatorRoute          = common.AWSAccountNamespace
	operatorServiceAccount = common.AWSAccountNamespace
)

// newCmdMetrics displays the metrics of aws-account-operator
func newCmdMetrics(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newMetricsOptions(streams, flags, client)
	resetCmd := &cobra.Command{
		Use:               "metrics",
		Short:             "Display metrics of aws-account-operator",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	resetCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	resetCmd.Flags().StringVarP(&ops.metricsURL, "metrics-url", "m", "", "The URL of aws-account-operator metrics endpoint. "+
		"Used only for debug purpose! Only HTTP scheme is supported.")
	resetCmd.Flags().StringVarP(&ops.routeName, "route", "r", operatorRoute, "The route created for aws-account-operator")
	resetCmd.Flags().StringVar(&ops.saName, "sa", operatorServiceAccount, "The service account name for aws-account-operator")
	resetCmd.Flags().BoolVar(&ops.useHTTPS, "https", false, "Use HTTPS to access metrics or not. By default we use HTTP scheme.")

	return resetCmd
}

// metricsOptions defines the struct for running metrics command
type metricsOptions struct {
	accountNamespace string
	routeName        string
	saName           string
	useHTTPS         bool

	// the URL of aws-account-operator metrics endpoint
	metricsURL string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newMetricsOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *metricsOptions {
	return &metricsOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *metricsOptions) complete(cmd *cobra.Command) error {
	// account CR name and account ID cannot be empty at the same time
	if o.metricsURL == "" && o.routeName == "" {
		return cmdutil.UsageErrorf(cmd, "Metrics URL and route name cannot be empty at the same time")
	}

	return nil
}

func (o *metricsOptions) run() error {
	var (
		metricsEndpoint string
		resp            *http.Response
		err             error
	)

	ctx := context.TODO()

	if o.metricsURL != "" {
		metricsEndpoint = o.metricsURL
		resp, err = http.Get(metricsEndpoint)
	} else {
		key := types.NamespacedName{
			Namespace: o.accountNamespace,
			Name:      o.routeName,
		}

		var route routev1.Route
		if err := o.kubeCli.Get(ctx, key, &route); err != nil {
			return err
		}

		if o.useHTTPS {
			var sa v1.ServiceAccount
			if err := o.kubeCli.Get(ctx, types.NamespacedName{
				Namespace: o.accountNamespace,
				Name:      o.saName,
			}, &sa); err != nil {
				return err
			}

			var secretName string
			for _, v := range sa.Secrets {
				if strings.Contains(v.Name, "token") {
					secretName = v.Name
				}
			}
			if secretName == "" {
				return fmt.Errorf("secret for service account %s doesn't exist", o.saName)
			}

			var secret v1.Secret
			if err := o.kubeCli.Get(ctx, types.NamespacedName{
				Namespace: o.accountNamespace,
				Name:      secretName,
			}, &secret); err != nil {
				return err
			}

			token, ok := secret.Data["token"]
			if !ok {
				return fmt.Errorf("secret %s doesn't have token field", secretName)
			}
			metricsEndpoint = "https://" + route.Spec.Host + "/metrics"
			req, err := http.NewRequest(http.MethodGet, metricsEndpoint, nil)
			if err != nil {
				return err
			}

			req.Header.Add("Authorization", "Bearer "+string(token))
			http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			resp, err = http.DefaultClient.Do(req)
		} else {
			metricsEndpoint = "http://" + route.Spec.Host + "/metrics"
			resp, err = http.Get(metricsEndpoint)
		}
	}

	if err != nil {
		return err
	}
	defer resp.Body.Close()
	metrics, err := prom.DecodeMetrics(resp.Body, map[string]string{"name": "aws-account-operator"})
	if err != nil {
		return err
	}

	for _, v := range metrics {
		fmt.Fprintln(o.IOStreams.Out, v)
	}

	return nil
}
