package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AirflowClearSpec defines the desired state of AirflowClear.
type AirflowClearSpec struct {
	// DagID is the Airflow DAG ID to target. If empty, all DAGs are scanned.
	// +optional
	DagID string `json:"dagId,omitempty"`

	// OlderThanDays is the age threshold in days. DAG runs older than this number of days will be cleared.
	// +kubebuilder:validation:Minimum=1
	OlderThanDays int32 `json:"olderThanDays"`

	// Mode operation mode: watch (log what would be deleted) or patch (delete old runs).
	// +optional
	// +kubebuilder:default=watch
	// +kubebuilder:validation:Enum=watch;patch
	Mode string `json:"mode,omitempty"`

	// ReconcileIntervalSeconds controls how often this AirflowClear is reconciled.
	// +optional
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=5
	ReconcileIntervalSeconds int32 `json:"reconcileIntervalSeconds,omitempty"`
}

// AirflowClearStatus defines the observed state of AirflowClear.
type AirflowClearStatus struct {
	// DeletedRuns is the count of DAG runs deleted (or that would be deleted in watch mode) in the last reconcile.
	// +optional
	DeletedRuns int32 `json:"deletedRuns,omitempty"`

	// ProcessedDags is the number of DAGs scanned in the last reconcile.
	// +optional
	ProcessedDags int32 `json:"processedDags,omitempty"`

	// LastCleanupTime is the timestamp of the last reconcile that ran cleanup.
	// +optional
	LastCleanupTime *metav1.Time `json:"lastCleanupTime,omitempty"`

	// LastAction is a human-readable description of the last reconcile action.
	// +optional
	LastAction string `json:"lastAction,omitempty"`

	// Conditions represent the latest available observations of the AirflowClear's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=aclear,path=airflowclears

// AirflowClear is the Schema for the airflowclears API.
type AirflowClear struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AirflowClearSpec   `json:"spec,omitempty"`
	Status AirflowClearStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AirflowClearList contains a list of AirflowClear.
type AirflowClearList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AirflowClear `json:"items"`
}

func (in *AirflowClear) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *AirflowClearList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *AirflowClear) DeepCopy() *AirflowClear {
	if in == nil {
		return nil
	}
	out := new(AirflowClear)
	in.DeepCopyInto(out)
	return out
}

func (in *AirflowClear) DeepCopyInto(out *AirflowClear) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

func (in *AirflowClearList) DeepCopy() *AirflowClearList {
	if in == nil {
		return nil
	}
	out := new(AirflowClearList)
	in.DeepCopyInto(out)
	return out
}

func (in *AirflowClearList) DeepCopyInto(out *AirflowClearList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AirflowClear, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *AirflowClearSpec) DeepCopy() *AirflowClearSpec {
	if in == nil {
		return nil
	}
	out := new(AirflowClearSpec)
	*out = *in
	return out
}

func (in *AirflowClearStatus) DeepCopyInto(out *AirflowClearStatus) {
	*out = *in
	if in.LastCleanupTime != nil {
		in, out := &in.LastCleanupTime, &out.LastCleanupTime
		*out = (*in).DeepCopy()
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *AirflowClearStatus) DeepCopy() *AirflowClearStatus {
	if in == nil {
		return nil
	}
	out := new(AirflowClearStatus)
	in.DeepCopyInto(out)
	return out
}

func init() {
	SchemeBuilder.Register(&AirflowClear{}, &AirflowClearList{})
}
