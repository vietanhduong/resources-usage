package main

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

type exportConfig struct {
	KubeClient   *kubernetes.Clientset
	MetricClient *metrics.Clientset

	IgnoreNamespaces []string
}

type Resources struct {
	CPU    resource.Quantity
	Memory resource.Quantity
}

type Service struct {
	Kind      string
	Namespace string
	Name      string
	Replicas  int32
	Usage     Resources
	Request   Resources
}

func (o *Service) CSV() string {
	if o == nil {
		return ""
	}
	cpu := fmt.Sprintf("%vm/unlimit", o.Usage.CPU.MilliValue())
	if !o.Request.CPU.IsZero() {
		cpu = fmt.Sprintf("%vm/%vm", o.Usage.CPU.MilliValue(), o.Request.CPU.MilliValue())
	}
	memory := fmt.Sprintf("%vMi/unlimit", o.Usage.Memory.Value()/(1024*1024))
	if !o.Request.Memory.IsZero() {
		memory = fmt.Sprintf("%vMi/%vMi", o.Usage.Memory.Value()/(1024*1024), o.Request.Memory.Value()/(1024*1024))
	}

	return fmt.Sprintf("%s,%s,%s,%d,%s,%s",
		o.Namespace,
		o.Name,
		o.Kind,
		o.Replicas,
		cpu,
		memory)
}

func export(cfg exportConfig) error {
	ignoreNamespaces := sets.New(cfg.IgnoreNamespaces...)
	fmt.Fprintln(os.Stdout, "Namespace,Name,Kind,Replicas,CPU Usage/CPU Request(m),Memory Usage/Memory Request(Mi)")
	// list all namespaces
	namespaces, err := cfg.KubeClient.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, ns := range namespaces.Items {
		if ignoreNamespaces.Has(ns.GetName()) {
			continue
		}

		// handle deployments
		var deploys []Service
		if deploys, err = handleExportDeployments(cfg, ns); err != nil {
			return err
		}
		// handle sts
		var stses []Service
		if stses, err = handleStatefulSets(cfg, ns); err != nil {
			return err
		}
		for _, e := range deploys {
			fmt.Fprintln(os.Stdout, e.CSV())
		}
		for _, e := range stses {
			fmt.Fprintln(os.Stdout, e.CSV())
		}
	}
	return nil
}

func handleExportDeployments(cfg exportConfig, ns corev1.Namespace) ([]Service, error) {
	deploys, err := cfg.KubeClient.AppsV1().Deployments(ns.GetName()).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	services := make([]Service, len(deploys.Items))

	for i, deploy := range deploys.Items {
		selector, err := metav1.LabelSelectorAsMap(deploy.Spec.Selector)
		if err != nil {
			return nil, err
		}
		services[i] = Service{
			Kind:      "Deployment",
			Namespace: deploy.Namespace,
			Name:      deploy.Name,
		}
		for _, container := range deploy.Spec.Template.Spec.Containers {
			services[i].Request.CPU.Add(*container.Resources.Requests.Cpu())
			services[i].Request.Memory.Add(*container.Resources.Requests.Memory())
		}
		query := metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(selector).String(),
		}
		podMetrics, err := cfg.MetricClient.MetricsV1beta1().PodMetricses(ns.GetName()).List(context.Background(), query)
		if err != nil {
			return nil, err
		}
		services[i].Replicas = int32(len(podMetrics.Items))
		for _, m := range podMetrics.Items {
			for _, container := range m.Containers {
				services[i].Usage.CPU.Add(*container.Usage.Cpu())
				services[i].Usage.Memory.Add(*container.Usage.Memory())
			}
		}
	}
	return services, nil
}

func handleStatefulSets(cfg exportConfig, ns corev1.Namespace) ([]Service, error) {
	stses, err := cfg.KubeClient.AppsV1().StatefulSets(ns.GetName()).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	services := make([]Service, len(stses.Items))

	for i, sts := range stses.Items {
		selector, err := metav1.LabelSelectorAsMap(sts.Spec.Selector)
		if err != nil {
			return nil, err
		}
		services[i] = Service{
			Kind:      "StatefulSets",
			Namespace: sts.Namespace,
			Name:      sts.Name,
		}
		for _, container := range sts.Spec.Template.Spec.Containers {
			services[i].Request.CPU.Add(*container.Resources.Requests.Cpu())
			services[i].Request.Memory.Add(*container.Resources.Requests.Memory())
		}
		query := metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(selector).String(),
		}
		podMetrics, err := cfg.MetricClient.MetricsV1beta1().PodMetricses(ns.GetName()).List(context.Background(), query)
		if err != nil {
			return nil, err
		}
		services[i].Replicas = int32(len(podMetrics.Items))
		for _, m := range podMetrics.Items {
			for _, container := range m.Containers {
				services[i].Usage.CPU.Add(*container.Usage.Cpu())
				services[i].Usage.Memory.Add(*container.Usage.Memory())
			}
		}
	}
	return services, nil
}
