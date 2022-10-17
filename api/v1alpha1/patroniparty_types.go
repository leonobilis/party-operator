/*
Copyright 2022.

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

package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// PatroniPartySpec defines the desired state of PatroniParty
type PatroniPartySpec struct {
	//+kubebuilder:validation:Minimum=0
	Participants int `json:"participants"`
	//+kubebuilder:validation:Minimum=0
	UpgradeDelay         int    `json:"upgradeDelay"`
	PatroniContainerName string `json:"patroniContainerName"`
	//+kubebuilder:validation:MinItems=2
	Images                 []string               `json:"images"`
	PatroniStatefulSetSpec appsv1.StatefulSetSpec `json:"patroniStatefulSetSpec"`
	// +optional
	CompanionDeploymentSpec *appsv1.DeploymentSpec `json:"companionDeploymentSpec,omitempty"`
}

// PatroniPartyStatus defines the observed state of PatroniParty
type PatroniPartyStatus struct {
	Participants     int      `json:"participants,omitempty"`
	ParticipantsList []string `json:"participantsList,omitempty"`
	Running          int      `json:"running,omitempty"`
	RunningList      []string `json:"runningList,omitempty"`
	Degraded         int      `json:"degraded,omitempty"`
	DegradedList     []string `json:"degradedList,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Participants",type=integer,JSONPath=`.status.participants`
// +kubebuilder:printcolumn:name="Running",type=integer,JSONPath=`.status.running`
// +kubebuilder:printcolumn:name="Degraded",type=integer,JSONPath=`.status.degraded`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PatroniParty is the Schema for the patroniparties API
type PatroniParty struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PatroniPartySpec   `json:"spec,omitempty"`
	Status PatroniPartyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PatroniPartyList contains a list of PatroniParty
type PatroniPartyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PatroniParty `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PatroniParty{}, &PatroniPartyList{})
}
