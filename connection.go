package main

import (
	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
)

// ConnectionDetailsExtractor extracts the connection details of a resource.
type ConnectionDetailsExtractor interface {
	// ExtractConnection of the supplied resource.
	ExtractConnection(cd xpresource.Composed, conn managed.ConnectionDetails, cfg ...v1beta1.ConnectionDetail) (managed.ConnectionDetails, error)
}

// A ConnectionDetailsExtractorFn is a function that satisfies
// ConnectionDetailsExtractor.
type ConnectionDetailsExtractorFn func(cd xpresource.Composed, conn managed.ConnectionDetails, cfg ...v1beta1.ConnectionDetail) (managed.ConnectionDetails, error)

// ExtractConnection of the supplied resource.
func (fn ConnectionDetailsExtractorFn) ExtractConnection(cd xpresource.Composed, conn managed.ConnectionDetails, cfg ...v1beta1.ConnectionDetail) (managed.ConnectionDetails, error) {
	return fn(cd, conn, cfg...)
}

// ExtractConnectionDetails extracts XR connection details from the supplied
// composed resource. If no ExtractConfigs are supplied no connection details
// will be returned.
func ExtractConnectionDetails(cd xpresource.Composed, data managed.ConnectionDetails, cfgs ...v1beta1.ConnectionDetail) (managed.ConnectionDetails, error) {
	out := map[string][]byte{}
	for _, cfg := range cfgs {
		if err := ValidateConnectionDetail(cfg); err != nil {
			return nil, errors.Wrap(err, "invalid")
		}
		switch cfg.Type {
		case v1beta1.ConnectionDetailTypeFromValue:
			out[cfg.Name] = []byte(*cfg.Value)
		case v1beta1.ConnectionDetailTypeFromConnectionSecretKey:
			if data[*cfg.FromConnectionSecretKey] == nil {
				// We don't consider this an error because it's possible the
				// key will still be written at some point in the future.
				continue
			}
			out[cfg.Name] = data[*cfg.FromConnectionSecretKey]
		case v1beta1.ConnectionDetailTypeFromFieldPath:
			// Note we're checking that the error _is_ nil. If we hit an error
			// we silently avoid including this connection secret. It's possible
			// the path will start existing with a valid value in future.
			if b, err := fromFieldPath(cd, *cfg.FromFieldPath); err == nil {
				out[cfg.Name] = b
			}
		}
	}
	return out, nil
}

// fromFieldPath tries to read the value from the supplied field path first as a
// plain string. If this fails, it falls back to reading it as JSON.
func fromFieldPath(from runtime.Object, path string) ([]byte, error) {
	fromMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(from)
	if err != nil {
		return nil, err
	}

	str, err := fieldpath.Pave(fromMap).GetString(path)
	if err == nil {
		return []byte(str), nil
	}

	in, err := fieldpath.Pave(fromMap).GetValue(path)
	if err != nil {
		return nil, err
	}

	return json.Marshal(in)
}

// supportsConnectionDetails determines if the given XR supports native/classic
// connection details.
func supportsConnectionDetails(xr *resource.Composite) bool {
	// v2 modern XRs don't support connection details. They should have a
	// spec.crossplane field, which may be our only indication it's a v2 XR
	_, err := xr.Resource.GetValue("spec.crossplane")
	return err != nil
}

// composeConnectionSecret creates a Secret composed resource containing the
// provided connection details.
func composeConnectionSecret(xr *resource.Composite, details resource.ConnectionDetails, ref *v1beta1.WriteConnectionSecretToRef) (*resource.DesiredComposed, error) {
	if len(details) == 0 {
		return nil, nil
	}

	secret := composed.New()
	secret.SetAPIVersion("v1")
	secret.SetKind("Secret")

	secretRef, err := getConnectionSecretRef(xr, ref)
	if err != nil {
		return nil, errors.Wrap(err, "cannot generate connection secret reference")
	}
	secret.SetName(secretRef.Name)
	secret.SetNamespace(secretRef.Namespace)

	if err := secret.SetValue("data", details); err != nil {
		return nil, errors.Wrap(err, "cannot set connection secret data")
	}

	if err := secret.SetValue("type", xpresource.SecretTypeConnection); err != nil {
		return nil, errors.Wrap(err, "cannot set connection secret type")
	}

	return &resource.DesiredComposed{
		Resource: secret,
		Ready:    resource.ReadyTrue,
	}, nil
}

