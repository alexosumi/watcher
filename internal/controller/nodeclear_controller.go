package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	api "github.com/alex.osumi/watcher/api/v1"
)

const (
	nodeConditionReady = "Ready"
	ncConditionType    = "Ready"
	ncReasonReconciled = "Reconciled"
	ncReasonError      = "Error"
	ncReasonWatchMode  = "WatchMode"
	ncReasonPatched    = "Patched"
)

// NodeClearReconciler reconciles NodeClear objects.
type NodeClearReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	K8sClient     kubernetes.Interface
	MaxConcurrent int
}

// +kubebuilder:rbac:groups=watcher.io,resources=nodeclears,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=watcher.io,resources=nodeclears/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;delete

func (r *NodeClearReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	var cr api.NodeClear
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	requeueAfter := ncReconcileInterval(cr.Spec.ReconcileIntervalSeconds)
	threshold := time.Duration(cr.Spec.DeleteAfterNotReadyMinutes) * time.Minute

	lg.Info("reconciling NodeClear CR",
		"namespace", req.Namespace,
		"name", req.Name,
		"deleteAfterNotReadyMinutes", cr.Spec.DeleteAfterNotReadyMinutes,
		"mode", cr.Spec.Mode,
		"reconcileInterval", requeueAfter.String(),
	)

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		lg.Error(err, "failed to list nodes")
		ncPatchConditions(&cr, metav1.ConditionFalse, ncReasonError, fmt.Sprintf("list nodes: %s", err.Error()))
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
	}

	var totalDeleted int32
	var monitored int32
	var firstErr error
	now := time.Now()

	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		notReadySince, isNotReady := ncNodeNotReadySince(node)
		if !isNotReady {
			continue
		}

		notReadyDuration := now.Sub(notReadySince)
		monitored++

		if notReadyDuration < threshold {
			lg.V(1).Info("node NotReady but below threshold",
				"node", node.Name,
				"notReadySince", notReadySince.UTC().Format(time.RFC3339),
				"duration", notReadyDuration.String(),
				"threshold", threshold.String(),
			)
			continue
		}

		if ncIsWatchMode(cr.Spec.Mode) {
			lg.Info("watch mode: would delete NotReady node",
				"node", node.Name,
				"notReadySince", notReadySince.UTC().Format(time.RFC3339),
				"duration", notReadyDuration.String(),
			)
			totalDeleted++
			continue
		}

		if err := r.K8sClient.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{}); err != nil {
			lg.Error(err, "failed to delete node", "node", node.Name)
			if firstErr == nil {
				firstErr = fmt.Errorf("delete node %s: %w", node.Name, err)
			}
			continue
		}

		lg.Info("deleted NotReady node",
			"node", node.Name,
			"notReadySince", notReadySince.UTC().Format(time.RFC3339),
			"duration", notReadyDuration.String(),
		)
		totalDeleted++
	}

	nowMeta := metav1.Now()
	cr.Status.LastCleanupTime = &nowMeta
	cr.Status.MonitoredNodes = monitored
	cr.Status.DeletedNodes = totalDeleted

	if firstErr != nil {
		cr.Status.LastAction = fmt.Sprintf("completed with errors: %s", firstErr.Error())
		ncPatchConditions(&cr, metav1.ConditionFalse, ncReasonError, cr.Status.LastAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
	}

	if ncIsWatchMode(cr.Spec.Mode) {
		cr.Status.LastAction = fmt.Sprintf("%s: would delete %d node(s), %d monitored",
			ncReasonWatchMode, totalDeleted, monitored)
		ncPatchConditions(&cr, metav1.ConditionTrue, ncReasonReconciled, cr.Status.LastAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
	}

	cr.Status.LastAction = fmt.Sprintf("%s: deleted %d node(s), %d monitored",
		ncReasonPatched, totalDeleted, monitored)
	ncPatchConditions(&cr, metav1.ConditionTrue, ncReasonReconciled, cr.Status.LastAction)
	lg.Info("reconcile complete", "action", cr.Status.LastAction)
	return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
}

// ncNodeNotReadySince returns the time the node transitioned to NotReady, or false if the node is Ready.
func ncNodeNotReadySince(node *corev1.Node) (time.Time, bool) {
	for _, cond := range node.Status.Conditions {
		if cond.Type == nodeConditionReady && cond.Status == corev1.ConditionFalse {
			return cond.LastTransitionTime.Time, true
		}
	}
	return time.Time{}, false
}

func ncPatchConditions(cr *api.NodeClear, readyStatus metav1.ConditionStatus, reason, message string) {
	cr.Status.Conditions = []metav1.Condition{{
		Type:               ncConditionType,
		Status:             readyStatus,
		ObservedGeneration: cr.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            trimMsg(message, 1024),
	}}
}

func ncIsWatchMode(mode string) bool { return mode == "watch" }

func ncReconcileInterval(seconds int32) time.Duration {
	if seconds < 5 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

// SetupWithManager registers the NodeClear reconciler.
func (r *NodeClearReconciler) SetupWithManager(mgr ctrl.Manager) error {
	maxC := r.MaxConcurrent
	if maxC < 1 {
		maxC = 1
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.NodeClear{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxC}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
