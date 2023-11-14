package v1beta1

// ConditionSpec defines the condition for rendering.
// Conditions are defined using the Common Expression Language
// For more information refer to https://github.com/google/cel-spec
type ConditionSpec struct {
	// Expression is the CEL expression to be evaluated. If the Expression
	// returns a true value, the function will render the resources
	Expression string `json:"expression"`
}
