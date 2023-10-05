package main

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
)

// Error strings
const (
	errUnmarshalJSON = "cannot unmarshal JSON data"

	errFmtKindChanged     = "cannot change the kind of a composed resource from %s to %s (possible composed resource template mismatch)"
	errFmtNamePrefixLabel = "cannot find top-level composite resource name label %q in composite resource metadata"

	// TODO(negz): Include more detail such as field paths if they exist.
	// Perhaps require each patch type to have a String() method to help
	// identify it.
	errFmtPatch = "cannot apply the %q patch at index %d"
)

// RenderFromJSON renders the supplied resource from JSON bytes.
func RenderFromJSON(o resource.Object, data []byte) error {
	gvk := o.GetObjectKind().GroupVersionKind()
	name := o.GetName()
	namespace := o.GetNamespace()

	if err := json.Unmarshal(data, o); err != nil {
		return errors.Wrap(err, errUnmarshalJSON)
	}

	// TODO(negz): Should we return an error if the name or namespace change,
	// rather than just silently re-setting it? Presumably these _changing_ is a
	// sign that something has gone wrong, similar to the GVK changing. What
	// about the UID changing?

	// Unmarshalling the template will overwrite any existing fields, so we must
	// restore the existing name, if any.
	o.SetName(name)
	o.SetNamespace(namespace)

	// This resource already had a GVK (probably because it already exists), but
	// when we rendered its template it changed. This shouldn't happen. Either
	// someone changed the kind in the template or we're trying to use the wrong
	// template (e.g. because the order of an array of anonymous templates
	// changed).
	empty := schema.GroupVersionKind{}
	if gvk != empty && o.GetObjectKind().GroupVersionKind() != gvk {
		return errors.Errorf(errFmtKindChanged, gvk, o.GetObjectKind().GroupVersionKind())
	}

	return nil
}

// RenderFromCompositePatches renders the supplied composed resource by applying
// all patches that are _from_ the supplied composite resource.
func RenderFromCompositePatches(cd resource.Composed, xr resource.Composite, p []v1beta1.Patch) error {
	for i := range p {
		if err := Apply(p[i], xr, cd, v1beta1.PatchTypeFromCompositeFieldPath, v1beta1.PatchTypeCombineFromComposite); err != nil {
			return errors.Wrapf(err, errFmtPatch, p[i].GetType(), i)
		}
	}
	return nil
}

// RenderToCompositePatches renders the supplied composite resource by applying
// all patches that are _from_ the supplied composed resource.
func RenderToCompositePatches(xr resource.Composite, cd resource.Composed, p []v1beta1.Patch) error {
	for i := range p {
		if err := Apply(p[i], xr, cd, v1beta1.PatchTypeToCompositeFieldPath, v1beta1.PatchTypeCombineToComposite); err != nil {
			return errors.Wrapf(err, errFmtPatch, p[i].GetType(), i)
		}
	}
	return nil
}

// RenderFromEnvironmentPatches renders the supplied object (an XR or composed
// resource) by applying all patches that are from the supplied environment.
func RenderFromEnvironmentPatches(o runtime.Object, env *unstructured.Unstructured, p []v1beta1.Patch) error {
	for i := range p {
		if err := ApplyToObjects(p[i], env, o, v1beta1.PatchTypeFromEnvironmentFieldPath, v1beta1.PatchTypeCombineFromEnvironment); err != nil {
			return errors.Wrapf(err, errFmtPatch, p[i].Type, i)
		}
	}
	return nil
}

// RenderToEnvironmentPatches renders the supplied environment by applying all
// patches that are to the environment, from the supplied object (an XR or
// composed resource).
func RenderToEnvironmentPatches(env *unstructured.Unstructured, o runtime.Object, p []v1beta1.Patch) error {
	for i := range p {
		if err := ApplyToObjects(p[i], env, o, v1beta1.PatchTypeToEnvironmentFieldPath, v1beta1.PatchTypeCombineToEnvironment); err != nil {
			return errors.Wrapf(err, errFmtPatch, p[i].Type, i)
		}
	}
	return nil
}
