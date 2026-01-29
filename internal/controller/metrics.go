package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// fetchPodMetrics retrieves metrics for a specific pod
func (r *WatcherReconciler) fetchPodMetrics(ctx context.Context, pod *corev1.Pod) (*metricsv1beta1.PodMetrics, error) {
	metricsCtx, cancel := context.WithTimeout(ctx, MetricsTimeoutSeconds*time.Second)
	defer cancel()
	
	return r.MetricsClient.MetricsV1beta1().PodMetricses(pod.Namespace).Get(metricsCtx, pod.Name, metav1.GetOptions{})
}

// fetchNodeMetrics retrieves metrics for a specific node
func (r *WatcherReconciler) fetchNodeMetrics(ctx context.Context, nodeName string) (*metricsv1beta1.NodeMetrics, error) {
	metricsCtx, cancel := context.WithTimeout(ctx, MetricsTimeoutSeconds*time.Second)
	defer cancel()
	
	return r.MetricsClient.MetricsV1beta1().NodeMetricses().Get(metricsCtx, nodeName, metav1.GetOptions{})
}

// fetchNode retrieves node information
func (r *WatcherReconciler) fetchNode(ctx context.Context, nodeName string) (*corev1.Node, error) {
	node := &corev1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}
	return node, nil
}

// findContainerMetrics finds metrics for a specific container
func findContainerMetrics(podMetrics *metricsv1beta1.PodMetrics, containerName string) (*metricsv1beta1.ContainerMetrics, error) {
	for _, cm := range podMetrics.Containers {
		if cm.Name == containerName {
			return &cm, nil
		}
	}
	return nil, fmt.Errorf("container metrics not found for %s", containerName)
}
