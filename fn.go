package main

import (
	"context"

	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"

	fncontext "github.com/crossplane/function-sdk-go/context"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
	"github.com/crossplane-contrib/function-patch-and-transform/pt"
)

// Function performs patch-and-transform style Composition.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

var (
	internalEnvironmentGVK = schema.GroupVersionKind{Group: "internal.crossplane.io", Version: "v1alpha1", Kind: "Environment"}
)

// RunFunction runs the Function.
func (f *Function) RunFunction(ctx context.Context, req *fnv1beta1.RunFunctionRequest) (*fnv1beta1.RunFunctionResponse, error) { //nolint:gocyclo // See below.
	// This loop is fairly complex, but more readable with less abstraction.

	log := f.log.WithValues("tag", req.GetMeta().GetTag())
	log.Info("Running Function")

	// TODO(negz): We can probably use a longer TTL if all resources are ready.
	rsp := response.To(req, response.DefaultTTL)

	input := &v1beta1.Resources{}
	if err := request.GetInput(req, input); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get Function input"))
		return rsp, nil
	}

	// Our input is an opaque object nested in a Composition, so unfortunately
	// it won't handle validation for us.
	if err := pt.ValidateResources(input); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "invalid Function input"))
		return rsp, nil
	}

	// The composite resource that actually exists.
	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	log = log.WithValues(
		"xr-version", oxr.Resource.GetAPIVersion(),
		"xr-kind", oxr.Resource.GetKind(),
		"xr-name", oxr.Resource.GetName(),
	)

	// The composite resource desired by previous functions in the pipeline.
	dxr, err := request.GetDesiredCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get desired composite resource"))
		return rsp, nil
	}

	// This is a bit of a hack. The Functions spec tells us we should only
	// return the desired status of the XR. Crossplane doesn't need anything
	// else. It already knows the XR's GVK and name, and thus "re-injects" them
	// into the desired state before applying it. However we need a GVK to be
	// able to use runtime.DefaultUnstructuredConverter internally, which fails
	// if you ask it to unmarshal JSON/YAML without a kind. Technically the
	// Function spec doesn't say anything about APIVersion and Kind, so we can
	// return these without being in violation. ;)
	// https://github.com/crossplane/crossplane/blob/53f71/contributing/specifications/functions.md
	dxr.Resource.SetAPIVersion(oxr.Resource.GetAPIVersion())
	dxr.Resource.SetKind(oxr.Resource.GetKind())

	// The composed resources that actually exist.
	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composed resources from %T", req))
		return rsp, nil
	}

	// The composed resources desired by any previous Functions in the pipeline.
	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composed resources from %T", req))
		return rsp, nil
	}

	cts, err := pt.ComposedTemplates(input.PatchSets, input.Resources)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot resolve PatchSets"))
		return rsp, nil
	}

	// The Composition environment. This could be set by Crossplane, and/or by a
	// previous Function in the pipeline.
	env := &unstructured.Unstructured{}
	if v, ok := request.GetContextKey(req, fncontext.KeyEnvironment); ok {
		if err := resource.AsObject(v.GetStructValue(), env); err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "cannot get Composition environment from %T context key %q", req, fncontext.KeyEnvironment))
			return rsp, nil
		}
		log.Debug("Loaded Composition environment from Function context", "context-key", fncontext.KeyEnvironment)
	}

	// Patching code assumes that the environment has a GVK, as it uses
	// runtime.DefaultUnstructuredConverter.FromUnstructured. This is a bit odd,
	// but it's what we've done in the past. We'll set a default GVK here if one
	// isn't set.
	if env.GroupVersionKind().Empty() {
		env.SetGroupVersionKind(internalEnvironmentGVK)
	}

	if input.Environment != nil {
		// Run all patches that are from the (observed) XR to the environment or
		// from the environment to the (desired) XR.
		for i := range input.Environment.Patches {
			p := &input.Environment.Patches[i]
			if err := pt.ApplyEnvironmentPatch(p, env, oxr.Resource, dxr.Resource); err != nil {

				// Ignore not found errors if patch policy is set to Optional
				if fieldpath.IsNotFound(err) && p.GetPolicy().GetFromFieldPathPolicy() == v1beta1.FromFieldPathPolicyOptional {
					continue
				}

				response.Fatal(rsp, errors.Wrapf(err, "cannot apply the %q environment patch at index %d", p.GetType(), i))
				return rsp, nil
			}
		}
	}

	// Increment this if you emit a warning result.
	warnings := 0

	// Increment this for each resource template with an existing, observed
	// composed resource.
	existing := 0

	for _, t := range cts {
		log := log.WithValues("resource-template-name", t.Name)
		log.Debug("Processing resource template")

		dcd := &resource.DesiredComposed{Resource: composed.New()}

		// If we have a base template, render it into our desired resource. If a
		// previous Function produced a desired resource with this name we'll
		// overwrite it. If we don't have a base template we'll try to patch to
		// and from a desired resource produced by a previous Function in the
		// pipeline.
		switch t.Base {
		case nil:
			cd, ok := desired[resource.Name(t.Name)]
			if !ok {
				response.Fatal(rsp, errors.Errorf("composed resource %q has no base template, and was not produced by a previous Function in the pipeline", t.Name))
				return rsp, nil
			}
			// We want to return this resource unmutated if rendering fails.
			dcd.Resource = cd.Resource.DeepCopy()
		default:
			if err := json.Unmarshal(t.Base.Raw, dcd.Resource); err != nil {
				response.Fatal(rsp, errors.Wrapf(err, "cannot parse base template of composed resource %q", t.Name))
				return rsp, nil
			}
		}

		ocd, exists := observed[resource.Name(t.Name)]
		if exists {
			existing++
			log.Debug("Resource template corresponds to existing composed resource", "metadata-name", ocd.Resource.GetName())

			// If this template corresponds to an existing observed resource we
			// want to keep them associated. We copy only the namespace and
			// name, not the entire observed state, because we're trying to
			// produce only a partial 'overlay' of desired state.
			dcd.Resource.SetNamespace(ocd.Resource.GetNamespace())
			dcd.Resource.SetName(ocd.Resource.GetName())

			conn, err := ExtractConnectionDetails(ocd.Resource, managed.ConnectionDetails(ocd.ConnectionDetails), t.ConnectionDetails...)
			if err != nil {
				response.Warning(rsp, errors.Wrapf(err, "cannot extract composite resource connection details from composed resource %q", t.Name))
				log.Info("Cannot extract composite resource connection details from composed resource", "warning", err)
				warnings++
			}
			for k, v := range conn {
				dxr.ConnectionDetails[k] = v
			}

			ready, err := IsReady(ctx, ocd.Resource, t.ReadinessChecks...)
			if err != nil {
				response.Warning(rsp, errors.Wrapf(err, "cannot check readiness of composed resource %q", t.Name))
				log.Info("Cannot check readiness of composed resource", "warning", err)
				warnings++
			}
			if ready {
				dcd.Ready = resource.ReadyTrue
			}

			log.Debug("Found corresponding observed resource",
				"ready", ready,
				"name", ocd.Resource.GetName())
		}

		// Run all patches that are to a desired composed resource, or from an
		// observed composed resource.
		skip := false
		for i := range t.Patches {
			p := &t.Patches[i]
			if err := pt.ApplyComposedPatch(p, ocd.Resource, dcd.Resource, oxr.Resource, dxr.Resource, env); err != nil {
				if fieldpath.IsNotFound(err) {
					// This is a patch from a required field path that does not
					// exist. The point of FromFieldPathPolicyRequired is to
					// block creation of the new 'to' resource until the 'from'
					// field path exists.
					//
					// The only kind of resource we could be patching to that
					// might not exist at this point is a composed resource. So
					// if we're patching to a composed resource that doesn't
					// exist we want to avoid creating it. Otherwise, we just
					// treat the patch from a required field path the same way
					// we'd treat a patch from an optional field path and skip
					// it.
					if p.GetPolicy().GetFromFieldPathPolicy() == v1beta1.FromFieldPathPolicyRequired {
						if pt.ToComposedResource(p) && !exists {
							response.Warning(rsp, errors.Wrapf(err, "not adding new composed resource %q to desired state because %q patch at index %d has 'policy.fromFieldPath: Required'", t.Name, p.GetType(), i))

							// There's no point processing further patches.
							// They'll either be from an observed composed
							// resource that doesn't exist yet, or to a desired
							// composed resource that we'll discard.
							skip = true
							break
						}
						response.Warning(rsp, errors.Wrapf(err, "cannot render composed resource %q %q patch at index %d: ignoring 'policy.fromFieldPath: Required' because 'to' resource already exists", t.Name, p.GetType(), i))
					}

					// If any optional field path isn't found we just skip this
					// patch and move on. The path may be populated by a
					// subsequent pt.
					continue
				}
				response.Fatal(rsp, errors.Wrapf(err, "cannot render composed resource %q %q patch at index %d", t.Name, p.GetType(), i))
				return rsp, nil
			}
		}

		// Skip adding this resource to the desired state because it doesn't
		// exist yet, and a required FromFieldPath was not (yet) found.
		if skip {
			continue
		}

		desired[resource.Name(t.Name)] = dcd
	}

	if err := response.SetDesiredCompositeResource(rsp, dxr); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composite resource in %T", rsp))
		return rsp, nil
	}

	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources in %T", rsp))
		return rsp, nil
	}

	v, err := resource.AsStruct(env)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot convert Composition environment to protobuf Struct well-known type"))
		return rsp, nil
	}
	response.SetContextKey(rsp, fncontext.KeyEnvironment, structpb.NewStructValue(v))

	log.Info("Successfully processed patch-and-transform resources",
		"resource-templates", len(input.Resources),
		"existing-resources", existing,
		"warnings", warnings)

	return rsp, nil
}
