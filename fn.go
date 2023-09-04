package main

import (
	"context"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/logging"

	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"

	"github.com/negz/function-patch-and-transform/input/v1beta1"
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
	rsp := NewResponseTo(req, 1*time.Minute)

	in := &v1beta1.Resources{}
	if err := GetObject(in, req.GetInput()); err != nil {
		Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
		return rsp, nil
	}

	// The composite resource that actually exists.
	oxr, err := GetObservedCompositeResource(req)
	if err != nil {
		Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	// The composite resource desired by previous functions in the pipeline.
	dxr, err := GetDesiredCompositeResource(req)
	if err != nil {
		Fatal(rsp, errors.Wrap(err, "cannot get desired composite resource"))
		return rsp, nil
	}

	// The composed resources that actually exist.
	observed, err := GetObservedComposedResources(req)
	if err != nil {
		Fatal(rsp, errors.Wrapf(err, "cannot get observed composed resources from %T", req))
		return rsp, nil
	}

	// The composed resources desired by any previous Functions in the pipeline.
	desired, err := GetDesiredComposedResources(req)
	if err != nil {
		Fatal(rsp, errors.Wrapf(err, "cannot get desired composed resources from %T", req))
		return rsp, nil
	}

	cts, err := ComposedTemplates(in.PatchSets, in.Resources)
	if err != nil {
		Fatal(rsp, errors.Wrap(err, "cannot resolve PatchSets"))
		return rsp, nil
	}

	for _, t := range cts {
		dcd := NewDesiredComposedResource()

		ocd, ok := observed[ComposedResourceName(t.Name)]
		if ok {
			conn, err := ExtractConnectionDetails(ocd.Resource, ocd.ConnectionDetails, t.ConnectionDetails...)
			if err != nil {
				Warning(rsp, errors.Wrapf(err, "cannot extract composite resource connection details from composed resource %q", t.Name))
			}
			for k, v := range conn {
				dxr.ConnectionDetails[k] = v
			}

			// TODO(negz): Extend RunFunctionResponse so we can report that this
			// composed resource is now ready.
			_, err = IsReady(ctx, ocd.Resource, t.ReadinessChecks...)
			if err != nil {
				Warning(rsp, errors.Wrapf(err, "cannot check readiness of composed resource %q", t.Name))
			}

			// We want to patch _to_ the XR from observed composed resources,
			// not from desired state that we've accumulated but not yet
			// applied. This is because folks will typically be patching from a
			// field that is set once the observed resource is applied such as
			// its status. Failures to patch the XR are terminal. We don't want
			// to apply the XR if a Required patch did not work, for example.
			if err := RenderToCompositePatches(dxr.Resource, ocd.Resource, t.Patches); err != nil {
				Fatal(rsp, errors.Wrapf(err, "cannot render ToComposite patches for composed resource %q", t.Name))
				return rsp, nil
			}

			// If this template corresponds to an existing observed resource we
			// want to keep them associated. We copy only the namespace and
			// name, not the entire observed state, because we're trying to
			// produce only a partial 'overlay' of desired state.
			dcd.Resource.SetNamespace(ocd.Resource.GetNamespace())
			dcd.Resource.SetName(ocd.Resource.GetName())
		}

		// If we have a base template, render it into our desired resource. If a
		// previous Function produced a desired resource with this name we'll
		// overwrite it. If we don't have a base template we'll try to patch to
		// and from a desired resource produced by a previous Function in the
		// pipeline.
		switch t.Base {
		case nil:
			if err := RenderFromJSON(dcd.Resource, t.Base.Raw); err != nil {
				Fatal(rsp, errors.Wrapf(err, "cannot parse base template of composed resource %q", t.Name))
				return rsp, nil
			}
		default:
			cd, ok := desired[ComposedResourceName(t.Name)]
			if !ok {
				Fatal(rsp, errors.Wrapf(err, "composed resource %q has no base template, and was not produced by a previous Function in the pipeline", t.Name))
				return rsp, nil
			}
			// We want to return this resource unmutated if rendering fails.
			// TODO(negz): Unstructured should have its own DeepCopy methods.
			dcd.Resource.Unstructured = *cd.Resource.GetUnstructured().DeepCopy()
		}

		// If this returns an error, most likely a required FromComposite patch
		// failed. We don't want to add this resource to our accumulated desired
		// state.
		if err := RenderFromCompositePatches(dcd.Resource, oxr.Resource, t.Patches); err != nil {
			Warning(rsp, errors.Wrapf(err, "cannot render FromComposite patches for composed resource %q", t.Name))
			continue
		}

		// Add or replace our desired resource.
		desired[ComposedResourceName(t.Name)] = dcd
	}

	if err := SetDesiredCompositeResource(rsp, dxr); err != nil {
		Fatal(rsp, errors.Wrapf(err, "cannot set desired composite resource in %T", rsp))
		return rsp, nil
	}

	if err := SetDesiredComposedResources(rsp, desired); err != nil {
		Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources in %T", rsp))
		return rsp, nil
	}

	return rsp, nil
}
