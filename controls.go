package pix

import (
	"cmp"
	"errors"
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
	if co.OnChange != nil {
		if err := co.OnChange(v); err != nil {
			return err
		}
	}
	co.Value = v
	return nil
}

// IntControl is implemented by controls with integer values: integers, booleans (0 or 1) and
// enumerations. Its methods do not box values, so UIs and serializers can read and change
// controls without allocating.
type IntControl interface {
	Control
	// IntValue returns the value, its inclusive limits and the step a UI should change it by.
	IntValue() (v, min, max, step int64)
	// SetInt changes the value. Values outside the limits are rejected.
	SetInt(v int64) error
}

var (
	_ IntControl = (*ControlInt)(nil)
	_ IntControl = (*ControlBool)(nil)
)

// ControlInt is an integer control. The value is changed only if OnChange, when set, returns
// no error. Step is a hint for UIs: SetInt accepts any value within limits.
type ControlInt struct {
	Name        string
	Description string
	Value       int64
	Min, Max    int64
	Step        int64
	OnChange    func(v int64) error
}

func (c *ControlInt) Describe() (name, description string) { return c.Name, c.Description }
func (c *ControlInt) ActualValue() any                     { return c.Value }
func (c *ControlInt) IntValue() (v, lo, hi, step int64) {
	return c.Value, c.Min, c.Max, max(c.Step, 1)
}
func (c *ControlInt) ChangeValue(newValue any) error {
	switch v := newValue.(type) {
	case int64:
		return c.SetInt(v)
	case int:
		return c.SetInt(int64(v))
	}
	return fmt.Errorf("new value %T not an integer", newValue)
}
func (c *ControlInt) SetInt(v int64) error {
	if v < c.Min || v > c.Max {
		return errOutOfLimits
	}
	if c.OnChange != nil {
		if err := c.OnChange(v); err != nil {
			return err
		}
	}
	c.Value = v
	return nil
}

// ControlBool is a boolean control, also usable as an [IntControl] with values 0 and 1.
type ControlBool struct {
	Name        string
	Description string
	Value       bool
	OnChange    func(v bool) error
}

func (c *ControlBool) Describe() (name, description string) { return c.Name, c.Description }
func (c *ControlBool) ActualValue() any                     { return c.Value }
func (c *ControlBool) IntValue() (v, min, max, step int64) {
	if c.Value {
		v = 1
	}
	return v, 0, 1, 1
}
func (c *ControlBool) ChangeValue(newValue any) error {
	v, ok := newValue.(bool)
	if !ok {
		return fmt.Errorf("new value %T not bool", newValue)
	}
	return c.set(v)
}
func (c *ControlBool) SetInt(v int64) error {
	if v != 0 && v != 1 {
		return errOutOfLimits
	}
	return c.set(v == 1)
}
func (c *ControlBool) set(v bool) error {
	if c.OnChange != nil {
		if err := c.OnChange(v); err != nil {
			return err
		}
	}
	c.Value = v
	return nil
}

var errOutOfLimits = errors.New("value out of control limits")

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
	if ce.OnChange != nil {
		if err := ce.OnChange(v); err != nil {
			return err
		}
	}
	ce.Value = v
	return nil
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
	if cc.OnChange != nil {
		if err := cc.OnChange(pts); err != nil {
			return err
		}
	}
	cc.Points = pts
	return nil
}
