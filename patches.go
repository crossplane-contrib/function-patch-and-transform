package main

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
)

const (
	errPatchSetType             = "a patch in a PatchSet cannot be of type PatchSet"
	errCombineRequiresVariables = "combine patch types require at least one variable"

	errFmtUndefinedPatchSet           = "cannot find PatchSet by name %s"
	errFmtInvalidPatchType            = "patch type %s is unsupported"
	errFmtCombineStrategyNotSupported = "combine strategy %s is not supported"
	errFmtCombineConfigMissing        = "given combine strategy %s requires configuration"
	errFmtCombineStrategyFailed       = "%s strategy could not combine"
	errFmtExpandingArrayFieldPaths    = "cannot expand ToFieldPath %s"
)

// Apply executes a patching operation between the from and to resources.
// Applies all patch types unless an 'only' filter is supplied.
func Apply(p v1beta1.Patch, xr resource.Composite, cd resource.Composed, only ...v1beta1.PatchType) error {
	return ApplyToObjects(p, xr, cd, only...)
}

// ApplyToObjects works like Apply but accepts any kind of runtime.Object. It
// might be vulnerable to conversion panics (see
// https://github.com/crossplane/crossplane/pull/3394 for details).
func ApplyToObjects(p v1beta1.Patch, a, b runtime.Object, only ...v1beta1.PatchType) error {
	if filterPatch(p, only...) {
		return nil
	}

	switch p.GetType() {
	case v1beta1.PatchTypeFromCompositeFieldPath, v1beta1.PatchTypeFromEnvironmentFieldPath:
		return ApplyFromFieldPathPatch(p, a, b)
	case v1beta1.PatchTypeToCompositeFieldPath, v1beta1.PatchTypeToEnvironmentFieldPath:
		return ApplyFromFieldPathPatch(p, b, a)
	case v1beta1.PatchTypeCombineFromComposite, v1beta1.PatchTypeCombineFromEnvironment:
		return ApplyCombineFromVariablesPatch(p, a, b)
	case v1beta1.PatchTypeCombineToComposite, v1beta1.PatchTypeCombineToEnvironment:
		return ApplyCombineFromVariablesPatch(p, b, a)
	case v1beta1.PatchTypePatchSet:
		// Already resolved - nothing to do.
	}
	return errors.Errorf(errFmtInvalidPatchType, p.Type)
}

// filterPatch returns true if patch should be filtered (not applied)
func filterPatch(p v1beta1.Patch, only ...v1beta1.PatchType) bool {
	// filter does not apply if not set
	if len(only) == 0 {
		return false
	}

	for _, patchType := range only {
		if patchType == p.GetType() {
			return false
		}
	}
	return true
}

