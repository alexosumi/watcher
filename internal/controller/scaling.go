package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// calculateNewMemoryRequest calculates the new memory request based on current usage and node capacity
func (r *WatcherReconciler) calculateNewMemoryRequest(currentRequest, currentUsage resource.Quantity, scalePercent int, nodeMetrics *metricsv1beta1.NodeMetrics, node *corev1.Node) resource.Quantity {
	increaseAmount := currentRequest.DeepCopy()
	increaseAmount.Set(increaseAmount.Value() * int64(scalePercent) / 100)
	proposedRequest := currentRequest.DeepCopy()
	proposedRequest.Add(increaseAmount)

	nodeCapacity := node.Status.Capacity[corev1.ResourceMemory]
	nodeUsage := nodeMetrics.Usage[corev1.ResourceMemory]
	
	maxNodeUsage := nodeCapacity.DeepCopy()
	maxNodeUsage.Set(maxNodeUsage.Value() * MaxNodeMemoryUsagePercent / 100)
	
	availableMemory := maxNodeUsage.DeepCopy()
	availableMemory.Sub(nodeUsage)
	availableMemory.Add(currentRequest)

	if proposedRequest.Cmp(availableMemory) > 0 {
		return availableMemory
	}
	return proposedRequest
}

// resizePodMemory performs in-place pod memory resize or logs recommendation based on mode
func (r *WatcherReconciler) resizePodMemory(ctx context.Context, pod *corev1.Pod, containerIndex int, newRequest resource.Quantity, currentUsage resource.Quantity, usagePercent float64, mode string) error {
	logger := log.FromContext(ctx)
	containerName := pod.Spec.Containers[containerIndex].Name
	oldRequest := pod.Spec.Containers[containerIndex].Resources.Requests[corev1.ResourceMemory]
	oldLimit := pod.Spec.Containers[containerIndex].Resources.Limits[corev1.ResourceMemory]
	usageMB := convertToMB(currentUsage)
	
	if mode == "watch" {
		logger.Info("Memory scaling recommendation",
			"pod", pod.Name,
			"container", containerName,
			"currentUsageMB", fmt.Sprintf("%.2f", usageMB),
			"currentUsagePercent", fmt.Sprintf("%.2f%%", usagePercent),
			"currentMemory", oldRequest.String(),
			"recommendedMemory", newRequest.String())
		return nil
	}
	
	patchData := fmt.Sprintf(`{
		"spec": {
			"containers": [
				{
					"name": "%s",
					"resources": {
						"requests": { "memory": "%s" },
						"limits": { "memory": "%s" }
					}
				}
			]
		}
	}`, containerName, newRequest.String(), newRequest.String())
	
	logger.Info("Resizing pod memory in-place", 
		"pod", pod.Name, 
		"container", containerName, 
		"currentUsageMB", fmt.Sprintf("%.2f", usageMB),
		"currentUsagePercent", fmt.Sprintf("%.2f%%", usagePercent),
		"oldConfig", fmt.Sprintf(`{"requests":{"memory":"%s"},"limits":{"memory":"%s"}}`, oldRequest.String(), oldLimit.String()),
		"newMemory", newRequest.String())
	
	_, err := r.K8sClient.CoreV1().Pods(pod.Namespace).Patch(
		ctx,
		pod.Name,
		"application/strategic-merge-patch+json",
		[]byte(patchData),
		metav1.PatchOptions{},
		"resize",
	)
	
	if err != nil {
		return fmt.Errorf("failed to resize pod: %w", err)
	}
	
	return nil
}
