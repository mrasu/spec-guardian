package gawssdkgov2

import (
	"context"
	"errors"

	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/mrasu/spec-guardian/guardian"
)

var _ smithyhttp.BeforeExecutionInterceptor = (*Interceptor)(nil)

// Interceptor injects faults before logical AWS SDK for Go v2 API calls.
type Interceptor struct {
	fault guardian.Fault
}

// NewInterceptor returns an AWS SDK for Go v2 interceptor controlled by fault.
// It panics if fault is nil.
func NewInterceptor(fault guardian.Fault) *Interceptor {
	if fault == nil {
		panic("gawssdkgov2: fault must not be nil")
	}
	return &Interceptor{fault: fault}
}

// BeforeExecution records one logical API call before the SDK enters its retry loop.
func (i *Interceptor) BeforeExecution(ctx context.Context, _ *smithyhttp.InterceptorContext) error {
	operation := middleware.GetOperationName(ctx)
	if operation == "" {
		return errors.New("gawssdkgov2: AWS operation name is missing from context")
	}
	return i.fault.BeforeIO(ctx, "aws."+operation)
}
