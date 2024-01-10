package main

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"

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

// RenderEnvironmentPatches renders the supplied environment by applying all
// patches that are to the environment, from the supplied XR.
func RenderEnvironmentPatches(env *unstructured.Unstructured, oxr, dxr *composite.Unstructured, ps []v1beta1.EnvironmentPatch) error {
	for i, p := range ps {
		p := p
		switch p.GetType() {
		case v1beta1.PatchTypeFromCompositeFieldPath, v1beta1.PatchTypeCombineFromComposite:
			if err := ApplyToObjects(&p, oxr, env); err != nil {
				return errors.Wrapf(err, errFmtPatch, p.GetType(), i)
			}
		case v1beta1.PatchTypeToCompositeFieldPath, v1beta1.PatchTypeCombineToComposite:
			if err := ApplyToObjects(&p, dxr, env); err != nil {
				return errors.Wrapf(err, errFmtPatch, p.GetType(), i)
			}
		case v1beta1.PatchTypePatchSet, v1beta1.PatchTypeFromEnvironmentFieldPath, v1beta1.PatchTypeCombineFromEnvironment, v1beta1.PatchTypeToEnvironmentFieldPath, v1beta1.PatchTypeCombineToEnvironment:
			// nothing to do
		}
	}
	return nil
}

// RenderComposedPatches renders the supplied composed resource by applying all
// patches that are to or from the supplied composite resource and environment
// in the order they were defined. Properly selecting the right source or
// destination between observed and desired resources.
func RenderComposedPatches( //nolint:gocyclo // just a switch
	ocd *composed.Unstructured,
	dcd *composed.Unstructured,
	oxr *composite.Unstructured,
	dxr *composite.Unstructured,
	env *unstructured.Unstructured,
	ps []v1beta1.ComposedPatch,
) (errs []error, store bool) {
	for i, p := range ps {
		p := p
		switch t := p.GetType(); t {
		case v1beta1.PatchTypeToCompositeFieldPath, v1beta1.PatchTypeCombineToComposite:
			// TODO(negz): Should failures to patch the XR be terminal? It could
			// indicate a required patch failed. A required patch means roughly
			// "this patch has to succeed before you mutate the resource". This
			// is useful to make sure we never create a composed resource in the
			// wrong state. It's less clear how useful it is for the XR, given
			// we'll only ever be updating it, not creating it.

			// We want to patch the XR from observed composed resources, not
			// from desired state. This is because folks will typically be
			// patching from a field that is set once the observed resource is
			// applied such as its status.
			if ocd == nil {
				continue
			}
			if err := ApplyToObjects(&p, dxr, ocd); err != nil {
				errs = append(errs, errors.Wrapf(err, errFmtPatch, t, i))
			}
		case v1beta1.PatchTypeToEnvironmentFieldPath, v1beta1.PatchTypeCombineToEnvironment:
			// TODO(negz): Same as above, but for the Environment. What does it
			// mean for a required patch to the environment to fail? Should it
			// be terminal?

			// Run all patches that are from the (observed) composed resource to
			// the environment.
			if ocd == nil {
				continue
			}
			if err := ApplyToObjects(&p, env, ocd); err != nil {
				errs = append(errs, errors.Wrapf(err, errFmtPatch, t, i))
			}
		// If either of the below renderings return an error, most likely a
		// required FromComposite or FromEnvironment patch failed. A required
		// patch means roughly "this patch has to succeed before you mutate the
		// resource." This is useful to make sure we never create a composed
		// resource in the wrong state. To that end, we don't want to add this
		// resource to our accumulated desired state.
		case v1beta1.PatchTypeFromCompositeFieldPath, v1beta1.PatchTypeCombineFromComposite:
			if err := ApplyToObjects(&p, oxr, dcd); err != nil {
				errs = append(errs, errors.Wrapf(err, errFmtPatch, t, i))
				return errs, false
			}
		case v1beta1.PatchTypeFromEnvironmentFieldPath, v1beta1.PatchTypeCombineFromEnvironment:
			if err := ApplyToObjects(&p, env, dcd); err != nil {
				errs = append(errs, errors.Wrapf(err, errFmtPatch, t, i))
				return errs, false
			}
		case v1beta1.PatchTypePatchSet:
			// Already resolved - nothing to do.
		}
	}
	return errs, true
}
