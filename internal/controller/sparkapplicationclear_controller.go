package controller

import (
	"context"
	"fmt"
	"path"
	"time"

	api "github.com/alex.osumi/watcher/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var sparkAppGVR = schema.GroupVersionResource{
	Group:    "sparkoperator.k8s.io",
	Version:  "v1beta2",
	Resource: "sparkapplications",
}

var defaultSparkStatuses = []string{"FAILED", "COMPLETED"}

const (
	sparkConditionReady     = "Ready"
	sparkReasonReconciled   = "Reconciled"
	sparkReasonError        = "Error"
	sparkReasonWatchMode    = "WatchMode"
	sparkReasonPatched      = "Patched"
)

// SparkApplicationClearReconciler deletes SparkApplication resources matching configured statuses.
type SparkApplicationClearReconciler struct {
	client.Client
	Dynamic       dynamic.Interface
	Scheme        *runtime.Scheme
	MaxConcurrent int
}

// +kubebuilder:rbac:groups=watcher.io,resources=sparkapplicationclears,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=watcher.io,resources=sparkapplicationclears/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sparkoperator.k8s.io,resources=sparkapplications,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list

func (r *SparkApplicationClearReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	var cr api.SparkApplicationClear
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	requeueAfter := sparkReconcileInterval(cr.Spec.ReconcileIntervalSeconds)
	statuses := cr.Spec.Statuses
	if len(statuses) == 0 {
		statuses = defaultSparkStatuses
	}
	statusSet := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		statusSet[s] = true
	}

	lg.Info("reconciling SparkApplicationClear CR",
		"namespace", req.Namespace,
		"name", req.Name,
		"namespacePattern", cr.Spec.Namespace,
		"statuses", statuses,
		"mode", cr.Spec.Mode,
		"deleteAfterTerminationMinutes", cr.Spec.DeleteAfterTerminationMinutes,
	)

	namespaces, err := r.resolveNamespaces(ctx, cr.Spec.Namespace)
	if err != nil {
		lg.Info("failed to resolve namespaces", "error", err.Error())
		sparkPatchConditions(&cr, metav1.ConditionFalse, sparkReasonError, err.Error())
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
	}

	var totalDeleted int32
	var firstErr error

	for _, ns := range namespaces {
		limit := int64(100)
		opts := metav1.ListOptions{Limit: limit}
		for {
			apps, err := r.Dynamic.Resource(sparkAppGVR).Namespace(ns).List(ctx, opts)
			if err != nil {
				lg.Info("failed to list SparkApplications", "namespace", ns, "error", err.Error())
				if firstErr == nil {
					firstErr = fmt.Errorf("namespace %s: %w", ns, err)
				}
				break
			}

			for i := range apps.Items {
				app := &apps.Items[i]
				state := sparkAppState(app)
				if !statusSet[state] {
					continue
				}

				if cr.Spec.DeleteAfterTerminationMinutes != nil {
					termTime, ok := sparkAppTerminationTime(app)
					if !ok {
						lg.V(1).Info("skipping SparkApplication: delay enabled but terminationTime missing or invalid",
							"namespace", ns, "name", app.GetName())
						continue
					}
					deadline := termTime.Add(time.Duration(*cr.Spec.DeleteAfterTerminationMinutes) * time.Minute)
					if time.Now().Before(deadline) {
						lg.V(1).Info("skipping SparkApplication: not yet past delete delay after termination",
							"namespace", ns, "name", app.GetName(),
							"terminationTime", termTime.UTC().Format(time.RFC3339),
							"eligibleAfter", deadline.UTC().Format(time.RFC3339),
						)
						continue
					}
				}

				if sparkIsWatchMode(cr.Spec.Mode) {
					lg.Info("watch mode: would delete SparkApplication",
						"namespace", ns,
						"name", app.GetName(),
						"state", state,
					)
					totalDeleted++
					continue
				}

				if err := r.Dynamic.Resource(sparkAppGVR).Namespace(ns).Delete(ctx, app.GetName(), metav1.DeleteOptions{}); err != nil {
					lg.Info("failed to delete SparkApplication", "namespace", ns, "name", app.GetName(), "error", err.Error())
					if firstErr == nil {
						firstErr = fmt.Errorf("delete %s/%s: %w", ns, app.GetName(), err)
					}
					continue
				}

				lg.Info("deleted SparkApplication",
					"namespace", ns,
					"name", app.GetName(),
					"state", state,
				)
				totalDeleted++
			}

			cont := apps.GetContinue()
			if cont == "" {
				break
			}
			opts.Continue = cont
		}
	}

	now := metav1.Now()
	cr.Status.LastCleanupTime = &now
	cr.Status.ProcessedNamespaces = int32(len(namespaces))
	cr.Status.DeletedApps = totalDeleted

	if firstErr != nil {
		cr.Status.LastAction = fmt.Sprintf("completed with errors: %s", firstErr.Error())
		sparkPatchConditions(&cr, metav1.ConditionFalse, sparkReasonError, cr.Status.LastAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
	}

	if sparkIsWatchMode(cr.Spec.Mode) {
		cr.Status.LastAction = fmt.Sprintf("%s: would delete %d app(s) across %d namespace(s)",
			sparkReasonWatchMode, totalDeleted, len(namespaces))
		sparkPatchConditions(&cr, metav1.ConditionTrue, sparkReasonReconciled, cr.Status.LastAction)
		return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
	}

	cr.Status.LastAction = fmt.Sprintf("%s: deleted %d app(s) across %d namespace(s)",
		sparkReasonPatched, totalDeleted, len(namespaces))
	sparkPatchConditions(&cr, metav1.ConditionTrue, sparkReasonReconciled, cr.Status.LastAction)
	lg.Info("reconcile complete", "action", cr.Status.LastAction)
	return ctrl.Result{RequeueAfter: requeueAfter}, r.Status().Update(ctx, &cr)
}

