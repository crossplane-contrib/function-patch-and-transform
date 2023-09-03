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
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (*fnv1beta1.RunFunctionResponse, error) {
	f.log.Info("Running Function", "tag", req.GetMeta().GetTag())

	rsp := NewResponseTo(req, 1*time.Minute)

	in := &v1beta1.Resources{}
	if err := GetObject(in, req.GetInput()); err != nil {
		Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
	}

	_, err := GetObservedComposite(req)
	if err != nil {
		Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	// TODO(negz): Can we always patch from observed -> desired?

	return rsp, nil
}
