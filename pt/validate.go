package pt

import (
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
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
	if len(r.Resources) == 0 && (r.Environment == nil || len(r.Environment.Patches) == 0) {
		return field.Required(field.NewPath("resources"), "resources or environment patches are required")
	}
	for i, r := range r.Resources {
		if err := ValidateComposedTemplate(r); err != nil {
			return WrapFieldError(err, field.NewPath("resources").Index(i))
		}
	}
	if err := ValidateEnvironment(r.Environment); err != nil {
		return WrapFieldError(err, field.NewPath("environment"))
	}
	return nil
}

// ValidateComposedTemplate validates a ComposedTemplate.
func ValidateComposedTemplate(t v1beta1.ComposedTemplate) *field.Error {
	if t.Name == "" {
		return field.Required(field.NewPath("name"), "name is required")
	}
	for i, p := range t.Patches {
		p := p
		if err := ValidatePatch(&p); err != nil {
			return WrapFieldError(err, field.NewPath("patches").Index(i))
		}
	}
	for i, cd := range t.ConnectionDetails {
		if err := ValidateConnectionDetail(cd); err != nil {
			return WrapFieldError(err, field.NewPath("connectionDetails").Index(i))
		}
	}
	for i, rc := range t.ReadinessChecks {
		if err := ValidateReadinessCheck(rc); err != nil {
			return WrapFieldError(err, field.NewPath("readinessChecks").Index(i))
		}
	}
	return nil
}

// ValidatePatchSet validates a PatchSet.
func ValidatePatchSet(ps v1beta1.PatchSet) *field.Error {
	if ps.Name == "" {
		return field.Required(field.NewPath("name"), "name is required")
	}
	for i, p := range ps.Patches {
		p := p
		if err := ValidatePatch(&p); err != nil {
			return WrapFieldError(err, field.NewPath("patches").Index(i))
		}
	}
	return nil
}

