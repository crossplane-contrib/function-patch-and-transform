package pt

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"

	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
)

const (
	errPatchSetType = "a patch in a PatchSet cannot be of type PatchSet"

	errFmtUndefinedPatchSet           = "cannot find PatchSet by name %s"
	errFmtCombineStrategyNotSupported = "combine strategy %s is not supported"
	errFmtCombineConfigMissing        = "given combine strategy %s requires configuration"
	errFmtCombineStrategyFailed       = "%s strategy could not combine"
	errFmtExpandingArrayFieldPaths    = "cannot expand ToFieldPath %s"
	errFmtInvalidPatchPolicy          = "invalid patch policy %s"
)

// A PatchInterface is a patch that can be applied between resources.
type PatchInterface interface {
	GetType() v1beta1.PatchType
	GetFromFieldPath() string
	GetToFieldPath() string
	GetCombine() *v1beta1.Combine
	GetTransforms() []v1beta1.Transform
	GetPolicy() *v1beta1.PatchPolicy
}

// PatchWithPatchSetName is a PatchInterface that has a PatchSetName field.
type PatchWithPatchSetName interface {
	PatchInterface
	GetPatchSetName() string
}

// ResolveTransforms applies a list of transforms to a patch value.
func ResolveTransforms(ts []v1beta1.Transform, input any) (any, error) {
	var err error
	for i, t := range ts {
		if input, err = Resolve(t, input); err != nil {
			// TODO(negz): Including the type might help find the offending transform faster.
			return nil, errors.Wrapf(err, errFmtTransformAtIndex, i)
		}
	}
	return input, nil
}

// ApplyFromFieldPathPatch patches the "to" resource, using a source field
// on the "from" resource. Values may be transformed if any are defined on
// the patch.
func ApplyFromFieldPathPatch(p PatchInterface, from, to runtime.Object) error {
	fromMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(from)
	if err != nil {
		return err
	}

	in, err := fieldpath.Pave(fromMap).GetValue(p.GetFromFieldPath())
	if err != nil {
		return err
	}

	// Apply transform pipeline
	out, err := ResolveTransforms(p.GetTransforms(), in)
	if err != nil {
		return err
	}

	// Round-trip the "from" source field value through Kubernetes JSON decoder,
	// so that the json integers are unmarshalled as int64 consistent with "to"/dest value handling.
	// Kubernetes JSON decoder will get us a map[string]any where number values are int64,
	// but protojson and structpb will get us one where number values are float64.
	// https://pkg.go.dev/sigs.k8s.io/json#UnmarshalCaseSensitivePreserveInts
	v, err := toValidJSON(out)
	if err != nil {
		return err
	}

	mo, err := toMergeOption(p)
	if err != nil {
		return err
	}

	// ComposedPatch all expanded fields if the ToFieldPath contains wildcards
	if strings.Contains(p.GetToFieldPath(), "[*]") {
		return patchFieldValueToMultiple(p.GetToFieldPath(), v, to, mo)
	}

	return errors.Wrap(patchFieldValueToObject(p.GetToFieldPath(), v, to, mo), "cannot patch to object")
}

func toValidJSON(value any) (any, error) {
	var v any
	j, err := json.Marshal(value)
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal value to JSON")
	}
	if err := json.Unmarshal(j, &v); err != nil {
		return nil, errors.Wrap(err, "cannot unmarshal value from JSON")
	}
	return v, nil
}

// toMergeOption returns the MergeOptions from the PatchPolicy's ToFieldPathPolicy, if defined.
func toMergeOption(p PatchInterface) (mo *xpv1.MergeOptions, err error) {
	if p == nil {
		return nil, nil
	}
	pp := p.GetPolicy()
	if pp == nil {
		return nil, nil
	}
	switch pp.GetToFieldPathPolicy() {
	case v1beta1.ToFieldPathPolicyReplace:
		// nothing to do, this is the default
	case v1beta1.ToFieldPathPolicyMergeObjects, v1beta1.ToFieldPathPolicyMergeObject: //nolint:staticcheck // MergeObject is deprecated but we must still support it.
		mo = &xpv1.MergeOptions{KeepMapValues: ptr.To(true)}
	case v1beta1.ToFieldPathPolicyMergeObjectsAppendArrays:
		mo = &xpv1.MergeOptions{KeepMapValues: ptr.To(true), AppendSlice: ptr.To(true)}
	case v1beta1.ToFieldPathPolicyForceMergeObjects:
		mo = &xpv1.MergeOptions{KeepMapValues: ptr.To(false)}
	case v1beta1.ToFieldPathPolicyForceMergeObjectsAppendArrays, v1beta1.ToFieldPathPolicyAppendArray: //nolint:staticcheck // AppendArray is deprecated but we must still support it.
		mo = &xpv1.MergeOptions{AppendSlice: ptr.To(true)}
	default:
		// should never happen
		return nil, errors.Errorf(errFmtInvalidPatchPolicy, pp.GetToFieldPathPolicy())
	}
	return mo, nil
}

