package main

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"

	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
)

// Function performs patch-and-transform style Composition.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

// RunFunction runs the Function.
func (f *Function) RunFunction(ctx context.Context, req *fnv1beta1.RunFunctionRequest) (*fnv1beta1.RunFunctionResponse, error) { //nolint:gocyclo // See below.
	// This loop is fairly complex, but more readable with less abstraction.

	f.log.Info("Running Function", "tag", req.GetMeta().GetTag())

	// TODO(negz): We can probably use a longer TTL if all resources are ready.
	rsp := response.To(req, response.DefaultTTL)

	input := &v1beta1.Resources{}
	if err := request.GetInput(req, input); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get Function input"))
		return rsp, nil
	}

	// Our input is an opaque object nested in a Composition, so unfortunately
	// it won't handle validation for us.
	if err := ValidateResources(input); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "invalid Function input"))
		return rsp, nil
	}

	// The composite resource that actually exists.
	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	// The composite resource desired by previous functions in the pipeline.
	dxr, err := request.GetDesiredCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get desired composite resource"))
		return rsp, nil
	}

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

	cts, err := ComposedTemplates(input.PatchSets, input.Resources)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot resolve PatchSets"))
		return rsp, nil
	}

	for _, t := range cts {
		dcd := resource.NewDesiredComposedResource()

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
			// TODO(negz): Unstructured should have its own DeepCopy methods.
			dcd.Resource.Unstructured = *cd.Resource.GetUnstructured().DeepCopy()
		default:
			if err := RenderFromJSON(dcd.Resource, t.Base.Raw); err != nil {
				response.Fatal(rsp, errors.Wrapf(err, "cannot parse base template of composed resource %q", t.Name))
				return rsp, nil
			}
		}

		ocd, ok := observed[resource.Name(t.Name)]
		if ok {
			// If this template corresponds to an existing observed resource we
			// want to keep them associated. We copy only the namespace and
			// name, not the entire observed state, because we're trying to
			// produce only a partial 'overlay' of desired state.
			dcd.Resource.SetNamespace(ocd.Resource.GetNamespace())
			dcd.Resource.SetName(ocd.Resource.GetName())

			conn, err := ExtractConnectionDetails(ocd.Resource, managed.ConnectionDetails(ocd.ConnectionDetails), t.ConnectionDetails...)
			if err != nil {
				response.Warning(rsp, errors.Wrapf(err, "cannot extract composite resource connection details from composed resource %q", t.Name))
			}
			for k, v := range conn {
				dxr.ConnectionDetails[k] = v
			}

			// TODO(negz): Extend RunFunctionResponse so we can report that this
			// composed resource is now ready.
			ready, err := IsReady(ctx, ocd.Resource, t.ReadinessChecks...)
			if err != nil {
				response.Warning(rsp, errors.Wrapf(err, "cannot check readiness of composed resource %q", t.Name))
			}
			if ready {
				dcd.Ready = resource.ReadyTrue
			}

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
			if err := RenderToCompositePatches(dxr.Resource, ocd.Resource, t.Patches); err != nil {
				response.Warning(rsp, errors.Wrapf(err, "cannot render ToComposite patches for composed resource %q", t.Name))
			}
		}

		// If this returns an error, most likely a required FromComposite patch
		// failed. A required patch means roughly "this patch has to succeed
		// before you mutate the resource." This is useful to make sure we never
		// create a composed resource in the wrong state. To that end, we don't
		// want to add this resource to our accumulated desired state.
		if err := RenderFromCompositePatches(dcd.Resource, oxr.Resource, t.Patches); err != nil {
			response.Warning(rsp, errors.Wrapf(err, "cannot render FromComposite patches for composed resource %q", t.Name))
			continue
		}

		// Add or replace our desired resource.
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

	return rsp, nil
}
