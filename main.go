package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/martian/v3/log"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

func newCommand() *cobra.Command {
	var kubeconfig string
	var exportCfg exportConfig
	cmd := &cobra.Command{
		Use: "resources-usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetLevel(log.Debug)
			restCfg, err := newRESTConfig(kubeconfig)
			if err != nil {
				return err
			}
			if exportCfg.KubeClient, err = kubernetes.NewForConfig(restCfg); err != nil {
				return err
			}
			if exportCfg.MetricClient, err = metrics.NewForConfig(restCfg); err != nil {
				return err
			}
			return export(exportCfg)
		},
	}
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Kubernetes config file. Create a local config if no specified")
	cmd.Flags().StringSliceVar(&exportCfg.IgnoreNamespaces, "ignore-namespaces", []string{"default", "kube-node-lease", "kube-public", "kube-system"}, "Ignore namespaces")
	return cmd
}

func main() {
	if err := newCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRESTConfig(kubeconfig string) (*rest.Config, error) {
	var fullKubeConfigPath string
	var err error

	if kubeconfig != "" {
		fullKubeConfigPath, err = filepath.Abs(kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("cannot expand path %s: %v", kubeconfig, err)
		}
	}

	if fullKubeConfigPath != "" {
		log.Debugf("Creating Kubernetes client from %s", fullKubeConfigPath)
	} else {
		log.Debugf("Creating in-cluster Kubernetes client")
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	loadingRules.ExplicitPath = kubeconfig
	overrides := clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, &overrides, os.Stdin)
	raw, _ := clientConfig.RawConfig()
	log.Debugf("Current Context: %s", raw.CurrentContext)
	return clientConfig.ClientConfig()
}
