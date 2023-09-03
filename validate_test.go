package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"

	"github.com/negz/function-patch-and-transform/input/v1beta1"
)

func TestValidateReadinessCheck(t *testing.T) {
	type args struct {
		r v1beta1.ReadinessCheck
	}
	type want struct {
		output *field.Error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ValidTypeNone": {
			reason: "Type none should be valid",
			args: args{
				r: v1beta1.ReadinessCheck{
					Type: v1beta1.ReadinessCheckTypeNone,
				},
			},
		},
		"ValidTypeMatchString": {
			reason: "Type matchString should be valid",
			args: args{
				r: v1beta1.ReadinessCheck{
					Type:        v1beta1.ReadinessCheckTypeMatchString,
					MatchString: pointer.String("foo"),
					FieldPath:   pointer.String("spec.foo"),
				},
			},
		},
		"ValidTypeMatchCondition": {
			reason: "Type matchCondition should be valid",
			args: args{
				r: v1beta1.ReadinessCheck{
					Type: v1beta1.ReadinessCheckTypeMatchCondition,
					MatchCondition: &v1beta1.MatchConditionReadinessCheck{
						Type:   "someType",
						Status: "someStatus",
					},
					FieldPath: pointer.String("spec.foo"),
				},
			},
		},
		"ValidTypeMatchTrue": {
			reason: "Type matchTrue should be valid",
			args: args{
				r: v1beta1.ReadinessCheck{
					Type:      v1beta1.ReadinessCheckTypeMatchTrue,
					FieldPath: pointer.String("spec.foo"),
				},
			},
		},
		"ValidTypeMatchFalse": {
			reason: "Type matchFalse should be valid",
			args: args{
				r: v1beta1.ReadinessCheck{
					Type:      v1beta1.ReadinessCheckTypeMatchFalse,
					FieldPath: pointer.String("spec.foo"),
				},
			},
		},
		"InvalidType": {
			reason: "Invalid type",
			args: args{
				r: v1beta1.ReadinessCheck{
					Type: "foo",
				},
			},
			want: want{
				output: &field.Error{
					Type:     field.ErrorTypeInvalid,
					Field:    "type",
					BadValue: "foo",
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := ValidateReadinessCheck(tc.args.r)
			if diff := cmp.Diff(tc.want.output, got, cmpopts.IgnoreFields(field.Error{}, "Detail", "BadValue")); diff != "" {
				t.Errorf("%s\nValidateReadinessCheck(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestValidatePatch(t *testing.T) {
	type args struct {
		patch v1beta1.Patch
	}

	type want struct {
		err *field.Error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ValidFromCompositeFieldPath": {
			reason: "FromCompositeFieldPath patch with FromFieldPath set should be valid",
			args: args{
				patch: v1beta1.Patch{
					Type:          v1beta1.PatchTypeFromCompositeFieldPath,
					FromFieldPath: pointer.String("spec.forProvider.foo"),
				},
			},
		},
		"FromCompositeFieldPathWithInvalidTransforms": {
			reason: "FromCompositeFieldPath with invalid transforms should return error",
			args: args{
				patch: v1beta1.Patch{
					Type:          v1beta1.PatchTypeFromCompositeFieldPath,
					FromFieldPath: pointer.String("spec.forProvider.foo"),
					Transforms: []v1beta1.Transform{
						{
							Type: v1beta1.TransformTypeMath,
							Math: nil,
						},
					},
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "transforms[0].math",
				},
			},
		},
		"InvalidFromCompositeFieldPathMissingFromFieldPath": {
			reason: "Invalid FromCompositeFieldPath missing FromFieldPath should return error",
			args: args{
				patch: v1beta1.Patch{
					Type:          v1beta1.PatchTypeFromCompositeFieldPath,
					FromFieldPath: nil,
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "fromFieldPath",
				},
			},
		},
		"InvalidFromCompositeFieldPathMissingToFieldPath": {
			reason: "Invalid ToCompositeFieldPath missing ToFieldPath should return error",
			args: args{
				patch: v1beta1.Patch{
					Type:        v1beta1.PatchTypeToCompositeFieldPath,
					ToFieldPath: nil,
				},
			},
			want: want{
				&field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "fromFieldPath",
				},
			},
		},
		"Invalidv1beta1.PatchSetMissingv1beta1.PatchSetName": {
			reason: "Invalid v1beta1.PatchSet missing v1beta1.PatchSetName should return error",
			args: args{
				patch: v1beta1.Patch{
					Type: v1beta1.PatchTypePatchSet,
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "patchSetName",
				},
			},
		},
		"InvalidCombineMissingCombine": {
			reason: "Invalid Combine missing Combine should return error",
			args: args{
				patch: v1beta1.Patch{
					Type: v1beta1.PatchTypeCombineToComposite,
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "combine",
				},
			},
		},
		"InvalidCombineMissingToFieldPath": {
			reason: "Invalid Combine missing ToFieldPath should return error",
			args: args{
				patch: v1beta1.Patch{
					Type: v1beta1.PatchTypeCombineToComposite,
					Combine: &v1beta1.Combine{
						Variables: []v1beta1.CombineVariable{
							{
								FromFieldPath: "spec.forProvider.foo",
							},
						},
					},
					ToFieldPath: nil,
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "toFieldPath",
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidatePatch(tc.args.patch)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.IgnoreFields(field.Error{}, "Detail", "BadValue")); diff != "" {
				t.Errorf("%s\nValidatePatch(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestValidateTransform(t *testing.T) {
	type args struct {
		transform v1beta1.Transform
	}
	type want struct {
		err *field.Error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ValidMathMultiply": {
			reason: "Math transform with MathTransform Multiply set should be valid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMath,
					Math: &v1beta1.MathTransform{
						Type:     v1beta1.MathTransformTypeMultiply,
						Multiply: pointer.Int64(2),
					},
				},
			},
		},
		"ValidMathDefaultType": {
			reason: "Math transform with MathTransform Default set should be valid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMath,
					Math: &v1beta1.MathTransform{
						Multiply: pointer.Int64(2),
					},
				},
			},
		},
		"ValidMathClampMin": {
			reason: "Math transform with valid MathTransform ClampMin set should be valid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMath,
					Math: &v1beta1.MathTransform{
						Type:     v1beta1.MathTransformTypeClampMin,
						ClampMin: pointer.Int64(10),
					},
				},
			},
		},
		"InvalidMathWrongSpec": {
			reason: "Math transform with invalid MathTransform set should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMath,
					Math: &v1beta1.MathTransform{
						Type:     v1beta1.MathTransformTypeMultiply,
						ClampMin: pointer.Int64(10),
					},
				},
			},
			want: want{
				&field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "math.multiply",
				},
			},
		},
		"InvalidMathNotDefinedAtAll": {
			reason: "Math transform with no MathTransform set should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMath,
					Math: nil,
				},
			},
			want: want{
				&field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "math",
				},
			},
		},
		"ValidMap": {
			reason: "Map transform with MapTransform set should be valid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMap,
					Map: &v1beta1.MapTransform{
						Pairs: map[string]extv1.JSON{
							"foo": {Raw: []byte(`"bar"`)},
						},
					},
				},
			},
		},
		"InvalidMapNoMap": {
			reason: "Map transform with no map set should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMap,
					Map:  nil,
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "map",
				},
			},
		},
		"InvalidMapNoPairs": {
			reason: "Map transform with no pairs in map should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMap,
					Map:  &v1beta1.MapTransform{},
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "map.pairs",
				},
			},
		},
		"InvalidMatchNoMatch": {
			reason: "Match transform with no match set should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type:  v1beta1.TransformTypeMatch,
					Match: nil,
				},
			},
			want: want{
				&field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "match",
				},
			},
		},
		"InvalidMatchEmptyTransform": {
			reason: "Match transform with empty MatchTransform should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type:  v1beta1.TransformTypeMatch,
					Match: &v1beta1.MatchTransform{},
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "match.patterns",
				},
			},
		},
		"ValidMatchTransformRegexp": {
			reason: "Match transform with valid MatchTransform of type regexp should be valid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMatch,
					Match: &v1beta1.MatchTransform{
						Patterns: []v1beta1.MatchTransformPattern{
							{
								Type:   v1beta1.MatchTransformPatternTypeRegexp,
								Regexp: pointer.String(".*"),
							},
						},
					},
				},
			},
		},
		"InvalidMatchTransformRegexp": {
			reason: "Match transform with an invalid MatchTransform of type regexp with a bad regexp should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMatch,
					Match: &v1beta1.MatchTransform{
						Patterns: []v1beta1.MatchTransformPattern{
							{
								Type:   v1beta1.MatchTransformPatternTypeRegexp,
								Regexp: pointer.String("?"),
							},
						},
					},
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeInvalid,
					Field: "match.patterns[0].regexp",
				},
			},
		},
		"ValidMatchTransformString": {
			reason: "Match transform with valid MatchTransform of type literal should be valid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeMatch,
					Match: &v1beta1.MatchTransform{
						Patterns: []v1beta1.MatchTransformPattern{
							{
								Type:    v1beta1.MatchTransformPatternTypeLiteral,
								Literal: pointer.String("foo"),
							},
							{
								Literal: pointer.String("bar"),
							},
						},
					},
				},
			},
		},
		"InvalidStringNoString": {
			reason: "String transform with no string set should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type:   v1beta1.TransformTypeString,
					String: nil,
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "string",
				},
			},
		},
		"ValidString": {
			reason: "String transform with set string should be valid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeString,
					String: &v1beta1.StringTransform{
						Format: pointer.String("foo"),
					},
				},
			},
		},
		"InvalidConvertMissingConvert": {
			reason: "Convert transform missing Convert should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type:    v1beta1.TransformTypeConvert,
					Convert: nil,
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "convert",
				},
			},
		},
		"InvalidConvertUnknownFormat": {
			reason: "Convert transform with unknown format should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeConvert,
					Convert: &v1beta1.ConvertTransform{
						Format: &[]v1beta1.ConvertTransformFormat{"foo"}[0],
					},
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeInvalid,
					Field: "convert.format",
				},
			},
		},
		"InvalidConvertUnknownToType": {
			reason: "Convert transform with unknown toType should be invalid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeConvert,
					Convert: &v1beta1.ConvertTransform{
						ToType: v1beta1.TransformIOType("foo"),
					},
				},
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeInvalid,
					Field: "convert.toType",
				},
			},
		},
		"ValidConvert": {
			reason: "Convert transform with valid format and toType should be valid",
			args: args{
				transform: v1beta1.Transform{
					Type: v1beta1.TransformTypeConvert,
					Convert: &v1beta1.ConvertTransform{
						Format: &[]v1beta1.ConvertTransformFormat{v1beta1.ConvertTransformFormatNone}[0],
						ToType: v1beta1.TransformIOTypeInt,
					},
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateTransform(tc.args.transform)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.IgnoreFields(field.Error{}, "Detail", "BadValue")); diff != "" {
				t.Errorf("%s\nValidateTransform(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}