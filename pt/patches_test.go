package pt

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
)

func TestApplyFromFieldPathPatch(t *testing.T) {
	type args struct {
		p    PatchInterface
		from runtime.Object
		to   runtime.Object
	}

	type want struct {
		to  runtime.Object
		err error
	}

	cases := map[string]struct {
		reason string
		args
		want
	}{
		"ValidFromCompositeFieldPath": {
			reason: "Should correctly apply a valid FromCompositeFieldPath patch",
			args: args{
				p: &v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.labels"),
						ToFieldPath:   ptr.To[string]("metadata.labels"),
					},
				},
				from: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "XR",
							"metadata": {
								"labels": {
									"test": "blah"
								}
							}
						}`)},
				},
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"name": "cd"
							}
						}`)},
				},
			},
			want: want{
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"name": "cd",
								"labels": {
									"test": "blah"
								}
							}
						}`)},
				},
				err: nil,
			},
		},
		"ValidFromFieldPathWithWildcards": {
			reason: "When passed a wildcarded path, adds a field to each element of an array",
			args: args{
				p: &v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.name"),
						ToFieldPath:   ptr.To[string]("metadata.ownerReferences[*].name"),
					},
				},
				from: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "XR",
							"metadata": {
								"name": "test"
							}
						}`)},
				},
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"ownerReferences": [
									{
										"name": ""
									},
									{
										"name": ""
									}
								]
							}
						}`)},
				},
			},
			want: want{
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"ownerReferences": [
									{
										"name": "test"
									},
									{
										"name": "test"
									}
								]
							}
						}`)},
				},
			},
		},
		"InvalidCompositeFieldPathPatchWithWildcards": {
			reason: "When passed a wildcarded path, throws an error if ToFieldPath cannot be expanded",
			args: args{
				p: &v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.name"),
						ToFieldPath:   ptr.To[string]("metadata.ownerReferences[*].badField"),
					},
				},
				from: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "XR",
							"metadata": {
								"name": "test"
							}
						}`)},
				},
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"ownerReferences": [
									{
										"name": "test"
									},
									{
										"name": "test"
									}
								]
							}
						}`)},
				},
			},
			want: want{
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"ownerReferences": [
									{
										"name": "test"
									},
									{
										"name": "test"
									}
								]
							}
						}`)},
				},
				err: errors.Errorf(errFmtExpandingArrayFieldPaths, "metadata.ownerReferences[*].badField"),
			},
		},
		"ValidToFieldPathWithWildcardsAndMergePolicy": {
			reason: "When passed a wildcarded path with appendSlice policy, appends the from field slice items to each of the expanded array field, with slice deduplication",
			args: args{
				p: &v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("spec.parameters.allowedGroups"),
						ToFieldPath:   ptr.To[string]("spec.forProvider.accessRules[*].allowedGroups"),
						Policy: &v1beta1.PatchPolicy{
							ToFieldPath: ptr.To(v1beta1.ToFieldPathPolicyForceMergeObjectsAppendArrays),
						},
					},
				},
				from: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "XR",
							"spec": {
								"parameters": {
									"allowedGroups": [12345678, 7891234]
								}
							}
						}`)},
				},
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"spec": {
								"forProvider": {
									"accessRules": [
										{
											"action": "Allow",
											"destination": "e1",
											"allowedGroups": [12345678]
										},
										{
											"action": "Allow",
											"destination": "e2",
											"allowedGroups": [12345678]
										}
									]
								}
							}
						}`)},
				},
			},
			want: want{
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"spec": {
								"forProvider": {
									"accessRules": [
										{
											"action": "Allow",
											"destination": "e1",
											"allowedGroups": [12345678, 7891234]
										},
										{
											"action": "Allow",
											"destination": "e2",
											"allowedGroups": [12345678, 7891234]
										}
									]
								}
							}
						}`)},
				},
			},
		},
		"DefaultToFieldCompositeFieldPathPatch": {
			reason: "Should correctly default the ToFieldPath value if not specified.",
			args: args{
				p: &v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.labels"),
					},
				},
				from: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "XR",
							"metadata": {
								"labels": {
									"test": "blah"
								}
							}
						}`)},
				},
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed"
						}`)},
				},
			},
			want: want{
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"labels": {
									"test": "blah"
								}
							}
						}`)},
				},
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ApplyFromFieldPathPatch(tc.args.p, tc.args.from, tc.args.to)

			if diff := cmp.Diff(tc.want.to, tc.args.to); diff != "" {
				t.Errorf("\n%s\nApplyFromFieldPathPatch(...): -want, +got:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nApplyFromFieldPathPatch(): -want error, +got error:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestApplyCombineFromVariablesPatch(t *testing.T) {
	errNotFound := func(path string) error {
		p := &fieldpath.Paved{}
		_, err := p.GetValue(path)
		return err
	}

	type args struct {
		p    PatchInterface
		from runtime.Object
		to   runtime.Object
	}

	type want struct {
		to  runtime.Object
		err error
	}

	cases := map[string]struct {
		reason string
		args
		want
	}{
		"VariableFromFieldPathNotFound": {
			reason: "Should return no error and not apply patch if an optional variable is missing",
			args: args{
				p: &v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeCombineFromComposite,
					Patch: v1beta1.Patch{
						Combine: &v1beta1.Combine{
							Variables: []v1beta1.CombineVariable{
								{FromFieldPath: "metadata.labels.source1"},
								{FromFieldPath: "metadata.labels.source2"},
								{FromFieldPath: "metadata.labels.source3"},
							},
							Strategy: v1beta1.CombineStrategyString,
							String:   &v1beta1.StringCombine{Format: "%s-%s"},
						},
						ToFieldPath: ptr.To[string]("metadata.labels.destination"),
					},
				},
				from: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "XR"
						}`)},
				},
			},
			want: want{
				err: errNotFound("metadata"),
			},
		},
		"ValidCombineFromComposite": {
			reason: "Should correctly apply a CombineFromComposite patch with valid settings",
			args: args{
				p: &v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeCombineFromComposite,
					Patch: v1beta1.Patch{
						Combine: &v1beta1.Combine{
							Variables: []v1beta1.CombineVariable{
								{FromFieldPath: "metadata.labels.source1"},
								{FromFieldPath: "metadata.labels.source2"},
							},
							Strategy: v1beta1.CombineStrategyString,
							String:   &v1beta1.StringCombine{Format: "%s-%s"},
						},
						ToFieldPath: ptr.To[string]("metadata.labels.destination"),
					},
				},
				from: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "XR",
							"metadata": {
								"labels": {
									"source1": "foo",
									"source2": "bar"
								}
							}
						}`)},
				},
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"labels": {
									"test": "blah"
								}
							}
						}`)},
				},
			},
			want: want{
				to: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"labels": {
									"destination": "foo-bar",
									"test": "blah"
								}
							}
						}`)},
				},
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ApplyCombineFromVariablesPatch(tc.args.p, tc.args.from, tc.args.to)

			if diff := cmp.Diff(tc.want.to, tc.args.to); diff != "" {
				t.Errorf("\n%s\nApplyCombineFromVariablesPatch(...): -want, +got:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nApplyCombineFromVariablesPatch(): -want error, +got error:\n%s", tc.reason, diff)
			}
		})
	}
}

func MustObject(j string) map[string]any {
	out := map[string]any{}
	if err := json.Unmarshal([]byte(j), &out); err != nil {
		panic(err)
	}
	return out
}

func TestComposedTemplates(t *testing.T) {
	asJSON := func(val interface{}) extv1.JSON {
		raw, err := json.Marshal(val)
		if err != nil {
			t.Fatal(err)
		}
		res := extv1.JSON{}
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatal(err)
		}
		return res
	}

	type args struct {
		pss []v1beta1.PatchSet
		cts []v1beta1.ComposedTemplate
	}

	type want struct {
		ct  []v1beta1.ComposedTemplate
		err error
	}

	cases := map[string]struct {
		reason string
		args
		want
	}{
		"NoCompositionPatchSets": {
			reason: "Patches defined on a composite resource should be applied correctly if no PatchSets are defined on the composition",
			args: args{
				cts: []v1beta1.ComposedTemplate{
					{
						Patches: []v1beta1.ComposedPatch{
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.name"),
								},
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.namespace"),
								},
							},
						},
					},
				},
			},
			want: want{
				ct: []v1beta1.ComposedTemplate{
					{
						Patches: []v1beta1.ComposedPatch{
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.name"),
								},
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.namespace"),
								},
							},
						},
					},
				},
			},
		},
		"UndefinedPatchSet": {
			reason: "Should return error and not modify the patches field when referring to an undefined PatchSet",
			args: args{
				cts: []v1beta1.ComposedTemplate{{
					Patches: []v1beta1.ComposedPatch{
						{
							Type:         v1beta1.PatchTypePatchSet,
							PatchSetName: ptr.To[string]("patch-set-1"),
						},
					},
				}},
			},
			want: want{
				err: errors.Errorf(errFmtUndefinedPatchSet, "patch-set-1"),
			},
		},
		"DefinedPatchSets": {
			reason: "Should de-reference PatchSets defined on the Composition when referenced in a composed resource",
			args: args{
				// PatchSets, existing patches and references
				// should output in the correct order.
				pss: []v1beta1.PatchSet{
					{
						Name: "patch-set-1",
						Patches: []v1beta1.PatchSetPatch{
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.namespace"),
								},
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("spec.parameters.test"),
								},
							},
						},
					},
					{
						Name: "patch-set-2",
						Patches: []v1beta1.PatchSetPatch{
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.annotations.patch-test-1"),
								},
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.annotations.patch-test-2"),
									Transforms: []v1beta1.Transform{{
										Type: v1beta1.TransformTypeMap,
										Map: &v1beta1.MapTransform{
											Pairs: map[string]extv1.JSON{
												"k-1": asJSON("v-1"),
												"k-2": asJSON("v-2"),
											},
										},
									}},
								},
							},
						},
					},
				},
				cts: []v1beta1.ComposedTemplate{
					{
						Patches: []v1beta1.ComposedPatch{
							{
								Type:         v1beta1.PatchTypePatchSet,
								PatchSetName: ptr.To[string]("patch-set-2"),
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.name"),
								},
							},
							{
								Type:         v1beta1.PatchTypePatchSet,
								PatchSetName: ptr.To[string]("patch-set-1"),
							},
						},
					},
					{
						Patches: []v1beta1.ComposedPatch{
							{
								Type:         v1beta1.PatchTypePatchSet,
								PatchSetName: ptr.To[string]("patch-set-1"),
							},
						},
					},
				},
			},
			want: want{
				err: nil,
				ct: []v1beta1.ComposedTemplate{
					{
						Patches: []v1beta1.ComposedPatch{
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.annotations.patch-test-1"),
								},
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.annotations.patch-test-2"),
									Transforms: []v1beta1.Transform{{
										Type: v1beta1.TransformTypeMap,
										Map: &v1beta1.MapTransform{
											Pairs: map[string]extv1.JSON{
												"k-1": asJSON("v-1"),
												"k-2": asJSON("v-2"),
											},
										},
									}},
								},
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.name"),
								},
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.namespace"),
								},
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("spec.parameters.test"),
								},
							},
						},
					},
					{
						Patches: []v1beta1.ComposedPatch{
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("metadata.namespace"),
								},
							},
							{
								Type: v1beta1.PatchTypeFromCompositeFieldPath,
								Patch: v1beta1.Patch{
									FromFieldPath: ptr.To[string]("spec.parameters.test"),
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ComposedTemplates(tc.args.pss, tc.args.cts)

			if diff := cmp.Diff(tc.want.ct, got); diff != "" {
				t.Errorf("\n%s\nrs.ComposedTemplates(...): -want, +got:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nrs.ComposedTemplates(...)): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestResolveTransforms(t *testing.T) {
	type args struct {
		ts    []v1beta1.Transform
		input any
	}
	type want struct {
		output any
		err    error
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "NoTransforms",
			args: args{
				ts: nil,
				input: map[string]interface{}{
					"spec": map[string]interface{}{
						"parameters": map[string]interface{}{
							"test": "test",
						},
					},
				},
			},
			want: want{
				output: map[string]interface{}{
					"spec": map[string]interface{}{
						"parameters": map[string]interface{}{
							"test": "test",
						},
					},
				},
			},
		},
		{
			name: "MathTransformWithConversionToFloat64",
			args: args{
				ts: []v1beta1.Transform{{
					Type: v1beta1.TransformTypeConvert,
					Convert: &v1beta1.ConvertTransform{
						ToType: v1beta1.TransformIOTypeFloat64,
					},
				}, {
					Type: v1beta1.TransformTypeMath,
					Math: &v1beta1.MathTransform{
						Type:     v1beta1.MathTransformTypeMultiply,
						Multiply: ptr.To[int64](2),
					},
				}},
				input: int64(2),
			},
			want: want{
				output: float64(4),
			},
		},
		{
			name: "MathTransformWithConversionToInt64",
			args: args{
				ts: []v1beta1.Transform{{
					Type: v1beta1.TransformTypeConvert,
					Convert: &v1beta1.ConvertTransform{
						ToType: v1beta1.TransformIOTypeInt64,
					},
				}, {
					Type: v1beta1.TransformTypeMath,
					Math: &v1beta1.MathTransform{
						Type:     v1beta1.MathTransformTypeMultiply,
						Multiply: ptr.To[int64](2),
					},
				}},
				input: int64(2),
			},
			want: want{
				output: int64(4),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveTransforms(tt.args.ts, tt.args.input)
			if diff := cmp.Diff(tt.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("ResolveTransforms(...): -want error, +got error:\n%s", diff)
			}

			if diff := cmp.Diff(tt.want.output, got); diff != "" {
				t.Errorf("ResolveTransforms(...): -want, +got:\n%s", diff)
			}
		})
	}
}