// getConnectionSecretRef creates a connection secret reference from the given
// XR and input. The patches for the reference will be applied before the
// reference is returned.
func getConnectionSecretRef(xr *resource.Composite, input *v1beta1.WriteConnectionSecretToRef) (xpv1.SecretReference, error) {
	// Get the base connection secret ref to start with
	ref := getBaseConnectionSecretRef(xr, input)

	// Apply patches to the base connection secret ref if they've been provided
	if input != nil && len(input.Patches) > 0 {
		if err := applyConnectionSecretPatches(xr, &ref, input.Patches); err != nil {
			return xpv1.SecretReference{}, errors.Wrap(err, "cannot apply connection secret patches")
		}
	}

	return ref, nil
}

// getBaseConnectionSecretRef determines the base connection secret reference
// without any patches. This reference is generated with the following
// precedence:
//  1. input.writeConnectionSecretToRef - if name or namespace is provided
//     then the whole ref will be used
//  2. xr.writeConnectionSecretToRef - this is no longer automatically added
//     to v2 XR schemas, but the community has been adding it manually, so if
//     it's present we will use it.
//  3. generate the reference from scratch, based on the XR name and namespace
func getBaseConnectionSecretRef(xr *resource.Composite, input *v1beta1.WriteConnectionSecretToRef) xpv1.SecretReference {
	// Use the input values if at least one of name or namespace has been provided
	if input != nil && (input.Name != "" || input.Namespace != "") {
		return xpv1.SecretReference{Name: input.Name, Namespace: input.Namespace}
	}

	// Check if XR author manually added writeConnectionSecretToRef to the XR's
	// schema and just use that if it exists
	xrRef := xr.Resource.GetWriteConnectionSecretToReference()
	if xrRef != nil {
		return *xrRef
	}

	// Nothing has been provided, so generate a default name using the name of the XR
	return xpv1.SecretReference{
		Name:      xr.Resource.GetName() + "-connection",
		Namespace: xr.Resource.GetNamespace(),
	}
}

// applyConnectionSecretPatches applies all patches provided on the input to the
// connection secret reference.
func applyConnectionSecretPatches(xr *resource.Composite, ref *xpv1.SecretReference, patches []v1beta1.ConnectionSecretPatch) error {
	// Convert the secret reference to an unstructured object so we can pass it to the patching logic
	// We use a fake (but reasonable) apiVersion and kind because the unstructured converter requires them.
	refObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "SecretReference",
			"name":       ref.Name,
			"namespace":  ref.Namespace,
		},
	}

	for i, patch := range patches {
		switch patch.GetType() {
		case v1beta1.PatchTypeFromCompositeFieldPath:
			if err := ApplyFromFieldPathPatch(&patch, xr.Resource, refObj); err != nil {
				// we got an error, but if the patch policy is Optional then just skip this patch
				if patch.GetPolicy().GetFromFieldPathPolicy() == v1beta1.FromFieldPathPolicyOptional {
					continue
				}
				return errors.Wrapf(err, "cannot apply patch type %s at index %d", patch.GetType(), i)
			}
		case v1beta1.PatchTypeCombineFromComposite:
			if err := ApplyCombineFromVariablesPatch(&patch, xr.Resource, refObj); err != nil {
				return errors.Wrapf(err, "cannot apply patch type %s at index %d", patch.GetType(), i)
			}
		default:
			return errors.Errorf("unsupported patch type %s at index %d", patch.GetType(), i)
		}
	}

	// Extract the patched values and return them on the reference
	if name, ok := refObj.Object["name"].(string); ok {
		ref.Name = name
	}
	if namespace, ok := refObj.Object["namespace"].(string); ok {
		ref.Namespace = namespace
	}

	return nil
}
