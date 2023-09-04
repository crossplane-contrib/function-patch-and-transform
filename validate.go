package main

import (
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/negz/function-patch-and-transform/input/v1beta1"
)

// WrapFieldError wraps the given field.Error adding the given field.Path as root of the Field.
func WrapFieldError(err *field.Error, path *field.Path) *field.Error {
	if err == nil {
		return nil
	}
	if path == nil {
		return err
	}
	err.Field = path.Child(err.Field).String()
	return err
}

// WrapFieldErrorList wraps the given field.ErrorList adding the given field.Path as root of the Field.
func WrapFieldErrorList(errs field.ErrorList, path *field.Path) field.ErrorList {
	if path == nil {
		return errs
	}
	for i := range errs {
		errs[i] = WrapFieldError(errs[i], path)
	}
	return errs
}

// ValidateResources validates the Resources object.
func ValidateResources(r *v1beta1.Resources) *field.Error {
	for _, ps := range r.PatchSets {
		if err := ValidatePatchSet(ps); err != nil {
			return err
		}
	}
	if len(r.Resources) == 0 {
		return field.Required(field.NewPath("resources"), "resources is required")
	}

	for _, r := range r.Resources {
		if err := ValidateComposedTemplate(r); err != nil {
			return field.Invalid(field.NewPath("resources"), r, "invalid resource")
		}
	}
	return nil
}

// ValidateComposedTemplate validates a ComposedTemplate.
func ValidateComposedTemplate(t v1beta1.ComposedTemplate) *field.Error {
	if t.Name == "" {
		return field.Required(field.NewPath("name"), "name is required")
	}
	for _, p := range t.Patches {
		if err := ValidatePatch(p); err != nil {
			return field.Invalid(field.NewPath("patches"), t.Patches, "invalid patches")
		}
	}
	for _, cd := range t.ConnectionDetails {
		if err := ValidateConnectionDetail(cd); err != nil {
			return field.Invalid(field.NewPath("connectionDetails"), t.Patches, "invalid connection details")
		}
	}
	for _, rc := range t.ReadinessChecks {
		if err := ValidateReadinessCheck(rc); err != nil {
			return field.Invalid(field.NewPath("readinessChecks"), t.Patches, "invalid readiness checks")
		}
	}
	return nil
}

// ValidatePatchSet validates a PatchSet.
func ValidatePatchSet(ps v1beta1.PatchSet) *field.Error {
	if ps.Name == "" {
		return field.Required(field.NewPath("name"), "name is required")
	}
	for _, p := range ps.Patches {
		if err := ValidatePatch(p); err != nil {
			return err
		}
	}
	return nil
}

// ValidateReadinessCheck checks if the readiness check is logically valid.
func ValidateReadinessCheck(r v1beta1.ReadinessCheck) *field.Error { //nolint:gocyclo // This function is not that complex, just a switch
	if !r.Type.IsValid() {
		return field.Invalid(field.NewPath("type"), string(r.Type), "unknown readiness check type")
	}
	switch r.Type {
	case v1beta1.ReadinessCheckTypeNone:
		return nil
	case v1beta1.ReadinessCheckTypeMatchString:
		if r.MatchString == nil {
			return field.Required(field.NewPath("matchString"), "cannot be nil for type MatchString")
		}
	case v1beta1.ReadinessCheckTypeMatchInteger:
		if r.MatchInteger == nil {
			return field.Required(field.NewPath("matchInteger"), "cannot be nil for type MatchInteger")
		}
	case v1beta1.ReadinessCheckTypeMatchCondition:
		if err := ValidateMatchConditionReadinessCheck(r.MatchCondition); err != nil {
			return WrapFieldError(err, field.NewPath("matchCondition"))
		}
		return nil
	case v1beta1.ReadinessCheckTypeNonEmpty, v1beta1.ReadinessCheckTypeMatchFalse, v1beta1.ReadinessCheckTypeMatchTrue:
		// No specific validation required.
	}
	if r.FieldPath == nil {
		return field.Required(field.NewPath("fieldPath"), "cannot be empty")
	}

	return nil
}

// ValidateMatchConditionReadinessCheck checks if the match condition is
// logically valid.
func ValidateMatchConditionReadinessCheck(m *v1beta1.MatchConditionReadinessCheck) *field.Error {
	if m == nil {
		return nil
	}
	if m.Type == "" {
		return field.Required(field.NewPath("type"), "cannot be empty for type MatchCondition")
	}
	if m.Status == "" {
		return field.Required(field.NewPath("status"), "cannot be empty for type MatchCondition")
	}
	return nil
}

