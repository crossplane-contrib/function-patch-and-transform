package main

import (
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

// Error strings
const (
	errUnmarshalJSON = "cannot unmarshal JSON data"

	errFmtKindOrGroupChanged = "cannot change the kind or group of a composed resource from %s to %s (possible composed resource template mismatch)"
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

	// This resource already had a GK (probably because it already exists), but
	// when we rendered its template it changed. This shouldn't happen. Either
	// someone changed the kind or group in the template, or we're trying to use the
	// wrong template (e.g. because the order of an array of anonymous templates
	// changed).
	// Please note, we don't check for version changes, as versions can change. For example,
	// if a composed resource was created with a template that has a version of "v1alpha1",
	// and then the template is updated to "v1beta1", the composed resource will still be valid.
	if !gvk.Empty() && o.GetObjectKind().GroupVersionKind().GroupKind() != gvk.GroupKind() {
		return errors.Errorf(errFmtKindOrGroupChanged, gvk, o.GetObjectKind().GroupVersionKind())
	}

	return nil
}
