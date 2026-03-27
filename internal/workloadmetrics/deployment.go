package workloadmetrics

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MetricKind matches api v1 WorkloadMetric values.
type MetricKind string

const (
	MetricCPU    MetricKind = "CPU"
	MetricMemory MetricKind = "Memory"
)

// MaxUsageForDeployment returns the max over pods of (max over that pod's containers).
// CPU is in millicores; Memory is in Mebibytes (Mi).
func MaxUsageForDeployment(
	ctx context.Context,
	k8s client.Client,
	metrics metricsclient.Interface,
	namespace, deploymentName string,
	metric MetricKind,
) (float64, error) {
	var dep appsv1.Deployment
	key := types.NamespacedName{Namespace: namespace, Name: deploymentName}
	if err := k8s.Get(ctx, key, &dep); err != nil {
		if errors.IsNotFound(err) {
			return 0, fmt.Errorf("deployment %s/%s not found", namespace, deploymentName)
		}
		return 0, err
	}
	if dep.Spec.Selector == nil {
		return 0, fmt.Errorf("deployment %s/%s has nil selector", namespace, deploymentName)
	}
	sel, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return 0, err
	}
	var podList corev1.PodList
	if err := k8s.List(ctx, &podList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return 0, err
	}
	if len(podList.Items) == 0 {
		return 0, fmt.Errorf("no pods for deployment %s/%s", namespace, deploymentName)
	}

	var podMax float64
	for i := range podList.Items {
		pm, err := metrics.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podList.Items[i].Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return 0, fmt.Errorf("metrics not yet available for pod %s/%s", namespace, podList.Items[i].Name)
			}
			return 0, fmt.Errorf("pod metrics %s/%s: %w", namespace, podList.Items[i].Name, err)
		}
		containerMax, err := maxContainerUsage(pm, metric)
		if err != nil {
			return 0, err
		}
		if containerMax > podMax {
			podMax = containerMax
		}
	}
	return podMax, nil
}

func maxContainerUsage(pm *metricsv1beta1.PodMetrics, metric MetricKind) (float64, error) {
	var m float64
	if len(pm.Containers) == 0 {
		return 0, fmt.Errorf("pod %s has no container metrics", pm.Name)
	}
	for _, c := range pm.Containers {
		switch metric {
		case MetricCPU:
			v := float64(c.Usage.Cpu().MilliValue())
			if v > m {
				m = v
			}
		case MetricMemory:
			mi := float64(c.Usage.Memory().Value()) / (1024 * 1024)
			if mi > m {
				m = mi
			}
		default:
			return 0, fmt.Errorf("unknown metric %q", metric)
		}
	}
	return m, nil
}
