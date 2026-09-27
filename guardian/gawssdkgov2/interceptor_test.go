package gawssdkgov2

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errInjected = errors.New("injected")

type recordingFault struct {
	operations []string
	err        error
}

func (f *recordingFault) BeforeIO(_ context.Context, operation string) error {
	f.operations = append(f.operations, operation)
	return f.err
}

func TestInterceptorRecordsOperation(t *testing.T) {
	fault := &recordingFault{}
	interceptor := NewInterceptor(fault)
	ctx := middleware.WithOperationName(t.Context(), "PutItem")

	require.NoError(t, interceptor.BeforeExecution(ctx, &smithyhttp.InterceptorContext{}))
	assert.Equal(t, []string{"aws.PutItem"}, fault.operations)
}

func TestInterceptorReturnsFault(t *testing.T) {
	fault := &recordingFault{err: errInjected}
	interceptor := NewInterceptor(fault)
	ctx := middleware.WithOperationName(t.Context(), "PutItem")

	err := interceptor.BeforeExecution(ctx, &smithyhttp.InterceptorContext{})
	assert.ErrorIs(t, err, errInjected)
	assert.Equal(t, []string{"aws.PutItem"}, fault.operations)
}

func TestNewInterceptorRejectsNilFault(t *testing.T) {
	assert.PanicsWithValue(t, "gawssdkgov2: fault must not be nil", func() {
		NewInterceptor(nil)
	})
}

func TestInterceptorRejectsMissingOperationName(t *testing.T) {
	fault := &recordingFault{}
	interceptor := NewInterceptor(fault)

	err := interceptor.BeforeExecution(t.Context(), &smithyhttp.InterceptorContext{})
	assert.EqualError(t, err, "gawssdkgov2: AWS operation name is missing from context")
	assert.Empty(t, fault.operations)
}
