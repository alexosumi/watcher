package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	watcherv1 "github.com/alex.osumi/watcher/api/v1"
)

const (
	DefaultMemoryThreshold              = 80
	DefaultScaleUpPercentage            = 50
	DefaultMemoryRequest                = "100Mi"
	DefaultMode                         = "patch"
	MaxNodeMemoryUsagePercent           = 99
	PodDelayMilliseconds                = 300
	DefaultWatcherReconcileIntervalSecs = 60
	MetricsTimeoutSeconds               = 10
)

// getThresholdOrDefault returns the memory threshold or default value
func getThresholdOrDefault(threshold int) int {
	if threshold == 0 {
		return DefaultMemoryThreshold
	}
	return threshold
}

// getScalePercentOrDefault returns the scale percentage or default value
func getScalePercentOrDefault(scalePercent int) int {
	if scalePercent == 0 {
		return DefaultScaleUpPercentage
	}
	return scalePercent
}

// getModeOrDefault returns the mode or default value
func getModeOrDefault(mode string) string {
	if mode == "" {
		return DefaultMode
	}
	return mode
}

// buildListOptions creates list options for pod filtering
func buildListOptions(watcher *watcherv1.Watcher) *client.ListOptions {
	listOptions := &client.ListOptions{
		Namespace: watcher.Spec.Namespace,
	}
	
	if len(watcher.Spec.LabelSelector) > 0 {
		labelSelector := labels.SelectorFromSet(watcher.Spec.LabelSelector)
		listOptions.LabelSelector = labelSelector
	}
	
	return listOptions
}

// getMemoryRequestOrDefault returns the memory request or default value
func getMemoryRequestOrDefault(memRequest resource.Quantity) resource.Quantity {
	if memRequest.IsZero() {
		return resource.MustParse(DefaultMemoryRequest)
	}
	return memRequest
}

// calculateUsagePercent calculates memory usage percentage
func calculateUsagePercent(usage, request resource.Quantity) float64 {
	return float64(usage.MilliValue()) / float64(request.MilliValue()) * 100
}

// convertToMB converts bytes to megabytes
func convertToMB(bytes resource.Quantity) float64 {
	return float64(bytes.Value()) / (1024 * 1024)
}

func watcherReconcileInterval(seconds int32) time.Duration {
	if seconds < 5 {
		seconds = DefaultWatcherReconcileIntervalSecs
	}
	return time.Duration(seconds) * time.Second
}