// ApplyCombineFromVariablesPatch patches the "to" resource, taking a list of
// input variables and combining them into a single output value. The single
// output value may then be further transformed if they are defined on the
// patch.
func ApplyCombineFromVariablesPatch(p PatchInterface, from, to runtime.Object) error {
	fromMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(from)
	if err != nil {
		return err
	}

	c := p.GetCombine()
	in := make([]any, len(c.Variables))

	// Get value of each variable
	// NOTE: This currently assumes all variables define a 'fromFieldPath'
	// value. If we add new variable types, this may not be the case and
	// this code may be better served split out into a dedicated function.
	for i, sp := range c.Variables {
		iv, err := fieldpath.Pave(fromMap).GetValue(sp.FromFieldPath)

		// If any source field is not found, we will not
		// apply the patch. This is to avoid situations
		// where a combine patch is expecting a fixed
		// number of inputs (e.g. a string format
		// expecting 3 fields '%s-%s-%s' but only
		// receiving 2 values).
		if err != nil {
			return err
		}
		in[i] = iv
	}

	// Combine input values
	cb, err := Combine(*c, in)
	if err != nil {
		return err
	}

	// Apply transform pipeline
	out, err := ResolveTransforms(p.GetTransforms(), cb)
	if err != nil {
		return err
	}

	mo, err := toMergeOption(p)
	if err != nil {
		return err
	}

	return errors.Wrap(patchFieldValueToObject(p.GetToFieldPath(), out, to, mo), "cannot patch to object")
}

// ApplyEnvironmentPatch applies a patch to or from the environment. Patches to
// the environment are always from the observed XR. Patches from the environment
// are always to the desired XR.
func ApplyEnvironmentPatch(p *v1beta1.EnvironmentPatch, env *unstructured.Unstructured, oxr, dxr *composite.Unstructured) error {
	switch p.GetType() {
	// From observed XR to environment.
	case v1beta1.PatchTypeFromCompositeFieldPath,
		v1beta1.PatchTypeToEnvironmentFieldPath:
		return ApplyFromFieldPathPatch(p, oxr, env)
	case v1beta1.PatchTypeCombineFromComposite:
		return ApplyCombineFromVariablesPatch(p, oxr, env)

	// From environment to desired XR.
	case v1beta1.PatchTypeToCompositeFieldPath,
		v1beta1.PatchTypeFromEnvironmentFieldPath:
		return ApplyFromFieldPathPatch(p, env, dxr)
	case v1beta1.PatchTypeCombineToComposite:
		return ApplyCombineFromVariablesPatch(p, env, dxr)

	// Invalid patch types in this context.
	case v1beta1.PatchTypeCombineFromEnvironment,
		v1beta1.PatchTypeCombineToEnvironment:
		// Nothing to do.

	case v1beta1.PatchTypePatchSet:
		// Already resolved - nothing to do.
	}
	return nil
}

// ApplyComposedPatch applies a patch to or from a composed resource. Patches
// from an observed composed resource can be to the desired XR, or to the
// environment. Patches to a desired composed resource can be from the observed
// XR, or from the environment.
func ApplyComposedPatch(p *v1beta1.ComposedPatch, ocd, dcd *composed.Unstructured, oxr, dxr *composite.Unstructured, env *unstructured.Unstructured) error { //nolint:gocyclo // Just a long switch.
	// Don't return an error if we're patching from a composed resource that
	// doesn't exist yet. We'll try patch from it once it's been created.
	if ocd == nil && !ToComposedResource(p) {
		return nil
	}

	// We always patch from observed state to desired state. This is because
	// folks will often want to patch from status fields, which only appear in
	// observed state. Observed state should also eventually be consistent with
	// desired state.
	switch t := p.GetType(); t {

	// From observed composed resource to desired XR.
	case v1beta1.PatchTypeToCompositeFieldPath:
		return ApplyFromFieldPathPatch(p, ocd, dxr)
	case v1beta1.PatchTypeCombineToComposite:
		return ApplyCombineFromVariablesPatch(p, ocd, dxr)

	// From observed composed resource to environment.
	case v1beta1.PatchTypeToEnvironmentFieldPath:
		return ApplyFromFieldPathPatch(p, ocd, env)
	case v1beta1.PatchTypeCombineToEnvironment:
		return ApplyCombineFromVariablesPatch(p, ocd, env)

	// From observed XR to desired composed resource.
	case v1beta1.PatchTypeFromCompositeFieldPath:
		return ApplyFromFieldPathPatch(p, oxr, dcd)
	case v1beta1.PatchTypeCombineFromComposite:
		return ApplyCombineFromVariablesPatch(p, oxr, dcd)

	// From environment to desired composed resource.
	case v1beta1.PatchTypeFromEnvironmentFieldPath:
		return ApplyFromFieldPathPatch(p, env, dcd)
	case v1beta1.PatchTypeCombineFromEnvironment:
		return ApplyCombineFromVariablesPatch(p, env, dcd)

	case v1beta1.PatchTypePatchSet:
		// Already resolved - nothing to do.
	}

	return nil
}

