package fn

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
		"OptionalFieldPathNotFound": {
			reason: "If we fail to patch a desired resource because an optional field path was not found we should skip the patch.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD","spec":{}}`)},
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
										// This patch should be skipped, because
										// the path is not found
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.doesNotExist"),
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
								// Watchers becomes "10" because our first patch
								// worked. We only skipped the second patch.
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":"10"}}`),
							},
						},
					},
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
				},
			},
		},
		"RequiredFieldPathNotFound": {
			reason: "If we fail to patch a desired resource because a required field path was not found, and the resource doesn't exist, we should not add it to desired state (i.e. create it).",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "new-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD","spec":{}}`)},
								Patches: []v1beta1.ComposedPatch{
									{
										// This patch will fail because the path
										// is not found.
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.doesNotExist"),
											Policy: &v1beta1.PatchPolicy{
												FromFieldPath: ptr.To[v1beta1.FromFieldPathPolicy](v1beta1.FromFieldPathPolicyRequired),
											},
										},
									},
								},
							},
							{
								Name: "existing-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"targetObject": {"keep": "me"}}}`)},
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
										// This patch should work too and properly handle mergeOptions.
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.sourceObject"),
											ToFieldPath:   ptr.To[string]("spec.targetObject"),
											Policy: &v1beta1.PatchPolicy{
												ToFieldPath: ptr.To(v1beta1.ToFieldPathPolicyMergeObject),
											},
										},
									},
									{
										// This patch will fail because the path
										// is not found.
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.doesNotExist"),
											Policy: &v1beta1.PatchPolicy{
												FromFieldPath: ptr.To[v1beta1.FromFieldPathPolicy](v1beta1.FromFieldPathPolicyRequired),
											},
										},
									},
								},
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR","spec":{"widgets":"10", "sourceObject": {"me": "too"}}}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							// "existing-resource" exists.
							"existing-resource": {},

							// Note "new-resource" doesn't appear in the
							// observed resources. It doesn't yet exist.
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
							// Note that the first patch did work. We only
							// skipped the patch from the required field path.
							"existing-resource": {
								Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"CD","spec":{"watchers":"10", "targetObject": {"me": "too", "keep": "me"}}}`),
							},

							// Note "new-resource" doesn't appear here.
						},
					},
					Context: &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(nil)}},
					Results: []*fnv1beta1.Result{
						{
							Severity: fnv1beta1.Severity_SEVERITY_WARNING,
							Message:  `not adding new composed resource "new-resource" to desired state because "FromCompositeFieldPath" patch at index 0 has 'policy.fromFieldPath: Required': spec.doesNotExist: no such field`,
						},
						{
							Severity: fnv1beta1.Severity_SEVERITY_WARNING,
							Message:  `cannot render composed resource "existing-resource" "FromCompositeFieldPath" patch at index 2: ignoring 'policy.fromFieldPath: Required' because 'to' resource already exists: spec.doesNotExist: no such field`,
						},
					},
				},
			},
		},
		"PatchErrorIsFatal": {
			reason: "If we fail to patch a desired resource we should return a fatal result.",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Resources{
						Resources: []v1beta1.ComposedTemplate{
							{
								Name: "cool-resource",
								Base: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"example.org/v1","kind":"CD","spec":{}}`)},
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
										// because the path is not an array.
										Type: v1beta1.PatchTypeFromCompositeFieldPath,
										Patch: v1beta1.Patch{
											FromFieldPath: ptr.To[string]("spec.widgets[0]"),
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
							Message:  fmt.Sprintf("cannot render composed resource %q %q patch at index 1: spec.widgets: not an array", "cool-resource", "FromCompositeFieldPath"),
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
									Type: v1beta1.PatchTypeToCompositeFieldPath,
									Patch: v1beta1.Patch{
										FromFieldPath: ptr.To[string]("widgets"),
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
									Type: v1beta1.PatchTypeFromCompositeFieldPath,
									Patch: v1beta1.Patch{
										FromFieldPath: ptr.To[string]("spec.watchers"),
										ToFieldPath:   ptr.To[string]("widgets"),
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
									FromFieldPath: ptr.To[string]("widgets"),
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
									FromFieldPath: ptr.To[string]("widgets"),
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
									FromFieldPath: ptr.To[string]("widgets"),
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
// { "apiVersion": "internal.crossplane.io/v1alpha1", "kind": "Environment", ... the actual environment content ... }
// See: https://github.com/crossplane/crossplane/blob/806f0d20d146f6f4f1735c5ec6a7dc78923814b3/internal/controller/apiextensions/composite/environment_fetcher.go#L85C1-L85C1
// That's because the patching code expects a resource to be able to use
// runtime.DefaultUnstructuredConverter.FromUnstructured to convert it back to
// an object.
func contextWithEnvironment(data map[string]interface{}) *structpb.Struct {
	if data == nil {
		data = map[string]interface{}{}
	}
	u := unstructured.Unstructured{Object: data}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "internal.crossplane.io", Version: "v1alpha1", Kind: "Environment"})
	d, err := structpb.NewStruct(u.UnstructuredContent())
	if err != nil {
		panic(err)
	}
	return &structpb.Struct{Fields: map[string]*structpb.Value{fncontext.KeyEnvironment: structpb.NewStructValue(d)}}
}