// resolveNamespaces returns namespaces matching the glob pattern, or all namespaces if pattern is empty.
func (r *SparkApplicationClearReconciler) resolveNamespaces(ctx context.Context, pattern string) ([]string, error) {
	nsList := &unstructured.UnstructuredList{}
	nsList.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "NamespaceList"})
	if err := r.List(ctx, nsList); err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	var matched []string
	for _, ns := range nsList.Items {
		name := ns.GetName()
		if pattern == "" {
			matched = append(matched, name)
			continue
		}
		ok, err := path.Match(pattern, name)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		if ok {
			matched = append(matched, name)
		}
	}
	return matched, nil
}

// sparkAppState extracts .status.applicationState.state from an unstructured SparkApplication.
func sparkAppState(obj *unstructured.Unstructured) string {
	status, ok := obj.Object["status"].(map[string]interface{})
	if !ok {
		return ""
	}
	appState, ok := status["applicationState"].(map[string]interface{})
	if !ok {
		return ""
	}
	state, _ := appState["state"].(string)
	return state
}

// sparkAppTerminationTime extracts and parses status.terminationTime (RFC3339) from an unstructured SparkApplication.
func sparkAppTerminationTime(obj *unstructured.Unstructured) (time.Time, bool) {
	status, ok := obj.Object["status"].(map[string]interface{})
	if !ok {
		return time.Time{}, false
	}
	raw, ok := status["terminationTime"]
	if !ok || raw == nil {
		return time.Time{}, false
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func sparkPatchConditions(cr *api.SparkApplicationClear, readyStatus metav1.ConditionStatus, reason, message string) {
	cr.Status.Conditions = []metav1.Condition{{
		Type:               sparkConditionReady,
		Status:             readyStatus,
		ObservedGeneration: cr.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            trimMsg(message, 1024),
	}}
}

func sparkIsWatchMode(mode string) bool { return mode == "watch" }

func sparkReconcileInterval(seconds int32) time.Duration {
	if seconds < 5 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// SetupWithManager registers the SparkApplicationClear reconciler.
func (r *SparkApplicationClearReconciler) SetupWithManager(mgr ctrl.Manager) error {
	maxC := r.MaxConcurrent
	if maxC < 1 {
		maxC = 1
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.SparkApplicationClear{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxC}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
