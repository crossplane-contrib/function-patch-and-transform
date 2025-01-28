package pt

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
)

func TestMapResolve(t *testing.T) {
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
		t *v1beta1.MapTransform
		i any
	}
	type want struct {
		o   any
		err error
	}

	cases := map[string]struct {
		args
		want
	}{
		"NonStringInput": {
			args: args{
				t: &v1beta1.MapTransform{},
				i: 5,
			},
			want: want{
				err: errors.Errorf(errFmtMapTypeNotSupported, "int"),
			},
		},
		"KeyNotFound": {
			args: args{
				t: &v1beta1.MapTransform{},
				i: "ola",
			},
			want: want{
				err: errors.Errorf(errFmtMapNotFound, "ola"),
			},
		},
		"SuccessString": {
			args: args{
				t: &v1beta1.MapTransform{Pairs: map[string]extv1.JSON{"ola": asJSON("voila")}},
				i: "ola",
			},
			want: want{
				o: "voila",
			},
		},
		"SuccessNumber": {
			args: args{
				t: &v1beta1.MapTransform{Pairs: map[string]extv1.JSON{"ola": asJSON(1.0)}},
				i: "ola",
			},
			want: want{
				o: 1.0,
			},
		},
		"SuccessBoolean": {
			args: args{
				t: &v1beta1.MapTransform{Pairs: map[string]extv1.JSON{"ola": asJSON(true)}},
				i: "ola",
			},
			want: want{
				o: true,
			},
		},
		"SuccessObject": {
			args: args{
				t: &v1beta1.MapTransform{Pairs: map[string]extv1.JSON{"ola": asJSON(map[string]interface{}{"foo": "bar"})}},
				i: "ola",
			},
			want: want{
				o: map[string]interface{}{"foo": "bar"},
			},
		},
		"SuccessSlice": {
			args: args{
				t: &v1beta1.MapTransform{Pairs: map[string]extv1.JSON{"ola": asJSON([]string{"foo", "bar"})}},
				i: "ola",
			},
			want: want{
				o: []interface{}{"foo", "bar"},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ResolveMap(tc.t, tc.i)

			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestMatchResolve(t *testing.T) {
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
		t *v1beta1.MatchTransform
		i any
	}
	type want struct {
		o   any
		err error
	}

	cases := map[string]struct {
		args
		want
	}{
		"ErrNonStringInput": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:    v1beta1.MatchTransformPatternTypeLiteral,
							Literal: ptr.To[string]("5"),
						},
					},
				},
				i: 5,
			},
			want: want{
				err: errors.Wrapf(errors.Errorf(errFmtMatchInputTypeInvalid, "int"), errFmtMatchPattern, 0),
			},
		},
		"ErrFallbackValueAndToInput": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns:      []v1beta1.MatchTransformPattern{},
					FallbackValue: asJSON("foo"),
					FallbackTo:    "Input",
				},
				i: "foo",
			},
			want: want{
				err: errors.New(errMatchFallbackBoth),
			},
		},
		"NoPatternsFallback": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns:      []v1beta1.MatchTransformPattern{},
					FallbackValue: asJSON("bar"),
				},
				i: "foo",
			},
			want: want{
				o: "bar",
			},
		},
		"NoPatternsFallbackToValueExplicit": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns:      []v1beta1.MatchTransformPattern{},
					FallbackValue: asJSON("bar"),
					FallbackTo:    "Value", // Explicitly set to Value, unnecessary but valid.
				},
				i: "foo",
			},
			want: want{
				o: "bar",
			},
		},
		"NoPatternsFallbackNil": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns:      []v1beta1.MatchTransformPattern{},
					FallbackValue: asJSON(nil),
				},
				i: "foo",
			},
			want: want{},
		},
		"NoPatternsFallbackToInput": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns:   []v1beta1.MatchTransformPattern{},
					FallbackTo: "Input",
				},
				i: "foo",
			},
			want: want{
				o: "foo",
			},
		},
		"NoPatternsFallbackNilToInput": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns:      []v1beta1.MatchTransformPattern{},
					FallbackValue: asJSON(nil),
					FallbackTo:    "Input",
				},
				i: "foo",
			},
			want: want{
				o: "foo",
			},
		},
		"MatchLiteral": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:    v1beta1.MatchTransformPatternTypeLiteral,
							Literal: ptr.To[string]("foo"),
							Result:  asJSON("bar"),
						},
					},
				},
				i: "foo",
			},
			want: want{
				o: "bar",
			},
		},
		"MatchLiteralFirst": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:    v1beta1.MatchTransformPatternTypeLiteral,
							Literal: ptr.To[string]("foo"),
							Result:  asJSON("bar"),
						},
						{
							Type:    v1beta1.MatchTransformPatternTypeLiteral,
							Literal: ptr.To[string]("foo"),
							Result:  asJSON("not this"),
						},
					},
				},
				i: "foo",
			},
			want: want{
				o: "bar",
			},
		},
		"MatchLiteralWithResultStruct": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:    v1beta1.MatchTransformPatternTypeLiteral,
							Literal: ptr.To[string]("foo"),
							Result: asJSON(map[string]interface{}{
								"Hello": "World",
							}),
						},
					},
				},
				i: "foo",
			},
			want: want{
				o: map[string]interface{}{
					"Hello": "World",
				},
			},
		},
		"MatchLiteralWithResultSlice": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:    v1beta1.MatchTransformPatternTypeLiteral,
							Literal: ptr.To[string]("foo"),
							Result: asJSON([]string{
								"Hello", "World",
							}),
						},
					},
				},
				i: "foo",
			},
			want: want{
				o: []any{
					"Hello", "World",
				},
			},
		},
		"MatchLiteralWithResultNumber": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:    v1beta1.MatchTransformPatternTypeLiteral,
							Literal: ptr.To[string]("foo"),
							Result:  asJSON(5),
						},
					},
				},
				i: "foo",
			},
			want: want{
				o: 5.0,
			},
		},
		"MatchLiteralWithResultBool": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:    v1beta1.MatchTransformPatternTypeLiteral,
							Literal: ptr.To[string]("foo"),
							Result:  asJSON(true),
						},
					},
				},
				i: "foo",
			},
			want: want{
				o: true,
			},
		},
		"MatchLiteralWithResultNil": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:    v1beta1.MatchTransformPatternTypeLiteral,
							Literal: ptr.To[string]("foo"),
							Result:  asJSON(nil),
						},
					},
				},
				i: "foo",
			},
			want: want{},
		},
		"MatchRegexp": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:   v1beta1.MatchTransformPatternTypeRegexp,
							Regexp: ptr.To[string]("^foo.*$"),
							Result: asJSON("Hello World"),
						},
					},
				},
				i: "foobar",
			},
			want: want{
				o: "Hello World",
			},
		},
		"ErrMissingRegexp": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type: v1beta1.MatchTransformPatternTypeRegexp,
						},
					},
				},
			},
			want: want{
				err: errors.Wrapf(errors.Errorf(errFmtRequiredField, "regexp", string(v1beta1.MatchTransformPatternTypeRegexp)), errFmtMatchPattern, 0),
			},
		},
		"ErrInvalidRegexp": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type:   v1beta1.MatchTransformPatternTypeRegexp,
							Regexp: ptr.To[string]("?="),
						},
					},
				},
			},
			want: want{
				// This might break if Go's regexp changes its internal error
				// messages:
				err: errors.Wrapf(errors.Wrapf(errors.Wrap(errors.Wrap(errors.New("`?`"), "missing argument to repetition operator"), "error parsing regexp"), errMatchRegexpCompile), errFmtMatchPattern, 0),
			},
		},
		"ErrMissingLiteral": {
			args: args{
				t: &v1beta1.MatchTransform{
					Patterns: []v1beta1.MatchTransformPattern{
						{
							Type: v1beta1.MatchTransformPatternTypeLiteral,
						},
					},
				},
			},
			want: want{
				err: errors.Wrapf(errors.Errorf(errFmtRequiredField, "literal", string(v1beta1.MatchTransformPatternTypeLiteral)), errFmtMatchPattern, 0),
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ResolveMatch(tc.args.t, tc.i)

			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestMathResolve(t *testing.T) {
	two := int64(2)

	type args struct {
		mathType   v1beta1.MathTransformType
		multiplier *int64
		clampMin   *int64
		clampMax   *int64
		i          any
	}
	type want struct {
		o   any
		err error
	}

	cases := map[string]struct {
		args
		want
	}{
		"InvalidType": {
			args: args{
				mathType: "bad",
				i:        25,
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeInvalid,
					Field: "type",
				},
			},
		},
		"NonNumberInput": {
			args: args{
				mathType:   v1beta1.MathTransformTypeMultiply,
				multiplier: &two,
				i:          "ola",
			},
			want: want{
				err: errors.Errorf(errFmtMathInputNonNumber, "ola"),
			},
		},
		"MultiplyNoConfig": {
			args: args{
				mathType: v1beta1.MathTransformTypeMultiply,
				i:        25,
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "multiply",
				},
			},
		},
		"MultiplySuccess": {
			args: args{
				mathType:   v1beta1.MathTransformTypeMultiply,
				multiplier: &two,
				i:          3,
			},
			want: want{
				o: 3 * two,
			},
		},
		"MultiplySuccessInt64": {
			args: args{
				mathType:   v1beta1.MathTransformTypeMultiply,
				multiplier: &two,
				i:          int64(3),
			},
			want: want{
				o: 3 * two,
			},
		},
		"ClampMinSuccess": {
			args: args{
				mathType: v1beta1.MathTransformTypeClampMin,
				clampMin: &two,
				i:        1,
			},
			want: want{
				o: int64(2),
			},
		},
		"ClampMinSuccessNoChangeInt": {
			args: args{
				mathType: v1beta1.MathTransformTypeClampMin,
				clampMin: &two,
				i:        3,
			},
			want: want{
				o: 3,
			},
		},
		"ClampMinSuccessNoChangeInt64": {
			args: args{
				mathType: v1beta1.MathTransformTypeClampMin,
				clampMin: &two,
				i:        int64(3),
			},
			want: want{
				o: int64(3),
			},
		},
		"ClampMinSuccessInt64": {
			args: args{
				mathType: v1beta1.MathTransformTypeClampMin,
				clampMin: &two,
				i:        int64(1),
			},
			want: want{
				o: int64(2),
			},
		},
		"ClampMinNoConfig": {
			args: args{
				mathType: v1beta1.MathTransformTypeClampMin,
				i:        25,
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "clampMin",
				},
			},
		},
		"ClampMaxSuccess": {
			args: args{
				mathType: v1beta1.MathTransformTypeClampMax,
				clampMax: &two,
				i:        3,
			},
			want: want{
				o: int64(2),
			},
		},
		"ClampMaxSuccessNoChange": {
			args: args{
				mathType: v1beta1.MathTransformTypeClampMax,
				clampMax: &two,
				i:        int64(1),
			},
			want: want{
				o: int64(1),
			},
		},
		"ClampMaxSuccessInt64": {
			args: args{
				mathType: v1beta1.MathTransformTypeClampMax,
				clampMax: &two,
				i:        int64(3),
			},
			want: want{
				o: int64(2),
			},
		},
		"ClampMaxNoConfig": {
			args: args{
				mathType: v1beta1.MathTransformTypeClampMax,
				i:        25,
			},
			want: want{
				err: &field.Error{
					Type:  field.ErrorTypeRequired,
					Field: "clampMax",
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := &v1beta1.MathTransform{Type: tc.mathType, Multiply: tc.multiplier, ClampMin: tc.clampMin, ClampMax: tc.clampMax}
			got, err := ResolveMath(tr, tc.i)

			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
			fieldErr := &field.Error{}
			if err != nil && errors.As(err, &fieldErr) {
				fieldErr.Detail = ""
				fieldErr.BadValue = nil
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestStringResolve(t *testing.T) {

	type args struct {
		stype   v1beta1.StringTransformType
		fmts    *string
		convert *v1beta1.StringConversionType
		trim    *string
		regexp  *v1beta1.StringTransformRegexp
		join    *v1beta1.StringTransformJoin
		replace *v1beta1.StringTransformReplace
		i       any
	}
	type want struct {
		o   string
		err error
	}
	sFmt := "verycool%s"
	iFmt := "the largest %d"

	upper := v1beta1.StringConversionTypeToUpper
	lower := v1beta1.StringConversionTypeToLower
	tobase64 := v1beta1.StringConversionTypeToBase64
	frombase64 := v1beta1.StringConversionTypeFromBase64
	toJSON := v1beta1.StringConversionTypeToJSON
	wrongConvertType := v1beta1.StringConversionType("Something")
	toSha1 := v1beta1.StringConversionTypeToSHA1
	toSha256 := v1beta1.StringConversionTypeToSHA256
	toSha512 := v1beta1.StringConversionTypeToSHA512
	toAdler32 := v1beta1.StringConversionTypeToAdler32

	prefix := "https://"
	suffix := "-test"

	cases := map[string]struct {
		args
		want
	}{
		"NotSupportedType": {
			args: args{
				stype: "Something",
				i:     "value",
			},
			want: want{
				err: errors.Errorf(errStringTransformTypeFailed, "Something"),
			},
		},
		"FmtFailed": {
			args: args{
				stype: v1beta1.StringTransformTypeFormat,
				i:     "value",
			},
			want: want{
				err: errors.Errorf(errStringTransformTypeFormat, string(v1beta1.StringTransformTypeFormat)),
			},
		},
		"FmtString": {
			args: args{
				stype: v1beta1.StringTransformTypeFormat,
				fmts:  &sFmt,
				i:     "thing",
			},
			want: want{
				o: "verycoolthing",
			},
		},
		"FmtInteger": {
			args: args{
				stype: v1beta1.StringTransformTypeFormat,
				fmts:  &iFmt,
				i:     8,
			},
			want: want{
				o: "the largest 8",
			},
		},
		"ConvertNotSet": {
			args: args{
				stype: v1beta1.StringTransformTypeConvert,
				i:     "crossplane",
			},
			want: want{
				err: errors.Errorf(errStringTransformTypeConvert, string(v1beta1.StringTransformTypeConvert)),
			},
		},
		"ConvertTypFailed": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &wrongConvertType,
				i:       "crossplane",
			},
			want: want{
				err: errors.Errorf(errStringConvertTypeFailed, wrongConvertType),
			},
		},
		"ConvertToUpper": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &upper,
				i:       "crossplane",
			},
			want: want{
				o: "CROSSPLANE",
			},
		},
		"ConvertToLower": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &lower,
				i:       "CrossPlane",
			},
			want: want{
				o: "crossplane",
			},
		},
		"ConvertToBase64": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &tobase64,
				i:       "CrossPlane",
			},
			want: want{
				o: "Q3Jvc3NQbGFuZQ==",
			},
		},
		"ConvertFromBase64": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &frombase64,
				i:       "Q3Jvc3NQbGFuZQ==",
			},
			want: want{
				o: "CrossPlane",
			},
		},
		"ConvertFromBase64Error": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &frombase64,
				i:       "ThisStringIsNotBase64",
			},
			want: want{
				o:   "N\x18\xacJ\xda\xe2\x9e\x02,6\x8bAjǺ",
				err: errors.Wrap(errors.New("illegal base64 data at input byte 20"), errDecodeString),
			},
		},
		"ConvertToSha1": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toSha1,
				i:       "Crossplane",
			},
			want: want{
				o: "3b683dc8ff44122b331a5e4f253dd69d90726d75",
			},
		},
		"ConvertToSha1Error": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toSha1,
				i:       func() {},
			},
			want: want{
				o:   "0000000000000000000000000000000000000000",
				err: errors.Wrap(errors.Wrap(errors.New("json: unsupported type: func()"), errMarshalJSON), errHash),
			},
		},
		"ConvertToSha256": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toSha256,
				i:       "Crossplane",
			},
			want: want{
				o: "19c8a7c24ed0067f606815b59e5b82d92935ff69deed04171457a55018e31224",
			},
		},
		"ConvertToSha256Error": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toSha256,
				i:       func() {},
			},
			want: want{
				o:   "0000000000000000000000000000000000000000000000000000000000000000",
				err: errors.Wrap(errors.Wrap(errors.New("json: unsupported type: func()"), errMarshalJSON), errHash),
			},
		},
		"ConvertToSha512": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toSha512,
				i:       "Crossplane",
			},
			want: want{
				o: "0016037c62c92b5cc4a282fbe30cdd228fa001624b26fd31baa9fcb76a9c60d48e2e7a16cf8729a2d9cba3d23e1d846e7721a5381b9a92dd813178e9a6686205",
			},
		},
		"ConvertToSha512Int": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toSha512,
				i:       1234,
			},
			want: want{
				o: "d404559f602eab6fd602ac7680dacbfaadd13630335e951f097af3900e9de176b6db28512f2e000b9d04fba5133e8b1c6e8df59db3a8ab9d60be4b97cc9e81db",
			},
		},
		"ConvertToSha512IntStr": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toSha512,
				i:       "1234",
			},
			want: want{
				o: "d404559f602eab6fd602ac7680dacbfaadd13630335e951f097af3900e9de176b6db28512f2e000b9d04fba5133e8b1c6e8df59db3a8ab9d60be4b97cc9e81db",
			},
		},
		"ConvertToSha512Error": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toSha512,
				i:       func() {},
			},
			want: want{
				o:   "00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				err: errors.Wrap(errors.Wrap(errors.New("json: unsupported type: func()"), errMarshalJSON), errHash),
			},
		},
		"ConvertToAdler32": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toAdler32,
				i:       "Crossplane",
			},
			want: want{
				o: "373097499",
			},
		},
		"ConvertToAdler32Unicode": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toAdler32,
				i:       "⡌⠁⠧⠑ ⠼⠁⠒  ⡍⠜⠇⠑⠹⠰⠎ ⡣⠕⠌",
			},
			want: want{
				o: "4110427190",
			},
		},
		"ConvertToAdler32Error": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toAdler32,
				i:       func() {},
			},
			want: want{
				o:   "0",
				err: errors.Wrap(errors.Wrap(errors.New("json: unsupported type: func()"), errMarshalJSON), errAdler),
			},
		},
		"TrimPrefix": {
			args: args{
				stype: v1beta1.StringTransformTypeTrimPrefix,
				trim:  &prefix,
				i:     "https://crossplane.io",
			},
			want: want{
				o: "crossplane.io",
			},
		},
		"TrimSuffix": {
			args: args{
				stype: v1beta1.StringTransformTypeTrimSuffix,
				trim:  &suffix,
				i:     "my-string-test",
			},
			want: want{
				o: "my-string",
			},
		},
		"TrimPrefixWithoutMatch": {
			args: args{
				stype: v1beta1.StringTransformTypeTrimPrefix,
				trim:  &prefix,
				i:     "crossplane.io",
			},
			want: want{
				o: "crossplane.io",
			},
		},
		"TrimSuffixWithoutMatch": {
			args: args{
				stype: v1beta1.StringTransformTypeTrimSuffix,
				trim:  &suffix,
				i:     "my-string",
			},
			want: want{
				o: "my-string",
			},
		},
		"RegexpNotCompiling": {
			args: args{
				stype: v1beta1.StringTransformTypeRegexp,
				regexp: &v1beta1.StringTransformRegexp{
					Match: "[a-z",
				},
				i: "my-string",
			},
			want: want{
				err: errors.Wrap(errors.New("error parsing regexp: missing closing ]: `[a-z`"), errStringTransformTypeRegexpFailed),
			},
		},
		"RegexpSimpleMatch": {
			args: args{
				stype: v1beta1.StringTransformTypeRegexp,
				regexp: &v1beta1.StringTransformRegexp{
					Match: "[0-9]",
				},
				i: "my-1-string",
			},
			want: want{
				o: "1",
			},
		},
		"RegexpCaptureGroup": {
			args: args{
				stype: v1beta1.StringTransformTypeRegexp,
				regexp: &v1beta1.StringTransformRegexp{
					Match: "my-([0-9]+)-string",
					Group: ptr.To[int](1),
				},
				i: "my-1-string",
			},
			want: want{
				o: "1",
			},
		},
		"RegexpNoSuchCaptureGroup": {
			args: args{
				stype: v1beta1.StringTransformTypeRegexp,
				regexp: &v1beta1.StringTransformRegexp{
					Match: "my-([0-9]+)-string",
					Group: ptr.To[int](2),
				},
				i: "my-1-string",
			},
			want: want{
				err: errors.Errorf(errStringTransformTypeRegexpNoMatch, "my-([0-9]+)-string", 2),
			},
		},
		"ConvertToJSONSuccess": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toJSON,
				i: map[string]any{
					"foo": "bar",
				},
			},
			want: want{
				o: "{\"foo\":\"bar\"}",
			},
		},
		"ConvertToJSONFail": {
			args: args{
				stype:   v1beta1.StringTransformTypeConvert,
				convert: &toJSON,
				i: map[string]any{
					"foo": func() {},
				},
			},
			want: want{
				o:   "",
				err: errors.Wrap(errors.New("json: unsupported type: func()"), errMarshalJSON),
			},
		},
		"JoinString": {
			args: args{
				stype: v1beta1.StringTransformTypeJoin,
				join: &v1beta1.StringTransformJoin{
					Separator: ",",
				},
				i: []interface{}{"cross", "plane"},
			},
			want: want{
				o: "cross,plane",
			},
		},
		"JoinStringEmptySeparator": {
			args: args{
				stype: v1beta1.StringTransformTypeJoin,
				join: &v1beta1.StringTransformJoin{
					Separator: "",
				},
				i: []interface{}{"cross", "plane"},
			},
			want: want{
				o: "crossplane",
			},
		},
		"JoinStringDifferentTypes": {
			args: args{
				stype: v1beta1.StringTransformTypeJoin,
				join: &v1beta1.StringTransformJoin{
					Separator: "-",
				},
				i: []interface{}{"cross", "plane", 42},
			},
			want: want{
				o: "cross-plane-42",
			},
		},
		"ReplaceFound": {
			args: args{
				stype: v1beta1.StringTransformTypeReplace,
				replace: &v1beta1.StringTransformReplace{
					Search:  "Cr",
					Replace: "B",
				},
				i: "Crossplane",
			},
			want: want{
				o: "Bossplane",
			},
		},
		"ReplaceNotFound": {
			args: args{
				stype: v1beta1.StringTransformTypeReplace,
				replace: &v1beta1.StringTransformReplace{
					Search:  "xx",
					Replace: "zz",
				},
				i: "Crossplane",
			},
			want: want{
				o: "Crossplane",
			},
		},
		"ReplaceRemove": {
			args: args{
				stype: v1beta1.StringTransformTypeReplace,
				replace: &v1beta1.StringTransformReplace{
					Search:  "ss",
					Replace: "",
				},
				i: "Crossplane",
			},
			want: want{
				o: "Croplane",
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			tr := &v1beta1.StringTransform{Type: tc.stype,
				Format:  tc.fmts,
				Convert: tc.convert,
				Trim:    tc.trim,
				Regexp:  tc.regexp,
				Join:    tc.join,
				Replace: tc.replace,
			}

			got, err := ResolveString(tr, tc.i)

			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestConvertResolve(t *testing.T) {
	type args struct {
		to     v1beta1.TransformIOType
		format *v1beta1.ConvertTransformFormat
		i      any
	}
	type want struct {
		o   any
		err error
	}

	cases := map[string]struct {
		args
		want
	}{
		"StringToBool": {
			args: args{
				i:  "true",
				to: v1beta1.TransformIOTypeBool,
			},
			want: want{
				o: true,
			},
		},
		"StringToFloat64": {
			args: args{
				i:  "1000",
				to: v1beta1.TransformIOTypeFloat64,
			},
			want: want{
				o: 1000.0,
			},
		},
		"StringToQuantityFloat64": {
			args: args{
				i:      "1000m",
				to:     v1beta1.TransformIOTypeFloat64,
				format: (*v1beta1.ConvertTransformFormat)(ptr.To[string](string(v1beta1.ConvertTransformFormatQuantity))),
			},
			want: want{
				o: 1.0,
			},
		},
		"StringToQuantityFloat64InvalidFormat": {
			args: args{
				i:      "1000 blabla",
				to:     v1beta1.TransformIOTypeFloat64,
				format: (*v1beta1.ConvertTransformFormat)(ptr.To[string](string(v1beta1.ConvertTransformFormatQuantity))),
			},
			want: want{
				err: resource.ErrFormatWrong,
			},
		},
		"SameTypeNoOp": {
			args: args{
				i:  true,
				to: v1beta1.TransformIOTypeBool,
			},
			want: want{
				o: true,
			},
		},
		"IntAliasToInt64": {
			args: args{
				i:  int64(1),
				to: v1beta1.TransformIOTypeInt,
			},
			want: want{
				o: int64(1),
			},
		},
		"StringToObject": {
			args: args{
				i:      "{\"foo\":\"bar\"}",
				to:     v1beta1.TransformIOTypeObject,
				format: (*v1beta1.ConvertTransformFormat)(ptr.To[string](string(v1beta1.ConvertTransformFormatJSON))),
			},
			want: want{
				o: map[string]any{
					"foo": "bar",
				},
			},
		},
		"StringToList": {
			args: args{
				i:      "[\"foo\", \"bar\", \"baz\"]",
				to:     v1beta1.TransformIOTypeArray,
				format: (*v1beta1.ConvertTransformFormat)(ptr.To[string](string(v1beta1.ConvertTransformFormatJSON))),
			},
			want: want{
				o: []any{
					"foo", "bar", "baz",
				},
			},
		},
		"InputTypeNotSupported": {
			args: args{
				i:  []int{64},
				to: v1beta1.TransformIOTypeString,
			},
			want: want{
				err: errors.Errorf(errFmtConvertInputTypeNotSupported, []int{}),
			},
		},
		"ConversionPairFormatNotSupported": {
			args: args{
				i:      100,
				to:     v1beta1.TransformIOTypeString,
				format: (*v1beta1.ConvertTransformFormat)(ptr.To[string](string(v1beta1.ConvertTransformFormatQuantity))),
			},
			want: want{
				err: errors.Errorf(errFmtConvertFormatPairNotSupported, "int", "string", string(v1beta1.ConvertTransformFormatQuantity)),
			},
		},
		"ConversionPairNotSupported": {
			args: args{
				i:  "[64]",
				to: "[]int",
			},
			want: want{
				err: &field.Error{
					Type:     field.ErrorTypeInvalid,
					Field:    "toType",
					BadValue: v1beta1.TransformIOType("[]int"),
					Detail:   "invalid type",
				},
			},
		},
		"ConversionPairSupportedFloat64Int64": {
			args: args{
				i:  float64(1.1),
				to: v1beta1.TransformIOTypeInt64,
			},
			want: want{
				o: int64(1),
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := &v1beta1.ConvertTransform{ToType: tc.args.to, Format: tc.format}
			got, err := ResolveConvert(tr, tc.i)

			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Resolve(b): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestConvertTransformGetConversionFunc(t *testing.T) {
	type args struct {
		ct   *v1beta1.ConvertTransform
		from v1beta1.TransformIOType
	}
	type want struct {
		err error
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"IntToString": {
			reason: "Int to String should be valid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeString,
				},
				from: v1beta1.TransformIOTypeInt,
			},
		},
		"IntToInt": {
			reason: "Int to Int should be valid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeInt,
				},
				from: v1beta1.TransformIOTypeInt,
			},
		},
		"IntToInt64": {
			reason: "Int to Int64 should be valid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeInt,
				},
				from: v1beta1.TransformIOTypeInt64,
			},
		},
		"Int64ToInt": {
			reason: "Int64 to Int should be valid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeInt64,
				},
				from: v1beta1.TransformIOTypeInt,
			},
		},
		"IntToFloat": {
			reason: "Int to Float should be valid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeInt,
				},
				from: v1beta1.TransformIOTypeFloat64,
			},
		},
		"IntToBool": {
			reason: "Int to Bool should be valid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeInt,
				},
				from: v1beta1.TransformIOTypeBool,
			},
		},
		"JSONStringToObject": {
			reason: "JSON string to Object should be valid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeObject,
					Format: &[]v1beta1.ConvertTransformFormat{v1beta1.ConvertTransformFormatJSON}[0],
				},
				from: v1beta1.TransformIOTypeString,
			},
		},
		"JSONStringToArray": {
			reason: "JSON string to Array should be valid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeArray,
					Format: &[]v1beta1.ConvertTransformFormat{v1beta1.ConvertTransformFormatJSON}[0],
				},
				from: v1beta1.TransformIOTypeString,
			},
		},
		"StringToObjectMissingFormat": {
			reason: "String to Object without format should be invalid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeObject,
				},
				from: v1beta1.TransformIOTypeString,
			},
			want: want{
				err: fmt.Errorf("conversion from string to object is not supported with format none"),
			},
		},
		"StringToIntInvalidFormat": {
			reason: "String to Int with invalid format should be invalid",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeInt,
					Format: &[]v1beta1.ConvertTransformFormat{"wrong"}[0],
				},
				from: v1beta1.TransformIOTypeString,
			},
			want: want{
				err: fmt.Errorf("conversion from string to int64 is not supported with format wrong"),
			},
		},
		"IntToIntInvalidFormat": {
			reason: "Int to Int, invalid format ignored because it is the same type",
			args: args{
				ct: &v1beta1.ConvertTransform{
					ToType: v1beta1.TransformIOTypeInt,
					Format: &[]v1beta1.ConvertTransformFormat{"wrong"}[0],
				},
				from: v1beta1.TransformIOTypeInt,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := GetConversionFunc(tc.args.ct, tc.args.from)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("%s\nGetConversionFunc(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
