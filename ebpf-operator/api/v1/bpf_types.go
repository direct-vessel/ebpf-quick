/*
Copyright 2023 The Kubernetes authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BPFSpec defines the desired state of BPF
type BPFSpec struct {
	Runner  string  `json:"runner"`
	Program Program `json:"program"`
}

type Program struct {
	// Source for the program value.
	// +optional
	ValueFrom *ProgramSource `json:"valueFrom,omitempty"`
}

type ProgramSource struct {
	// Selects a key of a ConfigMap in the BPF's namespace
	// +optional
	ConfigMapKeyRef *corev1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
}

// BPFStatus defines the observed state of BPF
type BPFStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BPF is the Schema for the bpfs API
type BPF struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BPFSpec   `json:"spec,omitempty"`
	Status BPFStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BPFList contains a list of BPF
type BPFList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BPF `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BPF{}, &BPFList{})
}
