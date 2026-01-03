package v1beta1

// WriteConnectionSecretToRef specifies a name and namespace for a connection secret.
type WriteConnectionSecretToRef struct {
	// Name of the connection secret. If not specified, defaults to {xr-name}-connection.
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace of the connection secret. If not specified for namespaced XRs,
	// Crossplane will default it to the XR's namespace. For cluster-scoped XRs,
	// namespace must be explicitly provided.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Patches define transformations to apply to the connection secret reference.
	// Patches are only applied from the composite resource to the secret reference.
	// Supported patch types: FromCompositeFieldPath, CombineFromComposite.
	// ToFieldPath must be either "name" or "namespace".
	// +optional
	Patches []ConnectionSecretPatch `json:"patches,omitempty"`
}
