package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadMetric identifies which resource to compare against Threshold.
// +kubebuilder:validation:Enum=CPU;Memory
type WorkloadMetric string

const (
	WorkloadMetricCPU    WorkloadMetric = "CPU"
	WorkloadMetricMemory WorkloadMetric = "Memory"
)

// PoolScaleSignal selects which Airflow pool metric drives scale-up / reset.
// +kubebuilder:validation:Enum=Scheduled;Queued
type PoolScaleSignal string

const (
	PoolScaleSignalScheduled PoolScaleSignal = "Scheduled"
	PoolScaleSignalQueued    PoolScaleSignal = "Queued"
)

// WorkloadSpec selects a Deployment and a metrics-server metric threshold.
type WorkloadSpec struct {
	// Namespace of the Deployment.
	Namespace string `json:"namespace"`
	// DeploymentName is the metadata.name of the target Deployment.
	DeploymentName string `json:"deploymentName"`
	// Metric is CPU (millicores) or Memory (Mi).
	Metric WorkloadMetric `json:"metric"`
	// Threshold is in millicores when Metric is CPU, or Mi when Metric is Memory.
	Threshold int64 `json:"threshold"`
}

// PoolSpec defines the desired state of Pool.
type PoolSpec struct {
	// AirflowPoolName is the Airflow pool name (e.g. submit).
	AirflowPoolName string `json:"airflowPoolName"`
	// ReconcileIntervalSeconds controls how often this Pool is reconciled.
	// +optional
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=5
	ReconcileIntervalSeconds int32 `json:"reconcileIntervalSeconds,omitempty"`
	// ScaleSignal selects scheduled_slots vs queued_slots from Airflow. Default is Scheduled.
	// +optional
	// +kubebuilder:default=Scheduled
	ScaleSignal PoolScaleSignal `json:"scaleSignal,omitempty"`
	// DefaultSlots is the pool slot count when the scale signal is zero.
	DefaultSlots int32 `json:"defaultSlots"`
	// IncreasePercent scales current slots by ceil(current * (1 + increasePercent/100)).
	IncreasePercent int32 `json:"increasePercent"`
	// Workload resolves pods from the Deployment and compares metrics-server usage to Threshold.
	Workload WorkloadSpec `json:"workload"`
	// MaxSlots caps the slot value after increase. Omit for no cap.
	// +optional
	MaxSlots *int32 `json:"maxSlots,omitempty"`
	// Mode operation mode: watch (log recommendations) or patch (apply changes)
	// +optional
	// +kubebuilder:default=patch
	// +kubebuilder:validation:Enum=watch;patch
	Mode string `json:"mode,omitempty"`
}

// PoolStatus defines the observed state of Pool.
type PoolStatus struct {
	// ObservedSlots is the last known Airflow pool slots value.
	// +optional
	ObservedSlots *int32 `json:"observedSlots,omitempty"`
	// RunningSlots is the last read running_slots from Airflow (informational).
	// +optional
	RunningSlots *int32 `json:"runningSlots,omitempty"`
	// ScheduledSlots is the last read scheduled_slots from Airflow (informational).
	// +optional
	ScheduledSlots *int32 `json:"scheduledSlots,omitempty"`
	// QueuedSlots is the last read queued_slots from Airflow (informational).
	// +optional
	QueuedSlots *int32 `json:"queuedSlots,omitempty"`
	// LastAirflowSyncTime is RFC3339 timestamp of last successful Airflow API read.
	// +optional
	LastAirflowSyncTime *metav1.Time `json:"lastAirflowSyncTime,omitempty"`
	// Conditions represent the latest available observations of the Pool's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// LastScaleAction is a short human-readable description of the last reconcile action.
	// +optional
	LastScaleAction string `json:"lastScaleAction,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=apool,path=pools

// Pool is the Schema for the pools API.
type Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PoolSpec   `json:"spec,omitempty"`
	Status PoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PoolList contains a list of Pool.
type PoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pool `json:"items"`
}

// DeepCopyObject returns a generically typed copy of an object
func (in *Pool) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyObject returns a generically typed copy of an object
func (in *PoolList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *Pool) DeepCopy() *Pool {
	if in == nil {
		return nil
	}
	out := new(Pool)
	in.DeepCopyInto(out)
	return out
}

func (in *Pool) DeepCopyInto(out *Pool) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *PoolList) DeepCopy() *PoolList {
	if in == nil {
		return nil
	}
	out := new(PoolList)
	in.DeepCopyInto(out)
	return out
}

func (in *PoolList) DeepCopyInto(out *PoolList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Pool, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *PoolSpec) DeepCopyInto(out *PoolSpec) {
	*out = *in
	out.Workload = in.Workload
	if in.MaxSlots != nil {
		in, out := &in.MaxSlots, &out.MaxSlots
		*out = new(int32)
		**out = **in
	}
}

func (in *PoolSpec) DeepCopy() *PoolSpec {
	if in == nil {
		return nil
	}
	out := new(PoolSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *PoolStatus) DeepCopyInto(out *PoolStatus) {
	*out = *in
	if in.ObservedSlots != nil {
		in, out := &in.ObservedSlots, &out.ObservedSlots
		*out = new(int32)
		**out = **in
	}
	if in.RunningSlots != nil {
		in, out := &in.RunningSlots, &out.RunningSlots
		*out = new(int32)
		**out = **in
	}
	if in.ScheduledSlots != nil {
		in, out := &in.ScheduledSlots, &out.ScheduledSlots
		*out = new(int32)
		**out = **in
	}
	if in.QueuedSlots != nil {
		in, out := &in.QueuedSlots, &out.QueuedSlots
		*out = new(int32)
		**out = **in
	}
	if in.LastAirflowSyncTime != nil {
		in, out := &in.LastAirflowSyncTime, &out.LastAirflowSyncTime
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

func (in *PoolStatus) DeepCopy() *PoolStatus {
	if in == nil {
		return nil
	}
	out := new(PoolStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *WorkloadSpec) DeepCopyInto(out *WorkloadSpec) {
	*out = *in
}

func (in *WorkloadSpec) DeepCopy() *WorkloadSpec {
	if in == nil {
		return nil
	}
	out := new(WorkloadSpec)
	in.DeepCopyInto(out)
	return out
}

func init() {
	SchemeBuilder.Register(&Pool{}, &PoolList{})
}
