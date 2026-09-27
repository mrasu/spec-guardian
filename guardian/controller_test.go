package guardian

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControllerImplementsFault(t *testing.T) {
	var _ Fault = NewInjectionController("test")
}

type customHook struct {
	fault Fault
}

func (h customHook) call(ctx context.Context, operation string) error {
	return h.fault.BeforeIO(ctx, operation)
}

func TestCustomHookUsesControllerCounter(t *testing.T) {
	controller := NewInjectionController("test")
	hook := customHook{fault: controller}
	endAttempt := controller.InjectFaultAt(2)
	defer endAttempt()

	assert.NoError(t, hook.call(t.Context(), "custom.first"))
	assert.ErrorIs(t, hook.call(t.Context(), "custom.second"), ErrInjected)

	want := []IOEvent{
		{Number: 1, Operation: "custom.first"},
		{Number: 2, Operation: "custom.second", Injected: true},
	}
	assert.Equal(t, want, controller.Events())
}

func TestAttemptsStopsAfterSuccessPath(t *testing.T) {
	controller := NewInjectionController("test")
	var attempts []int

	for faultNumber := range controller.Attempts() {
		attempts = append(attempts, faultNumber)
		endAttempt := controller.InjectFaultAt(faultNumber)
		for range 2 {
			_ = controller.BeforeIO(t.Context(), "test.IO")
		}
		endAttempt()
	}

	want := []int{1, 2, 3}
	assert.Equal(t, want, attempts)
}

func TestBeforeIOOnlyObservesEnabledAttempt(t *testing.T) {
	controller := NewInjectionController("test")
	assert.NoError(t, controller.BeforeIO(t.Context(), "before"))

	endAttempt := controller.InjectFaultAt(2)
	assert.NoError(t, controller.BeforeIO(t.Context(), "first"))
	assert.ErrorIs(t, controller.BeforeIO(t.Context(), "second"), ErrInjected)
	assert.NoError(t, controller.BeforeIO(t.Context(), "fallback"))
	endAttempt()
	assert.NoError(t, controller.BeforeIO(t.Context(), "after"))

	want := []IOEvent{
		{Number: 1, Operation: "first"},
		{Number: 2, Operation: "second", Injected: true},
		{Number: 3, Operation: "fallback"},
	}
	assert.Equal(t, want, controller.Events())
}

func TestControllerOptions(t *testing.T) {
	injectedError := errors.New("custom")
	controller := NewInjectionController("test", WithMaxFaultNumber(1), WithInjectedError(injectedError))

	var attempts int
	for faultNumber := range controller.Attempts() {
		attempts++
		endAttempt := controller.InjectFaultAt(faultNumber)
		assert.ErrorIs(t, controller.BeforeIO(context.Background(), "test.IO"), injectedError)
		endAttempt()
	}
	assert.Equal(t, 1, attempts)
}

func TestAttemptsWarnings(t *testing.T) {
	tests := []struct {
		name        string
		controller  *InjectionController
		invoke      func(*InjectionController, int)
		wantMessage string
	}{
		{
			name:       "no I/O",
			controller: NewInjectionController("empty"),
			invoke: func(_ *InjectionController, _ int) {
			},
			wantMessage: "Action executed no supported I/O injection points",
		},
		{
			name:       "limit",
			controller: NewInjectionController("limited", WithMaxFaultNumber(1)),
			invoke: func(controller *InjectionController, _ int) {
				_ = controller.BeforeIO(context.Background(), "test.IO")
			},
			wantMessage: "fault injection limit reached",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })

			for faultNumber := range test.controller.Attempts() {
				endAttempt := test.controller.InjectFaultAt(faultNumber)
				test.invoke(test.controller, faultNumber)
				endAttempt()
			}
			assert.Contains(t, logs.String(), test.wantMessage)
		})
	}
}

func TestEventsReturnsCopy(t *testing.T) {
	controller := NewInjectionController("test")
	endAttempt := controller.InjectFaultAt(2)
	_ = controller.BeforeIO(t.Context(), "original")
	endAttempt()

	events := controller.Events()
	require.Len(t, events, 1)
	events[0].Operation = "changed"
	assert.Equal(t, "original", controller.Events()[0].Operation)
}
