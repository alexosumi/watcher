package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	watcherv1 "github.com/alex.osumi/watcher/api/v1"
)

// WatcherReconciler reconciles a Watcher object
type WatcherReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	K8sClient     kubernetes.Interface
	MetricsClient metricsclientset.Interface
}

//+kubebuilder:rbac:groups=watcher.io,resources=watchers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=watcher.io,resources=watchers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=watcher.io,resources=watchers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups="",resources=pods/resize,verbs=get;patch
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=metrics.k8s.io,resources=pods;nodes,verbs=get;list

func (r *WatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var watcher watcherv1.Watcher
	if err := r.Get(ctx, req.NamespacedName, &watcher); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	threshold := getThresholdOrDefault(watcher.Spec.MemoryThreshold)
	scalePercent := getScalePercentOrDefault(watcher.Spec.ScaleUpPercentage)
	mode := getModeOrDefault(watcher.Spec.Mode)
	requeueAfter := watcherReconcileInterval(watcher.Spec.ReconcileIntervalSeconds)

	logger.Info("reconciling Watcher CR",
		"namespace", req.Namespace,
		"name", req.Name,
		"targetNamespace", watcher.Spec.Namespace,
		"mode", mode,
		"memoryThreshold", threshold,
		"scaleUpPercentage", scalePercent,
		"reconcileInterval", requeueAfter.String(),
	)

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, buildListOptions(&watcher)); err != nil {
		logger.Error(err, "Failed to list pods")
		return ctrl.Result{}, err
	}

	monitoredPods := r.processRunningPods(ctx, podList, threshold, scalePercent, mode)
	logger.Info("reconcile complete",
		"name", req.Name,
		"totalPods", len(podList.Items),
		"monitoredPods", monitoredPods,
	)

	if monitoredPods != watcher.Status.MonitoredPods {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			var latest watcherv1.Watcher
			if err := r.Get(ctx, req.NamespacedName, &latest); err != nil {
				return err
			}
			latest.Status.MonitoredPods = monitoredPods
			return r.Status().Update(ctx, &latest)
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctrl.Result{RequeueAfter: requeueAfter}, nil
			}
			logger.Error(err, "Failed to update watcher status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *WatcherReconciler) processRunningPods(ctx context.Context, podList *corev1.PodList, threshold, scalePercent int, mode string) int {
	logger := log.FromContext(ctx)
	monitoredPods := 0
	runningPodCount := 0

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		if runningPodCount > 0 {
			time.Sleep(PodDelayMilliseconds * time.Millisecond)
		}
		runningPodCount++

		if err := r.processPod(ctx, &pod, threshold, scalePercent, mode); err != nil {
			logger.V(1).Info("Skipped pod", "pod", pod.Name, "reason", err.Error())
			continue
		}
		monitoredPods++
	}

	return monitoredPods
}

func (r *WatcherReconciler) processPod(ctx context.Context, pod *corev1.Pod, threshold, scalePercent int, mode string) error {
	logger := log.FromContext(ctx)

	freshPod := &corev1.Pod{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: pod.Name}, freshPod); err != nil {
		return fmt.Errorf("failed to get fresh pod: %w", err)
	}

	podMetrics, err := r.fetchPodMetrics(ctx, freshPod)
	if err != nil {
		return fmt.Errorf("no metrics available: %w", err)
	}

	nodeMetrics, err := r.fetchNodeMetrics(ctx, freshPod.Spec.NodeName)
	if err != nil {
		return fmt.Errorf("failed to get node metrics: %w", err)
	}

	node, err := r.fetchNode(ctx, freshPod.Spec.NodeName)
	if err != nil {
		return err
	}

	for i, container := range freshPod.Spec.Containers {
		if err := r.processContainer(ctx, freshPod, &container, i, podMetrics, nodeMetrics, node, threshold, scalePercent, mode); err != nil {
			logger.Error(err, "Failed to process container", "container", container.Name)
		}
	}

	return nil
}

func (r *WatcherReconciler) processContainer(ctx context.Context, pod *corev1.Pod, container *corev1.Container, containerIndex int, podMetrics *metricsv1beta1.PodMetrics, nodeMetrics *metricsv1beta1.NodeMetrics, node *corev1.Node, threshold, scalePercent int, mode string) error {
	containerMetrics, err := findContainerMetrics(podMetrics, container.Name)
	if err != nil {
		return err
	}

	currentUsage := containerMetrics.Usage[corev1.ResourceMemory]
	currentRequest := getMemoryRequestOrDefault(container.Resources.Requests[corev1.ResourceMemory])
	usagePercent := calculateUsagePercent(currentUsage, currentRequest)

	if usagePercent < float64(threshold) {
		return nil
	}

	newRequest := r.calculateNewMemoryRequest(currentRequest, currentUsage, scalePercent, nodeMetrics, node)
	if newRequest.Cmp(currentRequest) <= 0 {
		return nil
	}

	return r.resizePodMemory(ctx, pod, containerIndex, newRequest, currentUsage, usagePercent, mode)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&watcherv1.Watcher{}).
		Complete(r)
}
