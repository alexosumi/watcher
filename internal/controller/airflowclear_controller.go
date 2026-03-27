package controller

import (
	"context"
	"fmt"
	"time"

	api "github.com/alex.osumi/watcher/api/v1"
	"github.com/alex.osumi/watcher/internal/airflow"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	clearConditionReady        = "Ready"
	clearReasonReconciled      = "Reconciled"
	clearReasonAirflowError    = "AirflowError"
	clearReasonWatchMode       = "WatchMode"
	clearReasonPatched         = "Patched"
)

// AirflowClearReconciler clears old Airflow DAG runs from an AirflowClear CR.
type AirflowClearReconciler struct {
	client.Client
	Airflow *airflow.Client
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=watcher.io,resources=airflowclears,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=watcher.io,resources=airflowclears/status,verbs=get;update;patch

func (r *AirflowClearReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	var cr api.AirflowClear
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	requeueAfter := clearReconcileInterval(cr.Spec.ReconcileIntervalSeconds)
	cutoff := time.Now().UTC().AddDate(0, 0, -int(cr.Spec.OlderThanDays))

	lg.Info("reconciling AirflowClear CR",
		"namespace", req.Namespace,
		"name", req.Name,
		"dagId", cr.Spec.DagID,
		"olderThanDays", cr.Spec.OlderThanDays,
		"mode", cr.Spec.Mode,
		"cutoff", cutoff.Format(time.RFC3339),
	)

	if r.Airflow == nil {
		return ctrl.Result{}, fmt.Errorf("airflow client is nil")
	}

	dagIDs, err := r.resolveDagIDs(cr.Spec.DagID)
	if err != nil {
		lg.Info("failed to resolve DAG IDs; check Airflow credentials have list-dags permission",
			"error", err.Error(),
		)
		clearPatchConditions(&cr, metav1.ConditionFalse, clearReasonAirflowError, err.Error())
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
	}

	var totalDeleted int32
	var firstErr error

	for _, dagID := range dagIDs {
		runs, err := r.Airflow.ListDagRuns(dagID, cutoff)
		if err != nil {
			lg.Info("failed to list DAG runs", "dagId", dagID, "error", err.Error())
			if firstErr == nil {
				firstErr = fmt.Errorf("dag %s: %w", dagID, err)
			}
			continue
		}

		lg.V(1).Info("found old DAG runs", "dagId", dagID, "count", len(runs), "cutoff", cutoff.Format(time.RFC3339))

		for _, run := range runs {
			if clearIsWatchMode(cr.Spec.Mode) {
				lg.Info("watch mode: would delete DAG run",
					"dagId", dagID,
					"dagRunId", run.DagRunID,
					"executionDate", run.ExecutionDate,
				)
				totalDeleted++
				continue
			}

			if err := r.Airflow.DeleteDagRun(dagID, run.DagRunID); err != nil {
				lg.Info("failed to delete DAG run", "dagId", dagID, "dagRunId", run.DagRunID, "error", err.Error())
				if firstErr == nil {
					firstErr = fmt.Errorf("delete dag %s run %s: %w", dagID, run.DagRunID, err)
				}
				continue
			}

			lg.Info("deleted DAG run",
				"dagId", dagID,
				"dagRunId", run.DagRunID,
				"executionDate", run.ExecutionDate,
			)
			totalDeleted++
		}
	}

	now := metav1.Now()
	cr.Status.LastCleanupTime = &now
	cr.Status.ProcessedDags = int32(len(dagIDs))
	cr.Status.DeletedRuns = totalDeleted

	if firstErr != nil {
		cr.Status.LastAction = fmt.Sprintf("completed with errors: %s", firstErr.Error())
		clearPatchConditions(&cr, metav1.ConditionFalse, clearReasonAirflowError, cr.Status.LastAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
	}

	if clearIsWatchMode(cr.Spec.Mode) || r.Airflow.DryRun() {
		reason := clearReasonWatchMode
		if r.Airflow.DryRun() {
			reason = "DryRun"
		}
		cr.Status.LastAction = fmt.Sprintf("%s: would delete %d run(s) across %d dag(s) older than %d days",
			reason, totalDeleted, len(dagIDs), cr.Spec.OlderThanDays)
		clearPatchConditions(&cr, metav1.ConditionTrue, clearReasonReconciled, cr.Status.LastAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
	}

	cr.Status.LastAction = fmt.Sprintf("%s: deleted %d run(s) across %d dag(s) older than %d days",
		clearReasonPatched, totalDeleted, len(dagIDs), cr.Spec.OlderThanDays)
	clearPatchConditions(&cr, metav1.ConditionTrue, clearReasonReconciled, cr.Status.LastAction)
	lg.Info("reconcile complete", "action", cr.Status.LastAction)
	return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
}

// resolveDagIDs returns a slice containing just dagID if set, or all DAG IDs from Airflow.
func (r *AirflowClearReconciler) resolveDagIDs(dagID string) ([]string, error) {
	if dagID != "" {
		return []string{dagID}, nil
	}
	dags, err := r.Airflow.ListDags()
	if err != nil {
		return nil, fmt.Errorf("list dags: %w", err)
	}
	ids := make([]string, 0, len(dags))
	for _, d := range dags {
		ids = append(ids, d.DagID)
	}
	return ids, nil
}

func clearPatchConditions(cr *api.AirflowClear, readyStatus metav1.ConditionStatus, reason, message string) {
	cr.Status.Conditions = []metav1.Condition{{
		Type:               clearConditionReady,
		Status:             readyStatus,
		ObservedGeneration: cr.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            trimMsg(message, 1024),
	}}
}

func clearIsWatchMode(mode string) bool { return mode == "watch" }

func clearReconcileInterval(seconds int32) time.Duration {
	if seconds < 5 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// SetupWithManager registers the AirflowClear reconciler.
func (r *AirflowClearReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.AirflowClear{}).
		Complete(r)
}
