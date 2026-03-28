package controller

import (
	"context"
	"fmt"
	"math"
	"time"

	api "github.com/alex.osumi/watcher/api/v1"
	"github.com/alex.osumi/watcher/internal/airflow"
	"github.com/alex.osumi/watcher/internal/workloadmetrics"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	defaultMinRunningPercent int32 = 95

	conditionReady                = "Ready"
	reasonReconciled              = "Reconciled"
	reasonAirflowError            = "AirflowError"
	reasonMissingScaleSignal      = "MissingScaleSignal"
	reasonWorkloadMetricsError    = "WorkloadMetricsError"
	reasonPatchedDefault          = "PatchedDefaultSlots"
	reasonPatchedIncrease         = "PatchedIncreasedSlots"
	reasonSkippedAlreadyDefault   = "SkippedAlreadyDefault"
	reasonSkippedNoIncreaseNeeded = "SkippedNoIncreaseNeeded"
	reasonSkippedHighUtilization  = "SkippedHighUtilization"
	reasonSkippedLowRunningUtil   = "SkippedLowRunningUtilization"
	reasonDryRunWouldPatch        = "DryRunWouldPatch"
)

// PoolReconciler adjusts Airflow pool slots from a Pool CR.
type PoolReconciler struct {
	client.Client
	Metrics        metricsclient.Interface
	Airflow        *airflow.Client
	Scheme         *runtime.Scheme
	MaxConcurrent  int
}

// +kubebuilder:rbac:groups=watcher.io,resources=pools,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=watcher.io,resources=pools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=metrics.k8s.io,resources=pods,verbs=get;list