// ValidateEnvironment validates (patches to and from) the Environment.
func ValidateEnvironment(e *v1beta1.Environment) *field.Error {
	if e == nil {
		return nil
	}
	for i, p := range e.Patches {
		p := p
		switch p.GetType() { //nolint:exhaustive // Only target valid patches according the API spec
		case
			v1beta1.PatchTypeFromCompositeFieldPath,
			v1beta1.PatchTypeToCompositeFieldPath,
			v1beta1.PatchTypeCombineFromComposite,
			v1beta1.PatchTypeCombineToComposite,
			v1beta1.PatchTypeFromEnvironmentFieldPath,
			v1beta1.PatchTypeToEnvironmentFieldPath:
		default:
			return field.Invalid(field.NewPath("patches").Index(i).Key("type"), p.GetType(), "invalid environment patch type")
		}

		if err := ValidatePatch(&p); err != nil {
			return WrapFieldError(err, field.NewPath("patches").Index(i))
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

// ValidatePatch validates a ComposedPatch.
func ValidatePatch(p PatchInterface) *field.Error { //nolint: gocyclo // This is a long but simple/same-y switch.
	switch p.GetType() {
	case v1beta1.PatchTypeFromCompositeFieldPath,
		v1beta1.PatchTypeToCompositeFieldPath,
		v1beta1.PatchTypeFromEnvironmentFieldPath,
		v1beta1.PatchTypeToEnvironmentFieldPath:
		if p.GetFromFieldPath() == "" {
			return field.Required(field.NewPath("fromFieldPath"), fmt.Sprintf("fromFieldPath must be set for patch type %s", p.GetType()))
		}
	case v1beta1.PatchTypePatchSet:
		ps, ok := p.(PatchWithPatchSetName)
		if !ok {
			return field.Invalid(field.NewPath("type"), p.GetType(), fmt.Sprintf("patch type %T does not support patch of type %s", p, p.GetType()))
		}
		if ps.GetPatchSetName() == "" {
			return field.Required(field.NewPath("patchSetName"), fmt.Sprintf("patchSetName must be set for patch type %s", p.GetType()))
		}
	case v1beta1.PatchTypeCombineFromComposite,
		v1beta1.PatchTypeCombineToComposite,
		v1beta1.PatchTypeCombineFromEnvironment,
		v1beta1.PatchTypeCombineToEnvironment:
		if p.GetCombine() == nil {
			return field.Required(field.NewPath("combine"), fmt.Sprintf("combine must be set for patch type %s", p.GetType()))
		}
		if p.GetToFieldPath() == "" {
			return field.Required(field.NewPath("toFieldPath"), fmt.Sprintf("toFieldPath must be set for patch type %s", p.GetType()))
		}
		return WrapFieldError(ValidateCombine(p.GetCombine()), field.NewPath("combine"))
	default:
		// Should never happen
		return field.Invalid(field.NewPath("type"), p.GetType(), "unknown patch type")
	}
	for i, t := range p.GetTransforms() {
		if err := ValidateTransform(t); err != nil {
			return WrapFieldError(err, field.NewPath("transforms").Index(i))
		}
	}
	if pp := p.GetPolicy(); pp != nil {
		switch pp.GetToFieldPathPolicy() {
		case v1beta1.ToFieldPathPolicyReplace,
			v1beta1.ToFieldPathPolicyMergeObjects,
			v1beta1.ToFieldPathPolicyMergeObjectsAppendArrays,
			v1beta1.ToFieldPathPolicyForceMergeObjects,
			v1beta1.ToFieldPathPolicyForceMergeObjectsAppendArrays,
			v1beta1.ToFieldPathPolicyMergeObject, //nolint:staticcheck // MergeObject is deprecated but we must still support it.
			v1beta1.ToFieldPathPolicyAppendArray: //nolint:staticcheck // AppendArray is deprecated but we must still support it.
			// ok
		default:
			return field.Invalid(field.NewPath("policy", "toFieldPathPolicy"), pp.GetToFieldPathPolicy(), "unknown toFieldPathPolicy")
		}
		switch pp.GetFromFieldPathPolicy() {
		case v1beta1.FromFieldPathPolicyRequired,
			v1beta1.FromFieldPathPolicyOptional:
			// ok
		default:
			return field.Invalid(field.NewPath("policy", "fromFieldPathPolicy"), pp.GetFromFieldPathPolicy(), "unknown fromFieldPathPolicy")
		}
	}
	return nil
}

// ValidateCombine validates a Combine.
func ValidateCombine(c *v1beta1.Combine) *field.Error {
	switch c.Strategy {
	case v1beta1.CombineStrategyString:
		if c.String == nil {
			return field.Required(field.NewPath("string"), fmt.Sprintf("string must be set for combine strategy %s", c.Strategy))
		}
	case "":
		return field.Required(field.NewPath("strategy"), "a combine strategy must be provided")
	default:
		return field.Invalid(field.NewPath("strategy"), c.Strategy, "unknown strategy type")
	}

	if len(c.Variables) == 0 {
		return field.Required(field.NewPath("variables"), "at least one variable must be provided")
	}

	for i := range c.Variables {
		if c.Variables[i].FromFieldPath == "" {
			return field.Required(field.NewPath("variables").Index(i).Child("fromFieldPath"), "fromFieldPath must be set for each combine variable")
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
	if m.Type == "" {
		return field.Required(field.NewPath("type"), "math transform type is required")
	}
	switch m.Type {
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
	if s.Type == "" {
		return field.Required(field.NewPath("type"), "string transform type is required")
	}
	switch s.Type {
	case v1beta1.StringTransformTypeFormat:
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
	case v1beta1.StringTransformTypeJoin:
		if s.Join == nil {
			return field.Required(field.NewPath("join"), "join transform requires a join")
		}
	case v1beta1.StringTransformTypeReplace:
		if s.Replace == nil {
			return field.Required(field.NewPath("replace"), "replace transform requires a replace")
		}
		if s.Replace.Search == "" {
			return field.Required(field.NewPath("replace", "search"), "replace transform requires a search")
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
	if cd.Type == "" {
		return field.Required(field.NewPath("type"), "connection detail type is required")
	}
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
