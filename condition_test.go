package main

import (
	"testing"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var xr = &resource.Composite{
	//Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
}

func TestEvaluateCondition(t *testing.T) {
	expressionTrue := "resource.spec.env == \"dev\" "
	//expressionFalse := "resource.spec.env = \"prod\""

	type args struct {
		cs  v1beta1.ConditionSpec
		req *fnv1beta1.RunFunctionRequest
	}
	type want struct {
		ret bool
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"InitlalTest": {
			args: args{
				cs:  v1beta1.ConditionSpec{Expression: expressionTrue},
				req: &fnv1beta1.RunFunctionRequest{},
			},
			want: want{
				ret: false,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ret, err := EvaluateCondition(tc.args.cs, tc.args.req)

			if diff := cmp.Diff(tc.want.ret, ret); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want ret, +got ret:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}
