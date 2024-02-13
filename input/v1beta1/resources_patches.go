package v1beta1

// A PatchType is a type of patch.
type PatchType string

// Patch types.
const (
	PatchTypeFromCompositeFieldPath PatchType = "FromCompositeFieldPath" // Default
	PatchTypePatchSet               PatchType = "PatchSet"
	PatchTypeToCompositeFieldPath   PatchType = "ToCompositeFieldPath"
	PatchTypeCombineFromComposite   PatchType = "CombineFromComposite"
	PatchTypeCombineToComposite     PatchType = "CombineToComposite"
)

// Environment patch types.
//
// COMPOSITION ENVIRONMENT IS AN ALPHA FEATURE.
// These patch types may be changed or removed without notice.
const (
	PatchTypeFromEnvironmentFieldPath PatchType = "FromEnvironmentFieldPath"
	PatchTypeToEnvironmentFieldPath   PatchType = "ToEnvironmentFieldPath"
	PatchTypeCombineFromEnvironment   PatchType = "CombineFromEnvironment"
	PatchTypeCombineToEnvironment     PatchType = "CombineToEnvironment"
)

// A FromFieldPathPolicy determines how to patch from a field path.
type FromFieldPathPolicy string

// FromFieldPath patch policies.
const (
	FromFieldPathPolicyOptional FromFieldPathPolicy = "Optional"
	FromFieldPathPolicyRequired FromFieldPathPolicy = "Required"
)

// A ToFieldPathPolicy determines how to patch to a field path.
type ToFieldPathPolicy string

// ToFieldPath patch policies.
const (
	ToFieldPathPolicyReplace     ToFieldPathPolicy = "Replace"
	ToFieldPathPolicyMergeObject ToFieldPathPolicy = "MergeObject"
	ToFieldPathPolicyAppendArray ToFieldPathPolicy = "AppendArray"
)

// A PatchPolicy configures the specifics of patching behaviour.
type PatchPolicy struct {
	// FromFieldPath specifies how to patch from a field path. The default is
	// 'Optional', which means the patch will be a no-op if the specified
	// fromFieldPath does not exist. Use 'Required' to prevent the creation of a
	// new composed resource until the required path exists.
	// +kubebuilder:validation:Enum=Optional;Required
	// +optional
	FromFieldPath *FromFieldPathPolicy `json:"fromFieldPath,omitempty"`

	// ToFieldPath specifies how to patch to a field path. The default is
	// 'Replace', which means the patch will completely replace the target field,
	// or create it if it does not exist. Use 'MergeObject' to merge the patch
	// object with the target object, or 'AppendArray' to append the patch array
	// to the target array.
	// +kubebuilder:validation:Enum=Replace;MergeObject;AppendArray
	// +optional
	ToFieldPath *ToFieldPathPolicy `json:"toFieldPath,omitempty"`
}

// GetFromFieldPathPolicy returns the FromFieldPathPolicy for this PatchPolicy, defaulting to FromFieldPathPolicyOptional if not specified.
func (pp *PatchPolicy) GetFromFieldPathPolicy() FromFieldPathPolicy {
	if pp == nil || pp.FromFieldPath == nil {
		return FromFieldPathPolicyOptional
	}
	return *pp.FromFieldPath
}

// GetToFieldPathPolicy returns the ToFieldPathPolicy for this PatchPolicy, defaulting to ToFieldPathPolicyReplace if not specified.
func (pp *PatchPolicy) GetToFieldPathPolicy() ToFieldPathPolicy {
	if pp == nil || pp.ToFieldPath == nil {
		return ToFieldPathPolicyReplace
	}
	return *pp.ToFieldPath
}

// Environment represents the Composition environment.
type Environment struct {
	// Patches is a list of environment patches that are executed before a
	// composition's resources are composed. These patches are between the XR
	// and the Environment. Either from the Environment to the XR, or vice
	// versa.
	Patches []EnvironmentPatch `json:"patches,omitempty"`
}

// EnvironmentPatch objects are applied between the composite resource and
// the environment. Their behaviour depends on the Type selected. The default
// Type, FromCompositeFieldPath, copies a value from the composite resource
// to the environment, applying any defined transformers.
type EnvironmentPatch struct {
	// Type sets the patching behaviour to be used. Each patch type may require
	// its own fields to be set on the Patch object.
	// +optional
	// +kubebuilder:validation:Enum=FromCompositeFieldPath;ToCompositeFieldPath;CombineFromComposite;CombineToComposite
	// +kubebuilder:default=FromCompositeFieldPath
	Type PatchType `json:"type,omitempty"`

	Patch `json:",inline"`
}

// GetType returns the patch type. If the type is not set, it returns the default type.
func (ep *EnvironmentPatch) GetType() PatchType {
	if ep.Type == "" {
		return PatchTypeFromCompositeFieldPath
	}
	return ep.Type
}