// ResolveTransforms applies a list of transforms to a patch value.
func ResolveTransforms(c v1beta1.Patch, input any) (any, error) {
	var err error
	for i, t := range c.Transforms {
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
func ApplyFromFieldPathPatch(p v1beta1.Patch, from, to runtime.Object) error {
	if p.FromFieldPath == nil {
		return errors.Errorf(errFmtRequiredField, "FromFieldPath", p.Type)
	}

	// Default to patching the same field on the composed resource.
	if p.ToFieldPath == nil {
		p.ToFieldPath = p.FromFieldPath
	}

	fromMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(from)
	if err != nil {
		return err
	}

	in, err := fieldpath.Pave(fromMap).GetValue(*p.FromFieldPath)
	if IsOptionalFieldPathNotFound(err, p.Policy) {
		return nil
	}
	if err != nil {
		return err
	}

	// Apply transform pipeline
	out, err := ResolveTransforms(p, in)
	if err != nil {
		return err
	}

	// Patch all expanded fields if the ToFieldPath contains wildcards
	if strings.Contains(*p.ToFieldPath, "[*]") {
		return patchFieldValueToMultiple(*p.ToFieldPath, out, to)
	}

	return errors.Wrap(patchFieldValueToObject(*p.ToFieldPath, out, to), "cannot patch to object")
}

// ApplyCombineFromVariablesPatch patches the "to" resource, taking a list of
// input variables and combining them into a single output value.
// The single output value may then be further transformed if they are defined
// on the patch.
func ApplyCombineFromVariablesPatch(p v1beta1.Patch, from, to runtime.Object) error {
	// Combine patch requires configuration
	if p.Combine == nil {
		return errors.Errorf(errFmtRequiredField, "Combine", p.Type)
	}
	// Destination field path is required since we can't default to multiple
	// fields.
	if p.ToFieldPath == nil {
		return errors.Errorf(errFmtRequiredField, "ToFieldPath", p.Type)
	}

	vl := len(p.Combine.Variables)

	if vl < 1 {
		return errors.New(errCombineRequiresVariables)
	}

	fromMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(from)
	if err != nil {
		return err
	}

	in := make([]any, vl)

	// Get value of each variable
	// NOTE: This currently assumes all variables define a 'fromFieldPath'
	// value. If we add new variable types, this may not be the case and
	// this code may be better served split out into a dedicated function.
	for i, sp := range p.Combine.Variables {
		iv, err := fieldpath.Pave(fromMap).GetValue(sp.FromFieldPath)

		// If any source field is not found, we will not
		// apply the patch. This is to avoid situations
		// where a combine patch is expecting a fixed
		// number of inputs (e.g. a string format
		// expecting 3 fields '%s-%s-%s' but only
		// receiving 2 values).
		if IsOptionalFieldPathNotFound(err, p.Policy) {
			return nil
		}
		if err != nil {
			return err
		}
		in[i] = iv
	}

	// Combine input values
	cb, err := Combine(*p.Combine, in)
	if err != nil {
		return err
	}

	// Apply transform pipeline
	out, err := ResolveTransforms(p, cb)
	if err != nil {
		return err
	}

	return errors.Wrap(patchFieldValueToObject(*p.ToFieldPath, out, to), "cannot patch to object")
}

// IsOptionalFieldPathNotFound returns true if the supplied error indicates a
// field path was not found, and the supplied policy indicates a patch from that
// field path was optional.
func IsOptionalFieldPathNotFound(err error, p *v1beta1.PatchPolicy) bool {
	switch {
	case p == nil:
		fallthrough
	case p.FromFieldPath == nil:
		fallthrough
	case *p.FromFieldPath == v1beta1.FromFieldPathPolicyOptional:
		return fieldpath.IsNotFound(err)
	default:
		return false
	}
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
	pn := make(map[string][]v1beta1.Patch)
	for _, s := range pss {
		for _, p := range s.Patches {
			if p.Type == v1beta1.PatchTypePatchSet {
				return nil, errors.New(errPatchSetType)
			}
		}
		pn[s.Name] = s.Patches
	}

	ct := make([]v1beta1.ComposedTemplate, len(cts))
	for i, r := range cts {
		var po []v1beta1.Patch
		for _, p := range r.Patches {
			if p.Type != v1beta1.PatchTypePatchSet {
				po = append(po, p)
				continue
			}
			if p.PatchSetName == nil {
				return nil, errors.Errorf(errFmtRequiredField, "PatchSetName", p.Type)
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
func patchFieldValueToObject(fieldPath string, value any, to runtime.Object) error {
	paved, err := fieldpath.PaveObject(to)
	if err != nil {
		return err
	}

	if err := paved.SetValue(fieldPath, value); err != nil {
		return err
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(paved.UnstructuredContent(), to)
}

// patchFieldValueToMultiple, given a path with wildcards in an array index,
// expands the arrays paths in the "to" object and patches the value into each
// of the resulting fields, returning any errors as they occur.
func patchFieldValueToMultiple(fieldPath string, value any, to runtime.Object) error {
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
		if err := paved.SetValue(field, value); err != nil {
			return err
		}
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(paved.UnstructuredContent(), to)
}
