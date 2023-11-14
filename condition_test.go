package main

import (
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/google/go-cmp/cmp"
	"github.com/stevendborrelli/function-conditional-patch-and-transform/input/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestEvaluateCondition(t *testing.T) {
	dxr := `{"apiVersion":"nopexample.org/v1alpha1","kind":"XNopResource","metadata":{"name":"test-resource"},"spec":{"env":"dev","render":true}}`
	oxr := `{"apiVersion":"nopexample.org/v1alpha1","kind":"XNopResource","metadata":{"name":"test-resource"},"spec":{"env":"dev","render":true},"status":{"id":"123","ready":false} }`

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
		"CELParseError": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "field = value"},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: false,
				err: errors.New("CEL Parse error"),
			},
		},
		"CELTypeError": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "size(desired.resources)"},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: false,
				err: errors.New("CEL Type error: expression 'size(desired.resources)' must return a boolean, got int instead"),
			},
		},
		"KeyError": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "badkey"},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: false,
				err: errors.New("CEL TypeCheck error: ERROR: <input>:1:1: undeclared reference to 'badkey' (in container '')\n | badkey\n | ^"),
			},
		},
		"TrueDesired": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "desired.composite.resource.spec.env == \"dev\" "},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: true,
				err: nil,
			},
		},
		"TrueDesiredBool": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "desired.composite.resource.spec.render == true"},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: true,
				err: nil,
			},
		},
		"FalseDesiredBool": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "desired.composite.resource.spec.render == false"},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: false,
				err: nil,
			},
		},
		"FalseObservedBool": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "observed.composite.resource.status.ready == true"},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: false,
				err: nil,
			},
		},
		"FalseLengthResources": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "size(desired.resources) == 0"},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Resources: map[string]*fnv1beta1.Resource{
							"test": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","metadata":{"namespace":"default","name":"cool-42"}}`),
							},
						},
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: false,
				err: nil,
			},
		},
		"TrueResourceMapKeyExists": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "\"test-resource\" in desired.resources"},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Resources: map[string]*fnv1beta1.Resource{
							"test-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","metadata":{"namespace":"default","name":"cool-42"}}`),
							},
						},
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: true,
				err: nil,
			},
		},
		"FalseResourceMapKeyExists": {
			args: args{
				cs: v1beta1.ConditionSpec{Expression: "\"bad-resource\" in desired.resources"},
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(oxr),
						},
					},
					Desired: &fnv1beta1.State{
						Resources: map[string]*fnv1beta1.Resource{
							"test-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","metadata":{"namespace":"default","name":"cool-42"}}`),
							},
						},
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(dxr),
						},
					},
				},
			},
			want: want{
				ret: false,
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ret, err := EvaluateCondition(tc.args.cs, tc.args.req)

			if diff := cmp.Diff(tc.want.ret, ret); diff != "" {
				t.Errorf("%s\nEvaluateCondition(...): -want ret, +got ret:\n%s", tc.reason, diff)
			}

			if tc.want.err != nil || err != nil {
				if !strings.HasPrefix(err.Error(), tc.want.err.Error()) {
					t.Errorf("\nEvaluateCondition(...): -want err, +got err:\n-want (error starts with): %s\n-got: %s", tc.want.err.Error(), err)
				}
			}

		})
	}
}
