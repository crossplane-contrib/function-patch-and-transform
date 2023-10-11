package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
)

func TestRunFunction(t *testing.T) {

	type args struct {
		ctx context.Context
		req *fnv1beta1.RunFunctionRequest
	}
	type want struct {
		rsp *fnv1beta1.RunFunctionResponse
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NoInput": {
			reason: "The Function should return a fatal result if no input was specified",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Results: []*fnv1beta1.Result{
						{
							Severity: fnv1beta1.Severity_SEVERITY_FATAL,
							Message:  "invalid Function input: resources: Required value: resources is required",
						},
					},
				},
			},
		},
		"RenderBaseTemplateWithoutPatches": {
			reason: "A simple base template with no patches should be rendered and returned as a desired object.",
			args: args{
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
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD"}`),
							},
						},
					},
				},
			},
		},
		"DesiredResourcesArePassedThrough": {
			reason: "Desired resources from previous Functions in the pipeline and without a corresponding ComposedTemplate are passed through untouched.",
			args: args{
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
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"existing-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"ExistingCD"}`),
							},
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"existing-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"ExistingCD"}`),
							},
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD"}`),
							},
						},
					},
				},
			},
		},
		"PatchBaseTemplate": {
			reason: "A base template with simple patches should be rendered and returned as a desired object.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
								Patches: []v1beta1.Patch{
									{
										Type:          v1beta1.PatchTypeFromCompositeFieldPath,
										FromFieldPath: pointer.String("spec.widgets"),
										ToFieldPath:   pointer.String("spec.watchers"),
										Transforms: []v1beta1.Transform{
											{
												Type: v1beta1.TransformTypeConvert,
												Convert: &v1beta1.ConvertTransform{
													ToType: v1beta1.TransformIOTypeInt64,
												},
											},
											{
												Type: v1beta1.TransformTypeMath,
												Math: &v1beta1.MathTransform{
													Type:     v1beta1.MathTransformTypeMultiply,
													Multiply: pointer.Int64(3),
												},
											},
										},
									},
								},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":30}}`),
							},
						},
					},
				},
			},
		},
		"PatchDesiredResource": {
			reason: "It should be possible to patch & transform a desired resource returned by a previous Function in the pipeline.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								// This template base no base, so we try to
								// patch the resource named "cool-resource" in
								// the desired resources array.
								Name: "cool-resource",
								Patches: []v1beta1.Patch{
									{
										Type:          v1beta1.PatchTypeFromCompositeFieldPath,
										FromFieldPath: pointer.String("spec.widgets"),
										ToFieldPath:   pointer.String("spec.watchers"),
										Transforms: []v1beta1.Transform{
											{
												Type: v1beta1.TransformTypeConvert,
												Convert: &v1beta1.ConvertTransform{
													ToType: v1beta1.TransformIOTypeInt64,
												},
											},
											{
												Type: v1beta1.TransformTypeMath,
												Math: &v1beta1.MathTransform{
													Type:     v1beta1.MathTransformTypeMultiply,
													Multiply: pointer.Int64(3),
												},
											},
										},
									},
								},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD"}`),
							},
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":30}}`),
							},
						},
					},
				},
			},
		},
		"NothingToPatch": {
			reason: "We should return an error if we're trying to patch a desired resource that doesn't exist.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								// This template base no base, so we try to
								// patch the resource named "cool-resource" in
								// the desired resources array.
								Name: "cool-resource",
								Patches: []v1beta1.Patch{
									{
										Type:          v1beta1.PatchTypeFromCompositeFieldPath,
										FromFieldPath: pointer.String("spec.widgets"),
										ToFieldPath:   pointer.String("spec.watchers"),
									},
								},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
					},
					Results: []*fnv1beta1.Result{
						{
							Severity: fnv1beta1.Severity_SEVERITY_FATAL,
							Message:  fmt.Sprintf("composed resource %q has no base template, and was not produced by a previous Function in the pipeline", "cool-resource"),
						},
					},
				},
			},
		},
		"ReplaceDesiredResource": {
			reason: "A simple base template with no patches should be rendered and replace an existing desired object with the same name.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"widgets":9001}}`)},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":42}}`),
							},
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"widgets":9001}}`),
							},
						},
					},
				},
			},
		},
		"FailedPatchNotSaved": {
			reason: "If we fail to patch a desired resource produced by a previous Function in the pipeline we should return a warning result, and leave the original desired resource untouched.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								// This template base no base, so we try to
								// patch the resource named "cool-resource" in
								// the desired resources array.
								Name: "cool-resource",
								Patches: []v1beta1.Patch{
									{
										// This patch should work.
										Type:          v1beta1.PatchTypeFromCompositeFieldPath,
										FromFieldPath: pointer.String("spec.widgets"),
										ToFieldPath:   pointer.String("spec.watchers"),
									},
									{
										// This patch should return an error,
										// because the required path does not
										// exist.
										Type:          v1beta1.PatchTypeFromCompositeFieldPath,
										FromFieldPath: pointer.String("spec.doesNotExist"),
										ToFieldPath:   pointer.String("spec.explode"),
										Policy: &v1beta1.PatchPolicy{
											FromFieldPath: func() *v1beta1.FromFieldPathPolicy {
												r := v1beta1.FromFieldPathPolicyRequired
												return &r
											}(),
										},
									},
								},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":42}}`),
							},
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10"}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								// spec.watchers would be "10" if we didn't
								// discard the patch that worked.
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":42}}`),
							},
						},
					},
					Results: []*fnv1beta1.Result{
						{
							Severity: fnv1beta1.Severity_SEVERITY_WARNING,
							Message:  fmt.Sprintf("cannot render FromComposite patches for composed resource %q: cannot apply the %q patch at index 1: spec.doesNotExist: no such field", "cool-resource", "FromCompositeFieldPath"),
						},
					},
				},
			},
		},
		"ObservedResourceKeepsItsName": {
			reason: "If a template corresponds to an existing observed resource we should keep its name (and namespace).",
			args: args{
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
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","metadata":{"namespace":"default","name":"cool-42"}}`),
							},
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","metadata":{"namespace":"default","name":"cool-42"}}`),
							},
						},
					},
				},
			},
		},
		"ExtractCompositeConnectionDetails": {
			reason: "We should extract any XR connection details specified by a composed template.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
								ConnectionDetails: []v1beta1.ConnectionDetail{
									{
										Type:                    v1beta1.ConnectionDetailTypeFromConnectionSecretKey,
										Name:                    "very",
										FromConnectionSecretKey: pointer.String("very"),
									},
								},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","metadata":{"namespace":"default","name":"cool-42"}}`),
								ConnectionDetails: map[string][]byte{
									"very": []byte("secret"),
								},
							},
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
							ConnectionDetails: map[string][]byte{
								"existing": []byte("supersecretvalue"),
							},
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
							ConnectionDetails: map[string][]byte{
								"existing": []byte("supersecretvalue"),
								"very":     []byte("secret"),
							},
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","metadata":{"namespace":"default","name":"cool-42"}}`),
							},
						},
					},
				},
			},
		},
		"PatchToComposite": {
			reason: "A basic ToCompositeFieldPath patch should work.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
								Patches: []v1beta1.Patch{
									{
										Type:          v1beta1.PatchTypeToCompositeFieldPath,
										FromFieldPath: pointer.String("spec.widgets"),
										ToFieldPath:   pointer.String("spec.watchers"),
										Transforms: []v1beta1.Transform{
											{
												Type: v1beta1.TransformTypeConvert,
												Convert: &v1beta1.ConvertTransform{
													ToType: v1beta1.TransformIOTypeInt64,
												},
											},
											{
												Type: v1beta1.TransformTypeMath,
												Math: &v1beta1.MathTransform{
													Type:     v1beta1.MathTransformTypeMultiply,
													Multiply: pointer.Int64(3),
												},
											},
										},
									},
								},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","metadata":{"namespace":"default","name":"cool-42"},"spec":{"widgets":"10"}}`),
							},
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"watchers":30}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","metadata":{"namespace":"default","name":"cool-42"}}`),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			if diff := cmp.Diff(rsp, tc.want.rsp, protocmp.Transform()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -got rsp, +want rsp:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(err, tc.want.err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -got err, +want err:\n%s", tc.reason, diff)
			}
		})
	}
}
