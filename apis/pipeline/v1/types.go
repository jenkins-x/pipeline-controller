package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const BuildResourcePlural = "pipelines"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Pipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              PipelineSpec   `json:"spec"`
	Status            PipelineStatus `json:"status,omitempty"`
}

type PipelineSpec struct {
	Foo string `json:"foo"`
	Bar bool   `json:"bar"`
}

type PipelineStatus struct {
	State   PipelineState `json:"state,omitempty"`
	Message string        `json:"message,omitempty"`
}

type PipelineState string

const (
	BuildStateCreated   PipelineState = "Created"
	BuildStateProcessed PipelineState = "Processed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Pipeline `json:"items"`
}
