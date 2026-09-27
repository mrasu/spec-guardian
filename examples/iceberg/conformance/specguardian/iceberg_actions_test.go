//go:build specguardian

package specguardian_test

import (
	"context"

	icebergexample "spec-guardian/examples/iceberg"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go/table"
	iceutils "github.com/apache/iceberg-go/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go/transport/http"
	"github.com/mrasu/spec-guardian/guardian"
	"github.com/mrasu/spec-guardian/guardian/gawssdkgov2"
)

// IcebergActions executes table actions with fault injection.
type IcebergActions struct {
	environment  *IcebergTableEnvironment
	actionConfig aws.Config
}

// NewIcebergActions binds an environment and controller to the actions.
func NewIcebergActions(environment *IcebergTableEnvironment, controller *guardian.InjectionController) *IcebergActions {
	if environment == nil || controller == nil {
		panic("IcebergActions requires an environment and injection controller")
	}
	actionConfig := environment.adminConfig.Copy()
	actionConfig.Retryer = func() aws.Retryer {
		return aws.NopRetryer{}
	}
	interceptors := http.InterceptorRegistry{}
	interceptors.AddBeforeExecution(gawssdkgov2.NewInterceptor(controller))
	actionConfig.Interceptors = interceptors

	return &IcebergActions{
		environment:  environment,
		actionConfig: actionConfig,
	}
}

func (a *IcebergActions) Append(ctx context.Context, input IcebergTableAppendInput) (IcebergTableAppendOutput, error) {
	reader, err := newActionRecords(a.environment.table, []icebergRow{input.Row})
	if err != nil {
		return IcebergTableAppendOutput{}, err
	}
	defer reader.Release()

	wrapped := icebergexample.NewIcebergGoTable(a.environment.table)
	result, err := wrapped.Append(iceutils.WithAwsConfig(ctx, &a.actionConfig), reader, nil)
	if err != nil {
		return IcebergTableAppendOutput{}, err
	}
	return IcebergTableAppendOutput{Table: result}, nil
}

// Delete executes an Iceberg delete with fault injection enabled.
func (a *IcebergActions) Delete(ctx context.Context, input IcebergTableDeleteInput) (IcebergTableDeleteOutput, error) {
	wrapped := icebergexample.NewIcebergGoTable(a.environment.table)
	result, err := wrapped.Delete(iceutils.WithAwsConfig(ctx, &a.actionConfig), input.Predicate, nil)
	if err != nil {
		return IcebergTableDeleteOutput{}, err
	}
	return IcebergTableDeleteOutput{Table: result}, nil
}

// Overwrite executes an Iceberg overwrite with fault injection enabled.
func (a *IcebergActions) Overwrite(ctx context.Context, input IcebergTableOverwriteInput) (IcebergTableOverwriteOutput, error) {
	reader, err := newActionRecords(a.environment.table, input.Rows)
	if err != nil {
		return IcebergTableOverwriteOutput{}, err
	}
	defer reader.Release()

	wrapped := icebergexample.NewIcebergGoTable(a.environment.table)
	result, err := wrapped.Overwrite(iceutils.WithAwsConfig(ctx, &a.actionConfig), reader, nil, table.WithOverwriteFilter(input.Predicate))
	if err != nil {
		return IcebergTableOverwriteOutput{}, err
	}
	return IcebergTableOverwriteOutput{Table: result}, nil
}

func newActionRecords(icebergTable *table.Table, rows []icebergRow) (array.RecordReader, error) {
	schema, err := table.SchemaToArrowSchema(icebergTable.Schema(), nil, true, false)
	if err != nil {
		return nil, err
	}
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()
	for _, row := range rows {
		for column, value := range row {
			builder.Field(column).(*array.StringBuilder).Append(value)
		}
	}
	batch := builder.NewRecordBatch()
	defer batch.Release()
	return array.NewRecordReader(schema, []arrow.RecordBatch{batch})
}
