package gvalkeygo

import (
	"context"
	"time"

	"github.com/mrasu/spec-guardian/guardian"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeyhook"
)

var _ valkeyhook.Hook = (*hook)(nil)

// NewHook returns a Valkey hook controlled by fault.
func NewHook(fault guardian.Fault) valkeyhook.Hook {
	return &hook{fault: fault}
}

type hook struct {
	fault guardian.Fault
}

func (h *hook) Do(client valkey.Client, ctx context.Context, command valkey.Completed) valkey.ValkeyResult {
	if err := h.fault.BeforeIO(ctx, "valkey.Do"); err != nil {
		return valkeyhook.NewErrorResult(err)
	}
	return client.Do(ctx, command)
}

func (h *hook) DoMulti(client valkey.Client, ctx context.Context, commands ...valkey.Completed) []valkey.ValkeyResult {
	if err := h.fault.BeforeIO(ctx, "valkey.DoMulti"); err != nil {
		results := make([]valkey.ValkeyResult, len(commands))
		for index := range results {
			results[index] = valkeyhook.NewErrorResult(err)
		}
		return results
	}
	return client.DoMulti(ctx, commands...)
}

func (h *hook) DoCache(client valkey.Client, ctx context.Context, command valkey.Cacheable, ttl time.Duration) valkey.ValkeyResult {
	if err := h.fault.BeforeIO(ctx, "valkey.DoCache"); err != nil {
		return valkeyhook.NewErrorResult(err)
	}
	return client.DoCache(ctx, command, ttl)
}

func (h *hook) DoMultiCache(client valkey.Client, ctx context.Context, commands ...valkey.CacheableTTL) []valkey.ValkeyResult {
	if err := h.fault.BeforeIO(ctx, "valkey.DoMultiCache"); err != nil {
		results := make([]valkey.ValkeyResult, len(commands))
		for index := range results {
			results[index] = valkeyhook.NewErrorResult(err)
		}
		return results
	}
	return client.DoMultiCache(ctx, commands...)
}

func (h *hook) Receive(client valkey.Client, ctx context.Context, subscribe valkey.Completed, fn func(valkey.PubSubMessage)) error {
	if err := h.fault.BeforeIO(ctx, "valkey.Receive"); err != nil {
		return err
	}
	return client.Receive(ctx, subscribe, fn)
}

func (h *hook) DoStream(client valkey.Client, ctx context.Context, command valkey.Completed) valkey.ValkeyResultStream {
	if err := h.fault.BeforeIO(ctx, "valkey.DoStream"); err != nil {
		return valkeyhook.NewErrorResultStream(err)
	}
	return client.DoStream(ctx, command)
}

func (h *hook) DoMultiStream(client valkey.Client, ctx context.Context, commands ...valkey.Completed) valkey.MultiValkeyResultStream {
	if err := h.fault.BeforeIO(ctx, "valkey.DoMultiStream"); err != nil {
		return valkeyhook.NewErrorResultStream(err)
	}
	return client.DoMultiStream(ctx, commands...)
}
