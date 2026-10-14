package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ResourceValues deliberately uses strings so schema and Go share quantity validation.
type ResourceValues struct {
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="isQuantity(self) ? quantity(self).isGreaterThan(quantity('0')) : false",message="cpu must be a positive Kubernetes quantity"
	CPU string `json:"cpu"`
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="isQuantity(self) ? quantity(self).isGreaterThan(quantity('0')) : false",message="memory must be a positive Kubernetes quantity"
	Memory string `json:"memory"`
}

// +kubebuilder:validation:XValidation:rule="isQuantity(self.requests.cpu) && isQuantity(self.limits.cpu) ? quantity(self.limits.cpu).compareTo(quantity(self.requests.cpu)) >= 0 : false",message="cpu limit must be at least the request"
// +kubebuilder:validation:XValidation:rule="isQuantity(self.requests.memory) && isQuantity(self.limits.memory) ? quantity(self.limits.memory).compareTo(quantity(self.requests.memory)) >= 0 : false",message="memory limit must be at least the request"
type ApplicationResources struct {
	Requests ResourceValues `json:"requests"`
	Limits   ResourceValues `json:"limits"`
}

type ApplicationSpec struct {
	// +kubebuilder:validation:Minimum=1
	Revision int64 `json:"revision"`
	// Supported policy: lowercase Docker repository, optional registry port, tag and/or sha256 digest.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	// +kubebuilder:validation:Pattern=`^([a-z0-9]+([.-][a-z0-9]+)*(:[0-9]{1,5})?/)?[a-z0-9]+(([._]|__|-+)[a-z0-9]+)*(/[a-z0-9]+(([._]|__|-+)[a-z0-9]+)*)*(:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(@sha256:[a-f0-9]{64})?$`
	Image string `json:"image"`
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	Replicas int32 `json:"replicas,omitempty"`
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port      int32                `json:"port,omitempty"`
	Resources ApplicationResources `json:"resources"`
	// +kubebuilder:default="/readyz"
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:Pattern=`^/[A-Za-z0-9._~/-]*$`
	// +kubebuilder:validation:XValidation:rule="!self.startsWith('//')",message="probe must be a local absolute path"
	ReadinessPath string `json:"readinessPath,omitempty"`
	// +kubebuilder:default="/healthz"
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:Pattern=`^/[A-Za-z0-9._~/-]*$`
	// +kubebuilder:validation:XValidation:rule="!self.startsWith('//')",message="probe must be a local absolute path"
	LivenessPath string `json:"livenessPath,omitempty"`
}

type ApplicationStatus struct {
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	ObservedRevision   int64  `json:"observedRevision,omitempty"`
	ReadyReplicas      int32  `json:"readyReplicas,omitempty"`
	AvailableReplicas  int32  `json:"availableReplicas,omitempty"`
	Image              string `json:"image,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=hfapp
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 63 && self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="application name must be a DNS label of at most 63 characters"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Revision",type=integer,JSONPath=`.spec.revision`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ApplicationSpec   `json:"spec"`
	Status            ApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

func init() { SchemeBuilder.Register(&Application{}, &ApplicationList{}) }