// ValidatePatch validates a Patch.
func ValidatePatch(p v1beta1.Patch) *field.Error {
	switch p.GetType() {
	case v1beta1.PatchTypeFromCompositeFieldPath, v1beta1.PatchTypeToCompositeFieldPath:
		if p.FromFieldPath == nil {
			return field.Required(field.NewPath("fromFieldPath"), fmt.Sprintf("fromFieldPath must be set for patch type %s", p.Type))
		}
	case v1beta1.PatchTypePatchSet:
		if p.PatchSetName == nil {
			return field.Required(field.NewPath("patchSetName"), fmt.Sprintf("patchSetName must be set for patch type %s", p.Type))
		}
	case v1beta1.PatchTypeCombineFromComposite, v1beta1.PatchTypeCombineToComposite:
		if p.Combine == nil {
			return field.Required(field.NewPath("combine"), fmt.Sprintf("combine must be set for patch type %s", p.Type))
		}
		if p.ToFieldPath == nil {
			return field.Required(field.NewPath("toFieldPath"), fmt.Sprintf("toFieldPath must be set for patch type %s", p.Type))
		}
	default:
		// Should never happen
		return field.Invalid(field.NewPath("type"), p.Type, "unknown patch type")
	}
	for i, t := range p.Transforms {
		if err := ValidateTransform(t); err != nil {
			return WrapFieldError(err, field.NewPath("transforms").Index(i))
		}
	}

	return nil
}

// ValidateTransform validates a Transform.
func ValidateTransform(t v1beta1.Transform) *field.Error { //nolint:gocyclo // This is a long but simple/same-y switch.
	switch t.Type {
	case v1beta1.TransformTypeMath:
		if t.Math == nil {
			return field.Required(field.NewPath("math"), "given transform type math requires configuration")
		}
		return WrapFieldError(ValidateMathTransform(t.Math), field.NewPath("math"))
	case v1beta1.TransformTypeMap:
		if t.Map == nil {
			return field.Required(field.NewPath("map"), "given transform type map requires configuration")
		}
		return WrapFieldError(ValidateMapTransform(t.Map), field.NewPath("map"))
	case v1beta1.TransformTypeMatch:
		if t.Match == nil {
			return field.Required(field.NewPath("match"), "given transform type match requires configuration")
		}
		return WrapFieldError(ValidateMatchTransform(t.Match), field.NewPath("match"))
	case v1beta1.TransformTypeString:
		if t.String == nil {
			return field.Required(field.NewPath("string"), "given transform type string requires configuration")
		}
		return WrapFieldError(ValidateStringTransform(t.String), field.NewPath("string"))
	case v1beta1.TransformTypeConvert:
		if t.Convert == nil {
			return field.Required(field.NewPath("convert"), "given transform type convert requires configuration")
		}
		if err := ValidateConvertTransform(t.Convert); err != nil {
			return WrapFieldError(err, field.NewPath("convert"))
		}
	default:
		// Should never happen
		return field.Invalid(field.NewPath("type"), t.Type, "unknown transform type")
	}

	return nil
}

// ValidateMathTransform validates a MathTransform.
func ValidateMathTransform(m *v1beta1.MathTransform) *field.Error {
	switch m.GetType() {
	case v1beta1.MathTransformTypeMultiply:
		if m.Multiply == nil {
			return field.Required(field.NewPath("multiply"), "must specify a value if a multiply math transform is specified")
		}
	case v1beta1.MathTransformTypeClampMin:
		if m.ClampMin == nil {
			return field.Required(field.NewPath("clampMin"), "must specify a value if a clamp min math transform is specified")
		}
	case v1beta1.MathTransformTypeClampMax:
		if m.ClampMax == nil {
			return field.Required(field.NewPath("clampMax"), "must specify a value if a clamp max math transform is specified")
		}
	default:
		return field.Invalid(field.NewPath("type"), m.Type, "unknown math transform type")
	}
	return nil
}

// ValidateMapTransform validates MapTransform.
func ValidateMapTransform(m *v1beta1.MapTransform) *field.Error {
	if len(m.Pairs) == 0 {
		return field.Required(field.NewPath("pairs"), "at least one pair must be specified if a map transform is specified")
	}
	return nil
}

