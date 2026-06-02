/*
Copyright 2026.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// MACAnnotation is the node annotation holding the primary interface MAC address.
	// Used by the node-interface controller to bind the failover IP to the right interface.
	MACAnnotation = "foip.noshoes.xyz/primary-mac"
	// ServerIDAnnotation is the node annotation holding the netcup server ID (integer).
	// Used by the foip controller to route the failover IP via the SCP REST API.
	ServerIDAnnotation = "foip.noshoes.xyz/server-id"
)

type FailoverIpSpec struct {
	// ip is the failover IP address to manage (bare IP, no prefix length).
	// +kubebuilder:validation:Required
	IP string `json:"ip"`

	// secretName is the name of the Secret containing netcup SCP credentials.
	// The Secret must have keys refreshToken (OAuth2 offline token) and userId (netcup user ID).
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`
}

type FailoverIpStatus struct {
	// desiredNode is the node the IP should be routed to.
	DesiredNode string `json:"desiredNode,omitempty"`
	// assignedNode is the node the IP was last successfully routed to via the netcup API.
	AssignedNode string `json:"assignedNode,omitempty"`
	// lastSyncAttempt is the RFC3339 timestamp of the last netcup API call attempt.
	LastSyncAttempt string `json:"lastSyncAttempt,omitempty"`
	// lastSyncSuccess is the RFC3339 timestamp of the last successful netcup API call.
	LastSyncSuccess string `json:"lastSyncSuccess,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=foip;foips

// FailoverIp is the Schema for the failoverips API.
type FailoverIp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FailoverIpSpec   `json:"spec"`
	Status            FailoverIpStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FailoverIpList contains a list of FailoverIp.
type FailoverIpList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FailoverIp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FailoverIp{}, &FailoverIpList{})
}
