package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type CanaryRollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CanaryRolloutSpec   `json:"spec,omitempty"`
	Status CanaryRolloutStatus `json:"status,omitempty"`
}

type CanaryRolloutSpec struct {
	ServiceID     string `json:"serviceId,omitempty"`
	TargetVersion string `json:"targetVersion,omitempty"`
	PolicyRef     string `json:"policyRef,omitempty"`
}

type CanaryRolloutStatus struct {
	Phase           string `json:"phase,omitempty"`
	Reason          string `json:"reason,omitempty"`
	LastDecision    string `json:"lastDecision,omitempty"`
	ObservedVersion string `json:"observedVersion,omitempty"`
	LastUpdatedUnix int64  `json:"lastUpdatedUnixMs,omitempty"`
}

type CanaryRolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CanaryRollout `json:"items"`
}

func (in *CanaryRollout) DeepCopyInto(out *CanaryRollout) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

func (in *CanaryRollout) DeepCopy() *CanaryRollout {
	if in == nil {
		return nil
	}
	out := new(CanaryRollout)
	in.DeepCopyInto(out)
	return out
}

func (in *CanaryRollout) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *CanaryRolloutList) DeepCopyInto(out *CanaryRolloutList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]CanaryRollout, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *CanaryRolloutList) DeepCopy() *CanaryRolloutList {
	if in == nil {
		return nil
	}
	out := new(CanaryRolloutList)
	in.DeepCopyInto(out)
	return out
}

func (in *CanaryRolloutList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}
