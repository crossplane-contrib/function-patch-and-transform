package v1beta1

// ConditionSpec defines the condition for rendering.
// It uses expr
type ConditionSpec struct {
	Expr string `json:"expr"`
}