// ComposedPatch objects are applied between composite and composed resources.
// Their behaviour depends on the Type selected. The default Type,
// FromCompositeFieldPath, copies a value from the composite resource to the
// composed resource, applying any defined transformers.
type ComposedPatch struct {
	// Type sets the patching behaviour to be used. Each patch type may require
	// its own fields to be set on the ComposedPatch object.
	// +optional
	// +kubebuilder:validation:Enum=FromCompositeFieldPath;PatchSet;ToCompositeFieldPath;CombineFromComposite;CombineToComposite;FromEnvironmentFieldPath;ToEnvironmentFieldPath;CombineFromEnvironment;CombineToEnvironment
	// +kubebuilder:default=FromCompositeFieldPath
	Type PatchType `json:"type,omitempty"`

	// PatchSetName to include patches from. Required when type is PatchSet.
	// +optional
	PatchSetName *string `json:"patchSetName,omitempty"`

	Patch `json:",inline"`
}

// GetType returns the patch type. If the type is not set, it returns the default type.
func (p *ComposedPatch) GetType() PatchType {
	if p.Type == "" {
		return PatchTypeFromCompositeFieldPath
	}
	return p.Type
}

// GetPatchSetName returns the PatchSetName for this ComposedPatch, or an empty
// string if it is nil.
func (p *ComposedPatch) GetPatchSetName() string {
	if p.PatchSetName == nil {
		return ""
	}
	return *p.PatchSetName
}

// PatchSetPatch defines a set of Patches that can be referenced by name by
// other patches of type PatchSet.
type PatchSetPatch struct {
	// Type sets the patching behaviour to be used. Each patch type may require
	// its own fields to be set on the ComposedPatch object.
	// +optional
	// +kubebuilder:validation:Enum=FromCompositeFieldPath;ToCompositeFieldPath;CombineFromComposite;CombineToComposite;FromEnvironmentFieldPath;ToEnvironmentFieldPath;CombineFromEnvironment;CombineToEnvironment
	// +kubebuilder:default=FromCompositeFieldPath
	Type PatchType `json:"type,omitempty"`

	Patch `json:",inline"`
}

// GetType returns the patch type. If the type is not set, it returns the default type.
func (psp *PatchSetPatch) GetType() PatchType {
	if psp.Type == "" {
		return PatchTypeFromCompositeFieldPath
	}
	return psp.Type
}

// Patch defines a patch between a source and destination.
type Patch struct {
	// FromFieldPath is the path of the field on the resource whose value is
	// to be used as input. Required when type is FromCompositeFieldPath or
	// ToCompositeFieldPath.
	// +optional
	FromFieldPath *string `json:"fromFieldPath,omitempty"`

	// Combine is the patch configuration for a CombineFromComposite,
	// CombineToComposite patch.
	// +optional
	Combine *Combine `json:"combine,omitempty"`

	// ToFieldPath is the path of the field on the resource whose value will
	// be changed with the result of transforms. Leave empty if you'd like to
	// propagate to the same path as fromFieldPath.
	// +optional
	ToFieldPath *string `json:"toFieldPath,omitempty"`

	// Transforms are the list of functions that are used as a FIFO pipe for the
	// input to be transformed.
	// +optional
	Transforms []Transform `json:"transforms,omitempty"`

	// Policy configures the specifics of patching behaviour.
	// +optional
	Policy *PatchPolicy `json:"policy,omitempty"`
}

// GetFromFieldPath returns the FromFieldPath for this Patch, or an empty string if it is nil.
func (p *Patch) GetFromFieldPath() string {
	if p.FromFieldPath == nil {
		return ""
	}
	return *p.FromFieldPath
}

// GetToFieldPath returns the ToFieldPath for this Patch, or an empty string if it is nil.
func (p *Patch) GetToFieldPath() string {
	if p.ToFieldPath == nil {
		// Default to patching the same field on the composed resource.
		return p.GetFromFieldPath()
	}
	return *p.ToFieldPath
}

// GetCombine returns the Combine for this ComposedPatch, or nil if it is nil.
func (p *Patch) GetCombine() *Combine {
	return p.Combine
}

// GetTransforms returns the Transforms for this ComposedPatch, or nil if it is nil.
func (p *Patch) GetTransforms() []Transform {
	return p.Transforms
}

// GetPolicy returns the PatchPolicy for this ComposedPatch, or nil if it is nil.
func (p *Patch) GetPolicy() *PatchPolicy {
	return p.Policy
}

// A CombineVariable defines the source of a value that is combined with
// others to form and patch an output value. Currently, this only supports
// retrieving values from a field path.
type CombineVariable struct {
	// FromFieldPath is the path of the field on the source whose value is
	// to be used as input.
	FromFieldPath string `json:"fromFieldPath"`
}

// A CombineStrategy determines what strategy will be applied to combine
// variables.
type CombineStrategy string

// CombineStrategy strategy definitions.
const (
	CombineStrategyString CombineStrategy = "string"
)

// A Combine configures a patch that combines more than
// one input field into a single output field.
type Combine struct {
	// Variables are the list of variables whose values will be retrieved and
	// combined.
	// +kubebuilder:validation:MinItems=1
	Variables []CombineVariable `json:"variables"`

	// Strategy defines the strategy to use to combine the input variable values.
	// Currently only string is supported.
	// +kubebuilder:validation:Enum=string
	Strategy CombineStrategy `json:"strategy"`

	// String declares that input variables should be combined into a single
	// string, using the relevant settings for formatting purposes.
	// +optional
	String *StringCombine `json:"string,omitempty"`
}

// A StringCombine combines multiple input values into a single string.
type StringCombine struct {
	// Format the input using a Go format string. See
	// https://golang.org/pkg/fmt/ for details.
	Format string `json:"fmt"`
}