// ToComposedResource returns true if the supplied patch is to a composed
// resource, not from it.
func ToComposedResource(p *v1beta1.ComposedPatch) bool {
	switch p.GetType() {

	// From observed XR to desired composed resource.
	case v1beta1.PatchTypeFromCompositeFieldPath, v1beta1.PatchTypeCombineFromComposite:
		return true
	// From environment to desired composed resource.
	case v1beta1.PatchTypeFromEnvironmentFieldPath, v1beta1.PatchTypeCombineFromEnvironment:
		return true

	// From composed resource to composite.
	case v1beta1.PatchTypeToCompositeFieldPath, v1beta1.PatchTypeCombineToComposite:
		return false
	// From composed resource to environment.
	case v1beta1.PatchTypeToEnvironmentFieldPath, v1beta1.PatchTypeCombineToEnvironment:
		return false
	// We can ignore patchsets; they're inlined.
	case v1beta1.PatchTypePatchSet:
		return false
	}

	return false
}

// Combine calls the appropriate combiner.
func Combine(c v1beta1.Combine, vars []any) (any, error) {
	var out any
	var err error

	switch c.Strategy {
	case v1beta1.CombineStrategyString:
		if c.String == nil {
			return nil, errors.Errorf(errFmtCombineConfigMissing, c.Strategy)
		}
		out = CombineString(c.String.Format, vars)
	default:
		return nil, errors.Errorf(errFmtCombineStrategyNotSupported, c.Strategy)
	}

	// Note: There are currently no tests or triggers to exercise this error as
	// our only strategy ("String") uses fmt.Sprintf, which cannot return an error.
	return out, errors.Wrapf(err, errFmtCombineStrategyFailed, string(c.Strategy))
}

// CombineString returns a single output by running a string format with all of
// its input variables.
func CombineString(format string, vars []any) string {
	return fmt.Sprintf(format, vars...)
}

// ComposedTemplates returns the supplied composed resource templates with any
// supplied patchsets dereferenced.
func ComposedTemplates(pss []v1beta1.PatchSet, cts []v1beta1.ComposedTemplate) ([]v1beta1.ComposedTemplate, error) {
	pn := make(map[string][]v1beta1.ComposedPatch)
	for _, s := range pss {
		for _, p := range s.Patches {
			if p.GetType() == v1beta1.PatchTypePatchSet {
				return nil, errors.New(errPatchSetType)
			}
		}
		pn[s.Name] = s.GetComposedPatches()
	}

	ct := make([]v1beta1.ComposedTemplate, len(cts))
	for i, r := range cts {
		var po []v1beta1.ComposedPatch
		for _, p := range r.Patches {
			if p.GetType() != v1beta1.PatchTypePatchSet {
				po = append(po, p)
				continue
			}
			if p.PatchSetName == nil {
				return nil, errors.Errorf(errFmtRequiredField, "PatchSetName", p.GetType())
			}
			ps, ok := pn[*p.PatchSetName]
			if !ok {
				return nil, errors.Errorf(errFmtUndefinedPatchSet, *p.PatchSetName)
			}
			po = append(po, ps...)
		}
		ct[i] = r
		ct[i].Patches = po
	}
	return ct, nil
}

// patchFieldValueToObject applies the value to the "to" object at the given
// path, returning any errors as they occur.
// If no merge options is supplied, then destination field is replaced
// with the given value.
func patchFieldValueToObject(fieldPath string, value any, to runtime.Object, mo *xpv1.MergeOptions) error {
	paved, err := fieldpath.PaveObject(to)
	if err != nil {
		return err
	}

	if err := paved.MergeValue(fieldPath, value, mo); err != nil {
		return err
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(paved.UnstructuredContent(), to)
}

// patchFieldValueToMultiple, given a path with wildcards in an array index,
// expands the arrays paths in the "to" object and patches the value into each
// of the resulting fields, returning any errors as they occur.
func patchFieldValueToMultiple(fieldPath string, value any, to runtime.Object, mo *xpv1.MergeOptions) error {
	paved, err := fieldpath.PaveObject(to)
	if err != nil {
		return err
	}

	arrayFieldPaths, err := paved.ExpandWildcards(fieldPath)
	if err != nil {
		return err
	}

	if len(arrayFieldPaths) == 0 {
		return errors.Errorf(errFmtExpandingArrayFieldPaths, fieldPath)
	}

	for _, field := range arrayFieldPaths {
		if err := paved.MergeValue(field, value, mo); err != nil {
			return err
		}
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(paved.UnstructuredContent(), to)
}
