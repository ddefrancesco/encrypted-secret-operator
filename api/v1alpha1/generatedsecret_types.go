package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type GeneratedSecretSpec struct {

	// tipo di segreto da generare
	Type string `json:"type"`

	// endpoint API generazione
	Endpoint Endpoint `json:"endpoint"`

	// parametri generazione
	Parameters map[string]string `json:"parameters,omitempty"`

	// trigger evento
	Trigger TriggerSpec `json:"trigger"`

	// rotazione
	RotationInterval string `json:"rotationInterval,omitempty"`

	// versioning
	MaxVersions int `json:"maxVersions,omitempty"`
}

type TriggerSpec struct {

	// eventi supportati
	OnCreate bool `json:"onCreate,omitempty"`

	OnRotate bool `json:"onRotate,omitempty"`

	OnSpecChange bool `json:"onSpecChange,omitempty"`

	// opzionale: cron
	Schedule string `json:"schedule,omitempty"`
}

type GeneratedSecretStatus struct {
	LastGeneration metav1.Time `json:"lastGeneration,omitempty"`

	CurrentVersion int `json:"currentVersion,omitempty"`

	ObservedHash string `json:"observedHash,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type GeneratedSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GeneratedSecretSpec   `json:"spec,omitempty"`
	Status GeneratedSecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type GeneratedSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GeneratedSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GeneratedSecret{}, &GeneratedSecretList{})
}
