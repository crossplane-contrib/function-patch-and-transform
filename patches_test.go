package main

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composed"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/function-sdk-go/resource/composite"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
)

func TestPatchApply(t *testing.T) {
	errNotFound := func(path string) error {
		p := &fieldpath.Paved{}
		_, err := p.GetValue(path)
		return err
	}

	type args struct {
		patch v1beta1.ComposedPatch
		xr    *composite.Unstructured
		cd    *composed.Unstructured
		only  []v1beta1.PatchType
	}
	type want struct {
		xr  *composite.Unstructured
		cd  *composed.Unstructured
		err error
	}

	cases := map[string]struct {
		reason string
		args
		want
	}{
		"InvalidCompositeFieldPathPatch": {
			reason: "Should return error when required fields not passed to applyFromFieldPathPatch",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					// This is missing fields.
				},
				xr: &composite.Unstructured{},
				cd: &composed.Unstructured{},
			},
			want: want{
				err: errors.Errorf(errFmtRequiredField, "FromFieldPath", v1beta1.PatchTypeFromCompositeFieldPath),
			},
		},
		"Invalidv1.PatchType": {
			reason: "Should return an error if an invalid patch type is specified",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: "invalid-patchtype",
				},
				xr: &composite.Unstructured{},
				cd: &composed.Unstructured{},
			},
			want: want{
				err: errors.Errorf(errFmtInvalidPatchType, "invalid-patchtype"),
			},
		},
		"ValidCompositeFieldPathPatch": {
			reason: "Should correctly apply a CompositeFieldPathPatch with valid settings",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.labels"),
						ToFieldPath:   ptr.To[string]("metadata.labels"),
					},
				},
				xr: &composite.Unstructured{
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
				cd: &composed.Unstructured{
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
				cd: &composed.Unstructured{
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
		"ValidCompositeFieldPathPatchWithWildcards": {
			reason: "When passed a wildcarded path, adds a field to each element of an array",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.name"),
						ToFieldPath:   ptr.To[string]("metadata.ownerReferences[*].name"),
					},
				},
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "XR",
						"metadata": {
							"name": "test"
						}
					}`)},
				},
				cd: &composed.Unstructured{
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
				cd: &composed.Unstructured{
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
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.name"),
						ToFieldPath:   ptr.To[string]("metadata.ownerReferences[*].badField"),
					},
				},
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "XR",
						"metadata": {
							"name": "test"
						}
					}`)},
				},
				cd: &composed.Unstructured{
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
				err: errors.Errorf(errFmtExpandingArrayFieldPaths, "metadata.ownerReferences[*].badField"),
			},
		},
		"MissingOptionalFieldPath": {
			reason: "A FromFieldPath patch should be a no-op when an optional fromFieldPath doesn't exist",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.labels"),
						ToFieldPath:   ptr.To[string]("metadata.labels"),
					},
				},
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "XR",
						"metadata": {
							"name": "test"
						}
					}`)},
				},
				cd: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "Composed",
						"metadata": {
							"name": "test"
						}
					}`)},
				},
			},
			want: want{
				cd: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "Composed",
						"metadata": {
							"name": "test"
						}
					}`)},
				},
				err: nil,
			},
		},
		"MissingRequiredFieldPath": {
			reason: "A FromFieldPath patch should return an error when a required fromFieldPath doesn't exist",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("wat"),
						Policy: &v1beta1.PatchPolicy{
							FromFieldPath: func() *v1beta1.FromFieldPathPolicy {
								s := v1beta1.FromFieldPathPolicyRequired
								return &s
							}(),
						},
						ToFieldPath: ptr.To[string]("wat"),
					},
				},
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "XR",
						"metadata": {
							"name": "test"
						}
					}`)},
				},
				cd: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "Composed",
						"metadata": {
							"name": "test"
						}
					}`)},
				},
			},
			want: want{
				cd: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "Composed",
						"metadata": {
							"name": "test"
						}
					}`)},
				},
				err: errNotFound("wat"),
			},
		},
		"FilterExcludeCompositeFieldPathPatch": {
			reason: "Should not apply the patch as the v1.PatchType is not present in filter.",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.labels"),
						ToFieldPath:   ptr.To[string]("metadata.labels"),
					},
				},
				xr: &composite.Unstructured{
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
				cd:   &composed.Unstructured{},
				only: []v1beta1.PatchType{v1beta1.PatchTypePatchSet},
			},
			want: want{
				cd:  &composed.Unstructured{},
				err: nil,
			},
		},
		"FilterIncludeCompositeFieldPathPatch": {
			reason: "Should apply the patch as the v1.PatchType is present in filter.",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.labels"),
						ToFieldPath:   ptr.To[string]("metadata.labels"),
					},
				},
				xr: &composite.Unstructured{
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
				cd: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "Composed"
					}`)},
				},
				only: []v1beta1.PatchType{v1beta1.PatchTypeFromCompositeFieldPath},
			},
			want: want{
				cd: &composed.Unstructured{
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
		"DefaultToFieldCompositeFieldPathPatch": {
			reason: "Should correctly default the ToFieldPath value if not specified.",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeFromCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.labels"),
					},
				},
				xr: &composite.Unstructured{
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
				cd: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "Composed"
					}`)},
				},
			},
			want: want{
				cd: &composed.Unstructured{
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
		"ValidToCompositeFieldPathPatch": {
			reason: "Should correctly apply a ToCompositeFieldPath patch with valid settings",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeToCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.labels"),
						ToFieldPath:   ptr.To[string]("metadata.labels"),
					},
				},
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "XR"
					}`)},
				},
				cd: &composed.Unstructured{
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
				xr: &composite.Unstructured{
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
				err: nil,
			},
		},
		"ValidToCompositeFieldPathPatchWithWildcards": {
			reason: "When passed a wildcarded path, adds a field to each element of an array",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeToCompositeFieldPath,
					Patch: v1beta1.Patch{
						FromFieldPath: ptr.To[string]("metadata.name"),
						ToFieldPath:   ptr.To[string]("metadata.ownerReferences[*].name"),
					},
				},
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "XR",
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
				cd: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "Composed",
						"metadata": {
							"name": "test"
						}
					}`)},
				},
			},
			want: want{
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "XR",
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
		"MissingCombineFromCompositeConfig": {
			reason: "Should return an error if Combine config is not passed",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeCombineFromComposite,
					Patch: v1beta1.Patch{
						ToFieldPath: ptr.To[string]("metadata.labels.destination"),
						// Missing a Combine field
						Combine: nil,
					},
				},
				xr: &composite.Unstructured{},
				cd: &composed.Unstructured{},
			},
			want: want{
				xr:  &composite.Unstructured{},
				cd:  &composed.Unstructured{},
				err: errors.Errorf(errFmtRequiredField, "Combine", v1beta1.PatchTypeCombineFromComposite),
			},
		},
		"MissingCombineStrategyFromCompositeConfig": {
			reason: "Should return an error if Combine strategy config is not passed",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeCombineFromComposite,
					Patch: v1beta1.Patch{
						Combine: &v1beta1.Combine{
							Variables: []v1beta1.CombineVariable{
								{FromFieldPath: "metadata.labels.source1"},
								{FromFieldPath: "metadata.labels.source2"},
							},
							Strategy: v1beta1.CombineStrategyString,
							// Missing a String combine config.
						},
						ToFieldPath: ptr.To[string]("metadata.labels.destination"),
					},
				},
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "XR",
						"metadata": {
							"labels": {
								"source1": "a",
								"source2": "b"
							}
						}
					}`)},
				},
			},
			want: want{
				err: errors.Errorf(errFmtCombineConfigMissing, v1beta1.CombineStrategyString),
			},
		},
		"MissingCombineVariablesFromCompositeConfig": {
			reason: "Should return an error if no variables have been passed",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeCombineFromComposite,
					Patch: v1beta1.Patch{
						Combine: &v1beta1.Combine{
							// This is empty.
							Variables: []v1beta1.CombineVariable{},
							Strategy:  v1beta1.CombineStrategyString,
							String:    &v1beta1.StringCombine{Format: "%s-%s"},
						},
						ToFieldPath: ptr.To[string]("objectMeta.labels.destination"),
					},
				},
			},
			want: want{
				err: errors.New(errCombineRequiresVariables),
			},
		},
		"NoOpOptionalInputFieldFromCompositeConfig": {
			// Note: OptionalFieldPathNotFound is tested below, but we want to
			// test that we abort the patch if _any_ of our source fields are
			// not available.
			reason: "Should return no error and not apply patch if an optional variable is missing",
			args: args{
				patch: v1beta1.ComposedPatch{
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
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
						"apiVersion": "test.crossplane.io/v1",
						"kind": "XR",
						"metadata": {
							"labels": {
								"source1": "foo",
								"source3": "baz"
							}
						}
					}`)},
				},
			},
			want: want{
				err: nil,
			},
		},
		"ValidCombineFromComposite": {
			reason: "Should correctly apply a CombineFromComposite patch with valid settings",
			args: args{
				patch: v1beta1.ComposedPatch{
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
				xr: &composite.Unstructured{
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
				cd: &composed.Unstructured{
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
				cd: &composed.Unstructured{
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
		"ValidCombineToComposite": {
			reason: "Should correctly apply a CombineToComposite patch with valid settings",
			args: args{
				patch: v1beta1.ComposedPatch{
					Type: v1beta1.PatchTypeCombineToComposite,
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
				xr: &composite.Unstructured{
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
				cd: &composed.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "Composed",
							"metadata": {
								"labels": {
									"source1": "foo",
									"source2": "bar"
								}
							}
						}`)},
				},
			},
			want: want{
				xr: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{Object: MustObject(`{
							"apiVersion": "test.crossplane.io/v1",
							"kind": "XR",
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
			ncp := tc.args.xr.DeepCopyObject().(resource.Composite)
			err := Apply(&tc.args.patch, ncp, tc.args.cd, tc.args.only...)

			if tc.want.xr != nil {
				if diff := cmp.Diff(tc.want.xr, ncp); diff != "" {
					t.Errorf("\n%s\nApply(cp): -want, +got:\n%s", tc.reason, diff)
				}
			}
			if tc.want.cd != nil {
				if diff := cmp.Diff(tc.want.cd, tc.args.cd); diff != "" {
					t.Errorf("\n%s\nApply(cd): -want, +got:\n%s", tc.reason, diff)
				}
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nApply(err): -want, +got:\n%s", tc.reason, diff)
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

func TestOptionalFieldPathNotFound(t *testing.T) {
	errBoom := errors.New("boom")
	errNotFound := func() error {
		p := &fieldpath.Paved{}
		_, err := p.GetValue("boom")
		return err
	}
	required := v1beta1.FromFieldPathPolicyRequired
	optional := v1beta1.FromFieldPathPolicyOptional
	type args struct {
		err error
		p   *v1beta1.PatchPolicy
	}

	cases := map[string]struct {
		reason string
		args
		want bool
	}{
		"NotAnError": {
			reason: "Should perform patch if no error finding field.",
			args:   args{},
			want:   false,
		},
		"NotFieldNotFoundError": {
			reason: "Should return error if something other than field not found.",
			args: args{
				err: errBoom,
			},
			want: false,
		},
		"DefaultOptionalNoPolicy": {
			reason: "Should return no-op if field not found and no patch policy specified.",
			args: args{
				err: errNotFound(),
			},
			want: true,
		},
		"DefaultOptionalNoPathPolicy": {
			reason: "Should return no-op if field not found and empty patch policy specified.",
			args: args{
				p:   &v1beta1.PatchPolicy{},
				err: errNotFound(),
			},
			want: true,
		},
		"OptionalNotFound": {
			reason: "Should return no-op if field not found and optional patch policy explicitly specified.",
			args: args{
				p: &v1beta1.PatchPolicy{
					FromFieldPath: &optional,
				},
				err: errNotFound(),
			},
			want: true,
		},
		"RequiredNotFound": {
			reason: "Should return error if field not found and required patch policy explicitly specified.",
			args: args{
				p: &v1beta1.PatchPolicy{
					FromFieldPath: &required,
				},
				err: errNotFound(),
			},
			want: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := IsOptionalFieldPathNotFound(tc.args.err, tc.args.p)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("IsOptionalFieldPathNotFound(...): -want, +got:\n%s", diff)
			}
		})
	}
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
