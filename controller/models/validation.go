package models

import "fmt"

const (
	maxFieldErrorValueLength = 20
)

type FieldError struct {
	Reason     string
	FieldName  string
	FieldValue interface{}
}

func (fe FieldError) Error() string {
	v := fe.FieldValue
	if s, ok := fe.FieldValue.(string); ok {
		if len(s) > maxFieldErrorValueLength {
			v = s[0:maxFieldErrorValueLength] + "..."
		}
	}
	return fmt.Sprintf("the value '%v' for '%s' is invalid: %s", v, fe.FieldName, fe.Reason)
}

func NewFieldError(reason, name string, value interface{}) *FieldError {
	return &FieldError{
		Reason:     reason,
		FieldName:  name,
		FieldValue: value,
	}
}
