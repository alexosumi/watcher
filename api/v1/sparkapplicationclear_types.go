package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SparkApplicationClearSpec defines the desired state of SparkApplicationClear.
type SparkApplicationClearSpec struct {
	// Namespace is a glob pattern for namespaces to scan (e.g. "spark-*").
	// If empty, all namespaces are scanned.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Statuses is the list of SparkApplication states to target for deletion.
	// Valid values include COMPLETED, FAILED, etc. Defaults to ["FAILED","COMPLETED"].
	// +optional
	Statuses []string `json:"statuses,omitempty"`

	// Mode operation mode: watch (log what would be deleted) or patch (actually delete).
	// +optional
	// +kubebuilder:default=watch
	// +kubebuilder:validation:Enum=watch;patch
	Mode string `json:"mode,omitempty"`

	// ReconcileIntervalSeconds controls how often this SparkApplicationClear is reconciled.
	// +optional
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=5
	ReconcileIntervalSeconds int32 `json:"reconcileIntervalSeconds,omitempty"`
}

// SparkApplicationClearStatus defines the observed state of SparkApplicationClear.
type SparkApplicationClearStatus struct {
	// DeletedApps is the count of SparkApplications deleted (or that would be deleted in watch mode) in the last reconcile.
	// +optional
	DeletedApps int32 `json:"deletedApps,omitempty"`

	// ProcessedNamespaces is the number of namespaces scanned in the last reconcile.
	// +optional
	ProcessedNamespaces int32 `json:"processedNamespaces,omitempty"`

	// LastCleanupTime is the timestamp of the last reconcile that ran cleanup.
	// +optional
	LastCleanupTime *metav1.Time `json:"lastCleanupTime,omitempty"`

	// LastAction is a human-readable description of the last reconcile action.
	// +optional
	LastAction string `json:"lastAction,omitempty"`

	// Conditions represent the latest available observations of the SparkApplicationClear's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=sparkclear,path=sparkapplicationclears

// SparkApplicationClear is the Schema for the sparkapplicationclears API.
type SparkApplicationClear struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SparkApplicationClearSpec   `json:"spec,omitempty"`
	Status SparkApplicationClearStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SparkApplicationClearList contains a list of SparkApplicationClear.
type SparkApplicationClearList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SparkApplicationClear `json:"items"`
}

func (in *SparkApplicationClear) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *SparkApplicationClearList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *SparkApplicationClear) DeepCopy() *SparkApplicationClear {
	if in == nil {
		return nil
	}
	out := new(SparkApplicationClear)
	in.DeepCopyInto(out)
	return out
}

func (in *SparkApplicationClear) DeepCopyInto(out *SparkApplicationClear) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *SparkApplicationClearList) DeepCopy() *SparkApplicationClearList {
	if in == nil {
		return nil
	}
	out := new(SparkApplicationClearList)
	in.DeepCopyInto(out)
	return out
}

func (in *SparkApplicationClearList) DeepCopyInto(out *SparkApplicationClearList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]SparkApplicationClear, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *SparkApplicationClearSpec) DeepCopy() *SparkApplicationClearSpec {
	if in == nil {
		return nil
	}
	out := new(SparkApplicationClearSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *SparkApplicationClearSpec) DeepCopyInto(out *SparkApplicationClearSpec) {
	*out = *in
	if in.Statuses != nil {
		in, out := &in.Statuses, &out.Statuses
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *SparkApplicationClearStatus) DeepCopyInto(out *SparkApplicationClearStatus) {
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

func (in *SparkApplicationClearStatus) DeepCopy() *SparkApplicationClearStatus {
	if in == nil {
		return nil
	}
	out := new(SparkApplicationClearStatus)
	in.DeepCopyInto(out)
	return out
}

func init() {
	SchemeBuilder.Register(&SparkApplicationClear{}, &SparkApplicationClearList{})
}
