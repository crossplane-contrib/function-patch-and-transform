package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// TypeReference is used to refer to a type for declaring compatibility.
type TypeReference struct {
	// APIVersion of the type.
	APIVersion string `json:"apiVersion"`

	// Kind of the type.
	Kind string `json:"kind"`
}

// TypeReferenceTo returns a reference to the supplied GroupVersionKind
func TypeReferenceTo(gvk schema.GroupVersionKind) TypeReference {
	return TypeReference{APIVersion: gvk.GroupVersion().String(), Kind: gvk.Kind}
}

// A PatchSet is a set of patches that can be reused from all resources.
type PatchSet struct {
	// Name of this PatchSet.
	Name string `json:"name"`

	// Patches will be applied as an overlay to the base resource.
	Patches []Patch `json:"patches"`
}

// ComposedTemplate is used to provide information about how the composed
// resource should be processed.
type ComposedTemplate struct {
	// A Name uniquely identifies this entry within its resources array.
	Name string `json:"name"`

	// Base of the composed resource that patches will be applied to and from.
	// If base is omitted, a previous Function within the pipeline must have
	// produced the named composed resource. Patches will be applied to and from
	// that resource. If base is specified, and a previous Function within the
	// pipeline produced the name composed resource, it will be overwritten.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:EmbeddedResource
	// +optional
	Base *runtime.RawExtension `json:"base,omitempty"`

	// Patches to and from the composed resource.
	// +optional
	Patches []Patch `json:"patches,omitempty"`

	// ConnectionDetails lists the propagation secret keys from this composed
	// resource to the composition instance connection secret.
	// +optional
	ConnectionDetails []ConnectionDetail `json:"connectionDetails,omitempty"`

	// ReadinessChecks allows users to define custom readiness checks. All
	// checks have to return true in order for resource to be considered ready.
	// The default readiness check is to have the "Ready" condition to be
	// "True".
	// +optional
	// +kubebuilder:default={{type:"MatchCondition",matchCondition:{type:"Ready",status:"True"}}}
	ReadinessChecks []ReadinessCheck `json:"readinessChecks,omitempty"`
}

// ReadinessCheckType is used for readiness check types.
type ReadinessCheckType string

// The possible values for readiness check type.
const (
	ReadinessCheckTypeNonEmpty       ReadinessCheckType = "NonEmpty"
	ReadinessCheckTypeMatchString    ReadinessCheckType = "MatchString"
	ReadinessCheckTypeMatchInteger   ReadinessCheckType = "MatchInteger"
	ReadinessCheckTypeMatchTrue      ReadinessCheckType = "MatchTrue"
	ReadinessCheckTypeMatchFalse     ReadinessCheckType = "MatchFalse"
	ReadinessCheckTypeMatchCondition ReadinessCheckType = "MatchCondition"
	ReadinessCheckTypeNone           ReadinessCheckType = "None"
)

// IsValid returns true if the readiness check type is valid.
func (t *ReadinessCheckType) IsValid() bool {
	switch *t {
	case ReadinessCheckTypeNonEmpty,
		ReadinessCheckTypeMatchString,
		ReadinessCheckTypeMatchInteger,
		ReadinessCheckTypeMatchTrue,
		ReadinessCheckTypeMatchFalse,
		ReadinessCheckTypeMatchCondition,
		ReadinessCheckTypeNone:
		return true
	}
	return false
}

// ReadinessCheck is used to indicate how to tell whether a resource is ready
// for consumption
type ReadinessCheck struct {
	// Type indicates the type of probe you'd like to use.
	// +kubebuilder:validation:Enum="MatchString";"MatchInteger";"NonEmpty";"MatchCondition";"MatchTrue";"MatchFalse";"None"
	Type ReadinessCheckType `json:"type"`

	// FieldPath shows the path of the field whose value will be used.
	// +optional
	FieldPath *string `json:"fieldPath,omitempty"`

	// MatchString is the value you'd like to match if you're using "MatchString" type.
	// +optional
	MatchString *string `json:"matchString,omitempty"`

	// MatchInt is the value you'd like to match if you're using "MatchInt" type.
	// +optional
	MatchInteger *int64 `json:"matchInteger,omitempty"`

	// MatchCondition specifies the condition you'd like to match if you're using "MatchCondition" type.
	// +optional
	MatchCondition *MatchConditionReadinessCheck `json:"matchCondition,omitempty"`
}

// MatchConditionReadinessCheck is used to indicate how to tell whether a resource is ready
// for consumption
type MatchConditionReadinessCheck struct {
	// Type indicates the type of condition you'd like to use.
	// +kubebuilder:default="Ready"
	Type xpv1.ConditionType `json:"type"`

	// Status is the status of the condition you'd like to match.
	// +kubebuilder:default="True"
	Status corev1.ConditionStatus `json:"status"`
}

// A ConnectionDetailType is a type of connection detail.
type ConnectionDetailType string

// ConnectionDetailType types.
const (
	ConnectionDetailTypeFromConnectionSecretKey ConnectionDetailType = "FromConnectionSecretKey"
	ConnectionDetailTypeFromFieldPath           ConnectionDetailType = "FromFieldPath"
	ConnectionDetailTypeFromValue               ConnectionDetailType = "FromValue"
)

// IsValid returns true if the connection detail type is valid.
func (t *ConnectionDetailType) IsValid() bool {
	switch *t {
	case ConnectionDetailTypeFromConnectionSecretKey,
		ConnectionDetailTypeFromFieldPath,
		ConnectionDetailTypeFromValue:
		return true
	}
	return false
}

// ConnectionDetail includes the information about the propagation of the connection
// information from one secret to another.
type ConnectionDetail struct {
	// Name of the connection secret key that will be propagated to the
	// connection secret of the composed resource.
	Name string `json:"name"`

	// Type sets the connection detail fetching behavior to be used. Each
	// connection detail type may require its own fields to be set on the
	// ConnectionDetail object.
	// +kubebuilder:validation:Enum=FromConnectionSecretKey;FromFieldPath;FromValue
	Type ConnectionDetailType `json:"type"`

	// FromConnectionSecretKey is the key that will be used to fetch the value
	// from the composed resource's connection secret.
	// +optional
	FromConnectionSecretKey *string `json:"fromConnectionSecretKey,omitempty"`

	// FromFieldPath is the path of the field on the composed resource whose
	// value to be used as input. Name must be specified if the type is
	// FromFieldPath.
	// +optional
	FromFieldPath *string `json:"fromFieldPath,omitempty"`

	// Value that will be propagated to the connection secret of the composite
	// resource. May be set to inject a fixed, non-sensitive connection secret
	// value, for example a well-known port.
	// +optional
	Value *string `json:"value,omitempty"`
}