// Reconcile implements the reconcile loop.
func (r *PoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	var pool api.Pool
	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	sig := pool.Spec.ScaleSignal
	if sig == "" {
		sig = api.PoolScaleSignalScheduled
	}
	requeueAfter := poolReconcileInterval(pool.Spec.ReconcileIntervalSeconds)

	lg.Info("reconciling Pool CR",
		"namespace", req.Namespace,
		"name", req.Name,
		"airflowPoolName", pool.Spec.AirflowPoolName,
		"scaleSignal", sig,
		"resourceVersion", pool.ResourceVersion,
		"generation", pool.Generation,
	)

	if r.Airflow == nil {
		return ctrl.Result{}, fmt.Errorf("airflow client is nil")
	}

	ast, err := r.Airflow.GetPool(pool.Spec.AirflowPoolName)
	if err != nil {
		lg.Error(err, "airflow get pool")
		poolPatchConditions(&pool, metav1.ConditionFalse, reasonAirflowError, err.Error())
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}

	now := metav1.Now()
	pool.Status.ObservedSlots = &ast.Slots
	pool.Status.LastAirflowSyncTime = &now
	pool.Status.RunningSlots = ast.RunningSlots
	pool.Status.ScheduledSlots = ast.ScheduledSlots
	pool.Status.QueuedSlots = ast.QueuedSlots

	scaleN, field, aerr := activeScaleCounter(sig, ast)
	if aerr != nil {
		msg := aerr.Error()
		lg.Info(msg, "pool", pool.Spec.AirflowPoolName, "scaleSignal", sig)
		poolPatchConditions(&pool, metav1.ConditionFalse, reasonMissingScaleSignal, msg)
		if err := r.Status().Update(ctx, &pool); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	lg.V(1).Info("Airflow pool snapshot",
		"airflowPool", pool.Spec.AirflowPoolName,
		"slots", ast.Slots,
		"runningSlots", ast.RunningSlots,
		"scaleSignal", sig,
		"counterField", field,
		"counterValue", scaleN,
		"scheduledSlots", ast.ScheduledSlots,
		"queuedSlots", ast.QueuedSlots,
	)

	if scaleN == 0 {
		resetSlots := desiredResetSlots(pool.Spec.DefaultSlots, ast.RunningSlots)
		if ast.Slots == resetSlots {
			pool.Status.LastScaleAction = reasonSkippedAlreadyDefault
			poolPatchConditions(&pool, metav1.ConditionTrue, reasonReconciled, fmt.Sprintf("%s=0 and slots already at reset target %d", field, resetSlots))
			return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
		}
		msg := fmt.Sprintf("reset slots from %d to target %d", ast.Slots, resetSlots)
		if ast.RunningSlots != nil && resetSlots > pool.Spec.DefaultSlots {
			msg = fmt.Sprintf("reset safeguard: running_slots=%d > default=%d, setting slots to %d (10%% above running)", *ast.RunningSlots, pool.Spec.DefaultSlots, resetSlots)
		}
		if poolIsWatchMode(pool.Spec.Mode) || r.Airflow.DryRun() {
			reason := reasonDryRunWouldPatch
			if poolIsWatchMode(pool.Spec.Mode) {
				reason = "WatchMode"
			}
			pool.Status.LastScaleAction = reason + ": " + msg
			poolPatchConditions(&pool, metav1.ConditionTrue, reasonReconciled, pool.Status.LastScaleAction)
			return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
		}
		if err := r.Airflow.PatchPoolSlots(pool.Spec.AirflowPoolName, ast, resetSlots); err != nil {
			poolPatchConditions(&pool, metav1.ConditionFalse, reasonAirflowError, err.Error())
			return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
		}
		pool.Status.ObservedSlots = ptrInt32(resetSlots)
		pool.Status.LastScaleAction = reasonPatchedDefault + ": " + msg
		poolPatchConditions(&pool, metav1.ConditionTrue, reasonReconciled, pool.Status.LastScaleAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}

	metricKind, err := toMetricKind(pool.Spec.Workload.Metric)
	if err != nil {
		poolPatchConditions(&pool, metav1.ConditionFalse, reasonWorkloadMetricsError, err.Error())
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}

	usage, err := workloadmetrics.MaxUsageForDeployment(
		ctx, r.Client, r.Metrics,
		pool.Spec.Workload.Namespace,
		pool.Spec.Workload.DeploymentName,
		metricKind,
	)
	if err != nil {
		lg.Error(err, "workload metrics")
		poolPatchConditions(&pool, metav1.ConditionFalse, reasonWorkloadMetricsError, err.Error())
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}

	threshold := float64(pool.Spec.Workload.Threshold)
	lg.V(1).Info("workload metrics vs threshold",
		"deployment", pool.Spec.Workload.DeploymentName,
		"namespace", pool.Spec.Workload.Namespace,
		"metric", pool.Spec.Workload.Metric,
		"usage", usage,
		"threshold", threshold,
	)

	if usage > threshold {
		pool.Status.LastScaleAction = fmt.Sprintf("%s: usage=%.2f threshold=%.2f (%s)",
			reasonSkippedHighUtilization, usage, threshold, pool.Spec.Workload.Metric)
		poolPatchConditions(&pool, metav1.ConditionTrue, reasonReconciled, pool.Status.LastScaleAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}

	minPct := defaultMinRunningPercent
	if pool.Spec.MinRunningPercent != nil {
		minPct = *pool.Spec.MinRunningPercent
	}
	minRunningUtil := float64(minPct) / 100.0

	runningRatio, runningSlots, rerr := runningUtilization(ast)
	if rerr != nil {
		poolPatchConditions(&pool, metav1.ConditionFalse, reasonMissingScaleSignal, rerr.Error())
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}
	lg.V(1).Info("running utilization check",
		"runningSlots", runningSlots,
		"slots", ast.Slots,
		"runningUtilization", runningRatio,
		"requiredMin", minRunningUtil,
	)
	if runningRatio < minRunningUtil {
		pool.Status.LastScaleAction = fmt.Sprintf("%s: running_slots=%d slots=%d ratio=%.4f < %.2f",
			reasonSkippedLowRunningUtil, runningSlots, ast.Slots, runningRatio, minRunningUtil)
		poolPatchConditions(&pool, metav1.ConditionTrue, reasonReconciled, pool.Status.LastScaleAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}

	if pool.Spec.IncreasePercent <= 0 {
		pool.Status.LastScaleAction = reasonSkippedNoIncreaseNeeded + ": increasePercent<=0"
		poolPatchConditions(&pool, metav1.ConditionTrue, reasonReconciled, pool.Status.LastScaleAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}

	factor := 1 + float64(pool.Spec.IncreasePercent)/100.0
	newSlots := int32(math.Ceil(float64(ast.Slots) * factor))
	if pool.Spec.MaxSlots != nil && newSlots > *pool.Spec.MaxSlots {
		newSlots = *pool.Spec.MaxSlots
	}
	if newSlots <= ast.Slots {
		pool.Status.LastScaleAction = fmt.Sprintf("%s: computed newSlots=%d current=%d",
			reasonSkippedNoIncreaseNeeded, newSlots, ast.Slots)
		poolPatchConditions(&pool, metav1.ConditionTrue, reasonReconciled, pool.Status.LastScaleAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}

	msg := fmt.Sprintf("increase slots %d -> %d (%s=%d, %s usage=%.2f<=%.2f)",
		ast.Slots, newSlots, field, scaleN, pool.Spec.Workload.Metric, usage, threshold)
	if poolIsWatchMode(pool.Spec.Mode) || r.Airflow.DryRun() {
		reason := reasonDryRunWouldPatch
		if poolIsWatchMode(pool.Spec.Mode) {
			reason = "WatchMode"
		}
		pool.Status.LastScaleAction = reason + ": " + msg
		poolPatchConditions(&pool, metav1.ConditionTrue, reasonReconciled, pool.Status.LastScaleAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}
	if err := r.Airflow.PatchPoolSlots(pool.Spec.AirflowPoolName, ast, newSlots); err != nil {
		poolPatchConditions(&pool, metav1.ConditionFalse, reasonAirflowError, err.Error())
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
	}
	pool.Status.ObservedSlots = ptrInt32(newSlots)
	pool.Status.LastScaleAction = reasonPatchedIncrease + ": " + msg
	poolPatchConditions(&pool, metav1.ConditionTrue, reasonReconciled, pool.Status.LastScaleAction)
	lg.Info("reconcile complete", "action", pool.Status.LastScaleAction)
	return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &pool)
}

func poolPatchConditions(pool *api.Pool, readyStatus metav1.ConditionStatus, reason, message string) {
	pool.Status.Conditions = []metav1.Condition{{
		Type:               conditionReady,
		Status:             readyStatus,
		ObservedGeneration: pool.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            trimMsg(message, 1024),
	}}
}

func trimMsg(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func ptrInt32(v int32) *int32 { return &v }

func poolIsWatchMode(mode string) bool { return mode == "watch" }

func toMetricKind(m api.WorkloadMetric) (workloadmetrics.MetricKind, error) {
	switch m {
	case api.WorkloadMetricCPU:
		return workloadmetrics.MetricCPU, nil
	case api.WorkloadMetricMemory:
		return workloadmetrics.MetricMemory, nil
	default:
		return "", fmt.Errorf("unsupported workload.metric %q (use CPU or Memory)", m)
	}
}

func activeScaleCounter(sig api.PoolScaleSignal, ast *airflow.PoolState) (n int32, field string, err error) {
	if ast == nil {
		return 0, "", fmt.Errorf("airflow pool state is nil")
	}
	switch sig {
	case "", api.PoolScaleSignalScheduled:
		if ast.ScheduledSlots == nil {
			return 0, "scheduled_slots", fmt.Errorf("Airflow pool response missing scheduled_slots; refusing to change slots")
		}
		return *ast.ScheduledSlots, "scheduled_slots", nil
	case api.PoolScaleSignalQueued:
		if ast.QueuedSlots == nil {
			return 0, "queued_slots", fmt.Errorf("Airflow pool response missing queued_slots; refusing to change slots")
		}
		return *ast.QueuedSlots, "queued_slots", nil
	default:
		return 0, "", fmt.Errorf("invalid spec.scaleSignal %q (use Scheduled or Queued)", sig)
	}
}

func runningUtilization(ast *airflow.PoolState) (ratio float64, running int32, err error) {
	if ast == nil {
		return 0, 0, fmt.Errorf("airflow pool state is nil")
	}
	if ast.Slots <= 0 {
		return 0, 0, fmt.Errorf("airflow pool slots must be > 0")
	}
	if ast.RunningSlots == nil {
		return 0, 0, fmt.Errorf("Airflow pool response missing running_slots; refusing to increase slots")
	}
	running = *ast.RunningSlots
	return float64(running) / float64(ast.Slots), running, nil
}

func desiredResetSlots(defaultSlots int32, runningSlots *int32) int32 {
	target := defaultSlots
	if runningSlots == nil {
		return target
	}
	if *runningSlots > target {
		target = int32(math.Ceil(float64(*runningSlots) * 1.10))
	}
	return target
}

func poolReconcileInterval(seconds int32) time.Duration {
	if seconds < 5 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

// SetupWithManager registers the Pool reconciler.
func (r *PoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	maxC := r.MaxConcurrent
	if maxC < 1 {
		maxC = 1
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.Pool{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxC}).
		Complete(r)
}
