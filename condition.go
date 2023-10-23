package main

import (
	"reflect"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/google/cel-go/cel"
)

type ConditionalComposite struct {
	*resource.Composite
}

type ConditionalRunFunctionRequest struct {
	*fnv1beta1.RunFunctionRequest
}

// NewCELEnvironment sets up the CEL Environment
func NewCELEnvironment(req *fnv1beta1.RunFunctionRequest) (*cel.Env, error) {
	return cel.NewEnv(
		cel.Types(&fnv1beta1.State{}),
		cel.Variable("observed", cel.ObjectType("apiextensions.fn.proto.v1beta1.State")),
		cel.Variable("desired", cel.ObjectType("apiextensions.fn.proto.v1beta1.State")),
	)
}

// NewCelData formats data in a suitable format for CEL Evaluation
func NewCELData(req *fnv1beta1.RunFunctionRequest) map[string]any {
	xra := make(map[string]any)
	xra["desired"] = req.GetDesired()
	xra["observed"] = req.GetObserved()
	return xra
}

// EvaluateCondition will evaluate a CEL expression
func EvaluateCondition(cs v1beta1.ConditionSpec, req *fnv1beta1.RunFunctionRequest) (bool, error) {
	if cs.Expression == "" {
		return false, nil
	}

	env, err := NewCELEnvironment(req)
	if err != nil {
		return false, errors.Wrap(err, "CEL Env error")
	}

	ast, iss := env.Parse(cs.Expression)
	if iss.Err() != nil {
		return false, errors.Wrap(iss.Err(), "CEL Parse error")
	}

	// Type-check the expression for correctness.
	checked, iss := env.Check(ast)
	// Report semantic errors, if present.
	if iss.Err() != nil {
		return false, errors.Wrap(iss.Err(), "CEL TypeCheck error")
	}

	// Ensure the output type is a bool.
	if !reflect.DeepEqual(checked.OutputType(), cel.BoolType) {
		return false, errors.Errorf(
			"expression must return a boolean, got %v instead",
			checked.OutputType())
	}

	// Plan the program.
	program, err := env.Program(checked)
	if err != nil {
		return false, errors.Wrap(err, "CEL program plan")
	}

	// Convert our Function Request into map[string]any for CEL evaluation
	val := NewCELData(req)

	// Evaluate the program without any additional arguments.
	result, _, err := program.Eval(val)
	if err != nil {
		return false, errors.Wrap(err, "CEL program Evaluation")
	}

	ret, ok := result.Value().(bool)
	if !ok {
		return false, errors.Wrap(err, "CEL program did not return a bool")
	}

	return bool(ret), nil
}
