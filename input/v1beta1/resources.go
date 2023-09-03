// Package v1beta1 contains the input type for the P&T Composition Function.
// +kubebuilder:object:generate=true
// +groupName=pt.fn.crossplane.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// Resources specifies Patch & Transform resource templates.
// +kubebuilder:resource:categories=crossplane
type Resources struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// TODO(negz): Support EnvironmentConfigs once RunFunctionRequest does.

	// PatchSets define a named set of patches that may be included by any
	// resource. PatchSets cannot themselves refer to other PatchSets.
	// +optionl
	PatchSets []PatchSet `json:"patchSets,omitempty"`

	// Resources is a list of resource templates that will be used when a
	// composite resourceis created.
	// +optional
	Resources []ComposedTemplate `json:"resources,omitempty"`
}
