package pix

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/soypat/geometry/ms2"
)

// Control represents an editable parameter of a filter.
// When Value is modified via OnChange, the filter updates its output immediately.
type Control interface {
	// Display/human readable name and description.
	Describe() (name, description string)
	// ActualValue returns the current value of the control.
	ActualValue() any
	// ChangeValue attempts to update the ActualValue to newValue.
	ChangeValue(newValue any) error
}

type ControlOrdered[T cmp.Ordered] struct {
	Name        string
	Description string
	Value       T
	Min         T
	Max         T
	Step        T
	OnChange    func(T) error
}

func (co *ControlOrdered[T]) Describe() (name, description string) {
	return co.Name, co.Description
}
func (co *ControlOrdered[T]) ActualValue() any { return co.Value }
func (co *ControlOrdered[T]) ChangeValue(newValue any) error {
	v, ok := newValue.(T)
	if !ok {
		return fmt.Errorf("new value %T not of type %T", newValue, co.Value)
	}
	if v < co.Min || v > co.Max {
		return fmt.Errorf("new value %v exceeds limits %v..%v", v, co.Min, co.Max)
	}
	err := co.OnChange(v)
	if err == nil {
		co.Value = v
	}
	return err
}

type integer interface {
	~int | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~int8 | ~int16 | ~int32 | ~int64
}

// enum best generated with stringer commands.
type enum interface {
	integer
	fmt.Stringer
}

// ControlEnum maps to dropdown kind of list.
type ControlEnum[T enum] struct {
	Name        string
	Description string
	Value       T
	ValidValues []T
	OnChange    func(T) error
}

func (ce *ControlEnum[T]) Describe() (name, description string) {
	return ce.Name, ce.Description
}
func (ce *ControlEnum[T]) ActualValue() any {
	return ce.Value
}
func (ce *ControlEnum[T]) ChangeValue(newValue any) error {
	v, ok := newValue.(T)
	if !ok {
		return fmt.Errorf("new value %T not of type %T", newValue, ce.Value)
	}
	if !slices.Contains(ce.ValidValues, v) {
		return fmt.Errorf("value %v of %T not valid", v, v)
	}
	err := ce.OnChange(v)
	if err == nil {
		ce.Value = v
	}
	return err
}

// CurvePoint is a control point for curve-type controls.
// X represents input (0-1), Y represents output (0-1).
type CurvePoint = ms2.Vec

// ControlCurve is a spline curve control with editable control points.
// Points are in normalized 0-1 range for both X (input) and Y (output).
type ControlCurve struct {
	Name        string
	Description string
	Points      []CurvePoint // Control points, X/Y in 0-1 range.
	OnChange    func([]CurvePoint) error
}

func (cc *ControlCurve) Describe() (name, description string) {
	return cc.Name, cc.Description
}

func (cc *ControlCurve) ActualValue() any {
	return cc.Points
}

func (cc *ControlCurve) ChangeValue(newValue any) error {
	pts, ok := newValue.([]CurvePoint)
	if !ok {
		return fmt.Errorf("new value %T not of type []CurvePoint", newValue)
	}
	err := cc.OnChange(pts)
	if err == nil {
		cc.Points = pts
	}
	return err
}
