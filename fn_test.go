package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	fncontext "github.com/crossplane/function-sdk-go/context"
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
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
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
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
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
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
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
								Patches: []v1beta1.ComposedPatch{
									{
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.widgets"),
											ToFieldPath:   ptr.To[string]("spec.watchers"),
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
														Multiply: ptr.To[int64](3),
													},
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
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
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
								Patches: []v1beta1.ComposedPatch{
									{
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.widgets"),
											ToFieldPath:   ptr.To[string]("spec.watchers"),
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
														Multiply: ptr.To[int64](3),
													},
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
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
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
								Patches: []v1beta1.ComposedPatch{
									{
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.widgets"),
											ToFieldPath:   ptr.To[string]("spec.watchers"),
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
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
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
								Patches: []v1beta1.ComposedPatch{
									{
										// This patch should work.
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.widgets"),
											ToFieldPath:   ptr.To[string]("spec.watchers"),
										},
									},
									{
										// This patch should return an error,
										// because the required path does not
										// exist.
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.doesNotExist"),
											ToFieldPath:   ptr.To[string]("spec.explode"),
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
							Message:  fmt.Sprintf("cannot render patches for composed resource %q: cannot apply the %q patch at index 1: spec.doesNotExist: no such field", "cool-resource", "FromCompositeFieldPath"),
						},
					},
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
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
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
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
										FromConnectionSecretKey: ptr.To[string]("very"),
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
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
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
								Patches: []v1beta1.ComposedPatch{
									{
										Type: v1beta1.PatchTypeToCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.widgets"),
											ToFieldPath:   ptr.To[string]("spec.watchers"),
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
														Multiply: ptr.To[int64](3),
													},
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
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
				},
			},
		},
		"PatchToCompositeWithEnvironmentPatches": {
			reason: "A basic ToCompositeFieldPath patch should work with environment.patches.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							}},
						Environment: &v1beta1.Environment{
							Patches: []v1beta1.EnvironmentPatch{
								{
									Type: v1beta1.PatchTypeFromEnvironmentFieldPath,
									Patch: v1beta1.Patch{
										FromFieldPath: ptr.To[string]("data.widgets"),
										ToFieldPath:   ptr.To[string]("spec.watchers"),
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
													Multiply: ptr.To[int64](3),
												}}}}}}}}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{},
					},
					Context: contextWithEnvironment(map[string]interface{}{
						"widgets": "10",
					})},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							// spec.watchers = 10 * 3 = 30
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":30}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD"}`),
							}}},
					Context: contextWithEnvironment(map[string]interface{}{
						"widgets": "10",
					})}}},
		"EnvironmentPatchToEnvironment": {
			reason: "A basic ToEnvironment patch should work with environment.patches.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							}},
						Environment: &v1beta1.Environment{
							Patches: []v1beta1.EnvironmentPatch{
								{
									Type: v1beta1.PatchTypeToEnvironmentFieldPath,
									Patch: v1beta1.Patch{
										FromFieldPath: ptr.To[string]("spec.watchers"),
										ToFieldPath:   ptr.To[string]("data.widgets"),
										Transforms: []v1beta1.Transform{
											{
												Type: v1beta1.TransformTypeMath,
												Math: &v1beta1.MathTransform{
													Type:     v1beta1.MathTransformTypeMultiply,
													Multiply: ptr.To[int64](3),
												},
											},
											{
												Type: v1beta1.TransformTypeConvert,
												Convert: &v1beta1.ConvertTransform{
													ToType: v1beta1.TransformIOTypeString,
												},
											}}}}}}}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":10}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{},
					},
					Context: contextWithEnvironment(nil)},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD"}`),
							}}},
					Context: contextWithEnvironment(map[string]interface{}{
						"widgets": "30",
					})}}},
		"PatchComposedResourceFromEnvironment": {
			reason: "A basic FromEnvironmentPatch should work if defined at spec.resources[*].patches.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{{
							Name: "cool-resource",
							Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							Patches: []v1beta1.ComposedPatch{{
								Type: v1beta1.PatchTypeFromEnvironmentFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("data.widgets"),
									ToFieldPath:   ptr.To[string]("spec.watchers"),
									Transforms: []v1beta1.Transform{{
										Type: v1beta1.TransformTypeConvert,
										Convert: &v1beta1.ConvertTransform{
											ToType: v1beta1.TransformIOTypeInt64,
										},
									}, {
										Type: v1beta1.TransformTypeMath,
										Math: &v1beta1.MathTransform{
											Type:     v1beta1.MathTransformTypeMultiply,
											Multiply: ptr.To[int64](3),
										},
									}}}}},
						}}}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{},
					},
					Context: contextWithEnvironment(map[string]interface{}{
						"widgets": "10",
					})},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								// spec.watchers = 10 * 3 = 30
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":30}}`),
							}}},
					Context: contextWithEnvironment(map[string]interface{}{
						"widgets": "10",
					})}}},

		"PatchComposedResourceFromEnvironmentShadowedNotSet": {
			reason: "A basic FromEnvironmentPatch should work if defined at spec.resources[*].patches, even if a successive patch shadows it and its source is not set.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{{
							Name: "cool-resource",
							Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							Patches: []v1beta1.ComposedPatch{{
								Type: v1beta1.PatchTypeFromEnvironmentFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("data.widgets"),
									ToFieldPath:   ptr.To[string]("spec.watchers"),
									Transforms: []v1beta1.Transform{{
										Type: v1beta1.TransformTypeConvert,
										Convert: &v1beta1.ConvertTransform{
											ToType: v1beta1.TransformIOTypeInt64,
										},
									}, {
										Type: v1beta1.TransformTypeMath,
										Math: &v1beta1.MathTransform{
											Type:     v1beta1.MathTransformTypeMultiply,
											Multiply: ptr.To[int64](3),
										},
									}}}},
								{
									Type: v1beta1.PatchTypeFromCompositeFieldPath,
									Patch: v1beta1.Patch{
										FromFieldPath: ptr.To[string]("spec.watchers"),
										ToFieldPath:   ptr.To[string]("spec.watchers"),
									}}}}}}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{},
					},
					Context: contextWithEnvironment(map[string]interface{}{
						"widgets": "10",
					})},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								// spec.watchers = 10 * 3 = 30
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":30}}`),
							}}},
					Context: contextWithEnvironment(map[string]interface{}{
						"widgets": "10",
					})}}},
		"PatchComposedResourceFromEnvironmentShadowedSet": {
			reason: "A basic FromEnvironmentPatch should work if defined at spec.resources[*].patches, even if a successive patch shadows it and its source is set.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{{
							Name: "cool-resource",
							Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD"}`)},
							Patches: []v1beta1.ComposedPatch{{
								Type: v1beta1.PatchTypeFromEnvironmentFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("data.widgets"),
									ToFieldPath:   ptr.To[string]("spec.watchers"),
									Transforms: []v1beta1.Transform{{
										Type: v1beta1.TransformTypeConvert,
										Convert: &v1beta1.ConvertTransform{
											ToType: v1beta1.TransformIOTypeInt64,
										},
									}, {
										Type: v1beta1.TransformTypeMath,
										Math: &v1beta1.MathTransform{
											Type:     v1beta1.MathTransformTypeMultiply,
											Multiply: ptr.To[int64](3),
										},
									}}}},
								{
									Type: v1beta1.PatchTypeFromCompositeFieldPath,
									Patch: v1beta1.Patch{
										FromFieldPath: ptr.To[string]("spec.watchers"),
										ToFieldPath:   ptr.To[string]("spec.watchers"),
									}}}}}}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							// I want this in the environment, 42
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":42}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{},
					},
					Context: contextWithEnvironment(map[string]interface{}{
						"widgets": "10",
					})},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD"}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"cool-resource": {
								// spec.watchers comes from the composite resource, 42
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":42}}`),
							}}},
					Context: contextWithEnvironment(map[string]interface{}{
						"widgets": "10",
					})}}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			if diff := cmp.Diff(tc.want.rsp, rsp, protocmp.Transform()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want rsp, +got rsp:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}

// Crossplane sends as context a fake resource:
// { "apiVersion": "internal.crossplane.io/v1alpha1", "kind": "Environment", "data": {... the actual environment content ...} }
// See: https://github.com/crossplane/crossplane/blob/806f0d20d146f6f4f1735c5ec6a7dc78923814b3/internal/controller/apiextensions/composite/environment_fetcher.go#L85C1-L85C1
// That's because the patching code expects a resource to be able to use
// runtime.DefaultUnstructuredConverter.FromUnstructured to convert it back to
// an object. This is also why all patches need to specify the full path from data.
func contextWithEnvironment(data map[string]interface{}) *structpb.Struct {
	if data == nil {
		data = map[string]interface{}{}
	}
	u := unstructured.Unstructured{Object: map[string]interface{}{
		"data": data,
	}}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "internal.crossplane.io", Version: "v1alpha1", Kind: "Environment"})
	d, err := structpb.NewStruct(u.UnstructuredContent())
	if err != nil {
		panic(err)
	}
	return &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(d)}}
}
