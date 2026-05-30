package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeClearSpec defines the desired state of NodeClear.
type NodeClearSpec struct {
	// DeleteAfterNotReadyMinutes is the number of minutes a node must be in NotReady
	// condition before it becomes eligible for deletion.
	// +kubebuilder:validation:Minimum=1
	DeleteAfterNotReadyMinutes int32 `json:"deleteAfterNotReadyMinutes"`

	// Mode operation mode: watch (log what would be deleted) or patch (actually delete).
	// +optional
	// +kubebuilder:default=watch
	// +kubebuilder:validation:Enum=watch;patch
	Mode string `json:"mode,omitempty"`

	// ReconcileIntervalSeconds controls how often this NodeClear is reconciled.
	// +optional
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=5
	ReconcileIntervalSeconds int32 `json:"reconcileIntervalSeconds,omitempty"`
}

// NodeClearStatus defines the observed state of NodeClear.
type NodeClearStatus struct {
	// DeletedNodes is the count of nodes deleted (or that would be deleted in watch mode) in the last reconcile.
	// +optional
	DeletedNodes int32 `json:"deletedNodes,omitempty"`

	// MonitoredNodes is the number of nodes currently in NotReady state being tracked.
	// +optional
	MonitoredNodes int32 `json:"monitoredNodes,omitempty"`

	// LastCleanupTime is the timestamp of the last reconcile that ran cleanup.
	// +optional
	LastCleanupTime *metav1.Time `json:"lastCleanupTime,omitempty"`

	// LastAction is a human-readable description of the last reconcile action.
	// +optional
	LastAction string `json:"lastAction,omitempty"`

	// Conditions represent the latest available observations of the NodeClear's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=nclear,path=nodeclears

// NodeClear is the Schema for the nodeclears API.
type NodeClear struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeClearSpec   `json:"spec,omitempty"`
	Status NodeClearStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeClearList contains a list of NodeClear.
type NodeClearList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeClear `json:"items"`
}

func (in *NodeClear) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *NodeClearList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *NodeClear) DeepCopy() *NodeClear {
	if in == nil {
		return nil
	}
	out := new(NodeClear)
	in.DeepCopyInto(out)
	return out
}

func (in *NodeClear) DeepCopyInto(out *NodeClear) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *NodeClearList) DeepCopy() *NodeClearList {
	if in == nil {
		return nil
	}
	out := new(NodeClearList)
	in.DeepCopyInto(out)
	return out
}

func (in *NodeClearList) DeepCopyInto(out *NodeClearList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]NodeClear, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *NodeClearSpec) DeepCopy() *NodeClearSpec {
	if in == nil {
		return nil
	}
	out := new(NodeClearSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *NodeClearSpec) DeepCopyInto(out *NodeClearSpec) {
	*out = *in
}

func (in *NodeClearStatus) DeepCopyInto(out *NodeClearStatus) {
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

func (in *NodeClearStatus) DeepCopy() *NodeClearStatus {
	if in == nil {
		return nil
	}
	out := new(NodeClearStatus)
	in.DeepCopyInto(out)
	return out
}

func init() {
	SchemeBuilder.Register(&NodeClear{}, &NodeClearList{})
}