// ValidateMatchTransform validates a MatchTransform.
func ValidateMatchTransform(m *v1beta1.MatchTransform) *field.Error {
	if len(m.Patterns) == 0 {
		return field.Required(field.NewPath("patterns"), "at least one pattern must be specified if a match transform is specified")
	}
	for i, p := range m.Patterns {
		if err := ValidateMatchTransformPattern(p); err != nil {
			return WrapFieldError(err, field.NewPath("patterns").Index(i))
		}
	}
	return nil
}

// ValidateMatchTransformPattern validates a MatchTransformPattern.
func ValidateMatchTransformPattern(p v1beta1.MatchTransformPattern) *field.Error {
	switch p.Type {
	case v1beta1.MatchTransformPatternTypeLiteral, "":
		if p.Literal == nil {
			return field.Required(field.NewPath("literal"), "literal pattern type requires a literal")
		}
	case v1beta1.MatchTransformPatternTypeRegexp:
		if p.Regexp == nil {
			return field.Required(field.NewPath("regexp"), "regexp pattern type requires a regexp")
		}
		if _, err := regexp.Compile(*p.Regexp); err != nil {
			return field.Invalid(field.NewPath("regexp"), *p.Regexp, "invalid regexp")
		}
	default:
		return field.Invalid(field.NewPath("type"), p.Type, "unknown pattern type")
	}
	return nil
}

// ValidateStringTransform validates a StringTransform.
func ValidateStringTransform(s *v1beta1.StringTransform) *field.Error { //nolint:gocyclo // just a switch
	switch s.Type {
	case v1beta1.StringTransformTypeFormat, "":
		if s.Format == nil {
			return field.Required(field.NewPath("fmt"), "format transform requires a format")
		}
	case v1beta1.StringTransformTypeConvert:
		if s.Convert == nil {
			return field.Required(field.NewPath("convert"), "convert transform requires a conversion type")
		}
	case v1beta1.StringTransformTypeTrimPrefix, v1beta1.StringTransformTypeTrimSuffix:
		if s.Trim == nil {
			return field.Required(field.NewPath("trim"), "trim transform requires a trim value")
		}
	case v1beta1.StringTransformTypeRegexp:
		if s.Regexp == nil {
			return field.Required(field.NewPath("regexp"), "regexp transform requires a regexp")
		}
		if s.Regexp.Match == "" {
			return field.Required(field.NewPath("regexp", "match"), "regexp transform requires a match")
		}
		if _, err := regexp.Compile(s.Regexp.Match); err != nil {
			return field.Invalid(field.NewPath("regexp", "match"), s.Regexp.Match, "invalid regexp")
		}
	default:
		return field.Invalid(field.NewPath("type"), s.Type, "unknown string transform type")
	}
	return nil
}

// ValidateConvertTransform validates a ConvertTransform.
func ValidateConvertTransform(t *v1beta1.ConvertTransform) *field.Error {
	if !t.GetFormat().IsValid() {
		return field.Invalid(field.NewPath("format"), t.Format, "invalid format")
	}
	if !t.ToType.IsValid() {
		return field.Invalid(field.NewPath("toType"), t.ToType, "invalid type")
	}
	return nil
}

// ValidateConnectionDetail checks if the connection detail is logically valid.
func ValidateConnectionDetail(cd v1beta1.ConnectionDetail) *field.Error {
	if !cd.Type.IsValid() {
		return field.Invalid(field.NewPath("type"), string(cd.Type), "unknown connection detail type")
	}
	if cd.Name == "" {
		return field.Required(field.NewPath("name"), "name is required")
	}
	switch cd.Type {
	case v1beta1.ConnectionDetailTypeFromValue:
		if cd.Value == nil {
			return field.Required(field.NewPath("value"), "value connection detail requires a value")
		}
	case v1beta1.ConnectionDetailTypeFromConnectionSecretKey:
		if cd.FromConnectionSecretKey == nil {
			return field.Required(field.NewPath("fromConnectionSecretKey"), "from connection secret key connection detail requires a key")
		}
	case v1beta1.ConnectionDetailTypeFromFieldPath:
		if cd.FromFieldPath == nil {
			return field.Required(field.NewPath("fromFieldPath"), "from field path connection detail requires a field path")
		}
	}
	return nil
}
