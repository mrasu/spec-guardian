package guardian

import (
	"context"
	"errors"
	"iter"
	"log/slog"
)

const defaultMaxFaultNumber = 100

// ErrInjected is the default error returned at the selected I/O boundary.
var ErrInjected = errors.New("spec guardian: injected fault")

// Fault is implemented by a fault-injection controller used at an I/O boundary.
type Fault interface {
	BeforeIO(context.Context, string) error
}

// IOEvent describes an I/O boundary observed during an attempt.
type IOEvent struct {
	Number    int
	Operation string
	Injected  bool
}

// Option configures a InjectionController.
type Option func(*InjectionController)

// WithMaxFaultNumber sets the maximum fault position attempted by a InjectionController.
// It panics if maxFaultNumber is less than one.
func WithMaxFaultNumber(maxFaultNumber int) Option {
	if maxFaultNumber < 1 {
		panic("guardian: maximum fault number must be positive")
	}
	return func(controller *InjectionController) {
		controller.maxFaultNumber = maxFaultNumber
	}
}

// WithInjectedError sets the error returned when a fault is injected.
// It panics if injectedError is nil.
func WithInjectedError(injectedError error) Option {
	if injectedError == nil {
		panic("guardian: injected error must not be nil")
	}
	return func(controller *InjectionController) {
		controller.injectedError = injectedError
	}
}

// InjectionController injects at most one fault into one Action invocation at a time.
//
// A InjectionController is not safe for concurrent use. In particular, the Action under
// test must not perform supported I/O concurrently.
type InjectionController struct {
	action         string
	maxFaultNumber int
	injectedError  error
	enabled        bool
	target         int
	count          int
	injected       bool
	events         []IOEvent
}

// NewInjectionController returns a disabled InjectionController for action.
func NewInjectionController(action string, options ...Option) *InjectionController {
	controller := &InjectionController{
		action:         action,
		maxFaultNumber: defaultMaxFaultNumber,
		injectedError:  ErrInjected,
	}
	for _, option := range options {
		option(controller)
	}
	return controller
}

// Attempts yields fault positions until an invocation completes without an
// injected fault or the configured maximum fault number is reached.
func (c *InjectionController) Attempts() iter.Seq[int] {
	return func(yield func(int) bool) {
		for faultNumber := 1; faultNumber <= c.maxFaultNumber; faultNumber++ {
			if !yield(faultNumber) {
				return
			}
			if !c.injected {
				if c.count == 0 {
					slog.Warn("Action executed no supported I/O injection points; only the success path was checked", "action", c.action)
				}
				return
			}
		}
		slog.Warn("fault injection limit reached; stopping fault iteration", "action", c.action, "max_fault_number", c.maxFaultNumber)
	}
}

// InjectFaultAt enables injection for a fresh Action invocation at faultNumber.
// The returned function ends the attempt and must be deferred around only the
// Action invocation so that setup and state observation are not counted.
func (c *InjectionController) InjectFaultAt(faultNumber int) func() {
	c.enabled = true
	c.target = faultNumber
	c.count = 0
	c.injected = false
	c.events = c.events[:0]

	return func() {
		c.enabled = false
	}
}

// BeforeIO records an enabled I/O boundary and injects the configured error at
// the selected boundary. Calls made while the InjectionController is disabled are ignored.
func (c *InjectionController) BeforeIO(_ context.Context, operation string) error {
	if !c.enabled {
		return nil
	}

	c.count++
	event := IOEvent{
		Number:    c.count,
		Operation: operation,
		Injected:  c.count == c.target,
	}
	c.events = append(c.events, event)
	if !event.Injected {
		return nil
	}

	c.injected = true
	return c.injectedError
}

// Events returns a copy of the I/O events observed in the current attempt.
func (c *InjectionController) Events() []IOEvent {
	return append([]IOEvent(nil), c.events...)
}
