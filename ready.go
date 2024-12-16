package main

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
	"github.com/crossplane-contrib/function-patch-and-transform/pt"
)

// Error strings
const (
	errInvalidCheck = "invalid"
	errPaveObject   = "cannot lookup field paths in supplied object"

	errFmtRunCheck = "cannot run readiness check at index %d"
)

// A ReadinessChecker checks whether a composed resource is ready or not.
type ReadinessChecker interface {
	IsReady(ctx context.Context, o ConditionedObject, rc ...v1beta1.ReadinessCheck) (ready bool, err error)
}

// A ReadinessCheckerFn checks whether a composed resource is ready or not.
type ReadinessCheckerFn func(ctx context.Context, o ConditionedObject, rc ...v1beta1.ReadinessCheck) (ready bool, err error)

// IsReady reports whether a composed resource is ready or not.
func (fn ReadinessCheckerFn) IsReady(ctx context.Context, o ConditionedObject, rc ...v1beta1.ReadinessCheck) (ready bool, err error) {
	return fn(ctx, o, rc...)
}

// A ConditionedObject is a runtime object with conditions.
type ConditionedObject interface {
	resource.Object
	resource.Conditioned
}

// IsReady returns whether the composed resource is ready.
func IsReady(_ context.Context, o ConditionedObject, rc ...v1beta1.ReadinessCheck) (bool, error) {
	// We don't have API server defaulting, so we default here.
	if len(rc) == 0 {
		return resource.IsConditionTrue(o.GetCondition(xpv1.TypeReady)), nil
	}

	for i := range rc {
		ready, err := RunReadinessCheck(rc[i], o)
		if err != nil {
			return false, errors.Wrapf(err, errFmtRunCheck, i)
		}
		if !ready {
			return false, nil
		}
	}
	return true, nil
}

// RunReadinessCheck runs the readiness check against the supplied object.
func RunReadinessCheck(c v1beta1.ReadinessCheck, o ConditionedObject) (bool, error) { //nolint:gocyclo // just a switch
	if err := pt.ValidateReadinessCheck(c); err != nil {
		return false, errors.Wrap(err, errInvalidCheck)
	}

	p, err := fieldpath.PaveObject(o)
	if err != nil {
		return false, errors.Wrap(err, errPaveObject)
	}

	switch c.Type {
	case v1beta1.ReadinessCheckTypeNone:
		return true, nil
	case v1beta1.ReadinessCheckTypeNonEmpty:
		if _, err := p.GetValue(*c.FieldPath); err != nil {
			return false, resource.Ignore(fieldpath.IsNotFound, err)
		}
		return true, nil
	case v1beta1.ReadinessCheckTypeMatchString:
		val, err := p.GetString(*c.FieldPath)
		if err != nil {
			return false, resource.Ignore(fieldpath.IsNotFound, err)
		}
		return val == *c.MatchString, nil
	case v1beta1.ReadinessCheckTypeMatchInteger:
		val, err := p.GetInteger(*c.FieldPath)
		if err != nil {
			return false, resource.Ignore(fieldpath.IsNotFound, err)
		}
		return val == *c.MatchInteger, nil
	case v1beta1.ReadinessCheckTypeMatchCondition:
		val := o.GetCondition(c.MatchCondition.Type)
		return val.Status == c.MatchCondition.Status, nil
	case v1beta1.ReadinessCheckTypeMatchFalse:
		val, err := p.GetBool(*c.FieldPath)
		if err != nil {
			return false, resource.Ignore(fieldpath.IsNotFound, err)
		}
		return val == false, nil //nolint:gosimple // returning '!val' here as suggested hurts readability
	case v1beta1.ReadinessCheckTypeMatchTrue:
		val, err := p.GetBool(*c.FieldPath)
		if err != nil {
			return false, resource.Ignore(fieldpath.IsNotFound, err)
		}
		return val == true, nil //nolint:gosimple // returning 'val' here as suggested hurts readability
	}

	return false, nil
}
