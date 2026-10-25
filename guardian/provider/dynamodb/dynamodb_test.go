package dynamodb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestProvider_ObserveItemListIncludesKeysAndAllPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ExclusiveStartKey map[string]any
			ConsistentRead    bool
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.True(t, request.ConsistentRead)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if len(request.ExclusiveStartKey) == 0 {
			fmt.Fprint(w, `{"Items":[{"id":{"S":"delivery-1"},"order_id":{"N":"42"}}],"LastEvaluatedKey":{"id":{"S":"delivery-1"}}}`)
		} else {
			require.Equal(t, map[string]any{"id": map[string]any{"S": "delivery-1"}}, request.ExclusiveStartKey)
			fmt.Fprint(w, `{"Items":[{"id":{"S":"unexpected-delivery"},"order_id":{"N":"43"}}]}`)
		}
	}))
	defer server.Close()
	observer, err := NewProvider(Config{}, testProviderConfig(server.URL, "deliveries"), []provider.FieldRequest{{
		Field:  "deliveries",
		Config: map[string]any{"table": "deliveries", "result": map[string]any{"type": "item_list"}},
		Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindList, Values: &provider.Type{
			Kind: provider.KindMessage, Fields: map[string]provider.Type{
				"id": {Kind: provider.KindString}, "order_id": {Kind: provider.KindInt64},
			},
		}}},
	}})
	require.NoError(t, err)

	got, err := observer.Observe(t.Context(), "deliveries")

	require.NoError(t, err)
	require.ElementsMatch(t, []map[string]any{
		{"id": "delivery-1", "order_id": int64(42)},
		{"id": "unexpected-delivery", "order_id": int64(43)},
	}, got)
}

func TestProvider_ObserveItemListHandlesEmptyAndInvalidData(t *testing.T) {
	tests := []struct {
		name     string
		response string
		status   int
		wantErr  string
	}{
		{name: "empty table", response: `{"Items":[]}`, status: http.StatusOK},
		{name: "missing key", response: `{"Items":[{"order_id":{"N":"42"}}]}`, status: http.StatusOK, wantErr: "key attribute"},
		{name: "extra attribute", response: `{"Items":[{"id":{"S":"d"},"order_id":{"N":"42"},"extra":{"S":"x"}}]}`, status: http.StatusOK, wantErr: "extra attribute"},
		{name: "invalid value", response: `{"Items":[{"id":{"S":"d"},"order_id":{"N":"bad"}}]}`, status: http.StatusOK, wantErr: "invalid int64"},
		{name: "duplicate key", response: `{"Items":[{"id":{"S":"d"},"order_id":{"N":"42"}},{"id":{"S":"d"},"order_id":{"N":"43"}}]}`, status: http.StatusOK, wantErr: "duplicate key"},
		{name: "scan failure", response: `{"__type":"ResourceNotFoundException","message":"missing table"}`, status: http.StatusBadRequest, wantErr: "missing table"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-amz-json-1.0")
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.response)
			}))
			defer server.Close()
			observer, err := NewProvider(Config{}, testProviderConfig(server.URL, "deliveries"), []provider.FieldRequest{{
				Field:  "deliveries",
				Config: map[string]any{"table": "deliveries", "result": map[string]any{"type": "item_list"}},
				Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindList, Values: &provider.Type{
					Kind: provider.KindMessage, Fields: map[string]provider.Type{
						"id": {Kind: provider.KindString}, "order_id": {Kind: provider.KindInt64},
					},
				}}},
			}})
			require.NoError(t, err)

			got, err := observer.Observe(t.Context(), "deliveries")

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []map[string]any{}, got)
		})
	}
}

func seedTestField(name, table, result string) provider.FieldRequest {
	fields := map[string]provider.Type{"value": {Kind: provider.KindInt64, Nullable: true}}
	kind := provider.KindMap
	if result == "item_list" {
		kind = provider.KindList
		fields["id"] = provider.Type{Kind: provider.KindString}
	}
	return provider.FieldRequest{
		Field: name, Config: map[string]any{"table": table, "result": map[string]any{"type": result}},
		Output: provider.OutputSpec{Type: provider.Type{Kind: kind, Values: &provider.Type{Kind: provider.KindMessage, Fields: fields}}},
	}
}

func testProviderConfig(endpoint string, names ...string) map[string]any {
	tables := make([]map[string]any, 0, len(names))
	seen := make(map[string]bool)
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		tables = append(tables, map[string]any{
			"name":                  name,
			"attribute_definitions": []map[string]any{{"attribute_name": "id", "attribute_type": "S"}},
			"key_schema":            []map[string]any{{"attribute_name": "id", "key_type": "HASH"}},
		})
	}
	return map[string]any{"endpoint": endpoint, "region": "us-east-1", "access_key_id": "test", "secret_access_key": "test", "tables": tables}
}

func newSeedTestProvider(t *testing.T, server *httptest.Server, fields ...provider.FieldRequest) *Provider {
	t.Helper()
	names := make([]string, len(fields))
	for i, field := range fields {
		names[i] = field.Config["table"].(string)
	}
	p, err := NewProvider(Config{}, testProviderConfig(server.URL, names...), fields)
	require.NoError(t, err)
	return p
}

func TestProvider_SeedPreservesModelValuesAndRequestOrder(t *testing.T) {
	var writes []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		writes = append(writes, body)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	p := newSeedTestProvider(t, server, seedTestField("map", "map_table", "item_map"), seedTestField("list", "list_table", "item_list"))
	type row struct {
		ID    string `json:"id"`
		Value *int64 `json:"value"`
	}
	number := int64(42)
	request := provider.SeedRequest{Values: []provider.SeedValue{
		{Field: "list", Value: []row{{ID: "second", Value: &number}, {ID: "first"}}},
		{Field: "map", Value: map[string]map[string]any{"request": {"value": int64(-3)}}},
	}}

	result, err := p.Seed(t.Context(), request)

	require.NoError(t, err)
	require.Equal(t, request.Values, result.Values)
	require.Equal(t, []map[string]any{
		{"TableName": "list_table", "Item": map[string]any{"id": map[string]any{"S": "second"}, "value": map[string]any{"N": "42"}}},
		{"TableName": "list_table", "Item": map[string]any{"id": map[string]any{"S": "first"}, "value": map[string]any{"NULL": true}}},
		{"TableName": "map_table", "Item": map[string]any{"id": map[string]any{"S": "request"}, "value": map[string]any{"N": "-3"}}},
	}, writes)
}

func TestProvider_SeedRejectsInvalidRequestsBeforeWriting(t *testing.T) {
	tests := []struct {
		name   string
		values []provider.SeedValue
		want   string
	}{
		{name: "missing field", want: "requires all"},
		{name: "unknown field", values: []provider.SeedValue{{Field: "unknown"}}, want: "unknown or duplicate"},
		{name: "duplicate key", values: []provider.SeedValue{{Field: "rows", Value: []map[string]any{{"id": "a", "value": nil}, {"id": "a", "value": nil}}}}, want: "duplicate DynamoDB key"},
		{name: "empty key", values: []provider.SeedValue{{Field: "rows", Value: []map[string]any{{"id": "", "value": nil}}}}, want: "empty or duplicate"},
		{name: "wrong scalar", values: []provider.SeedValue{{Field: "rows", Value: []map[string]any{{"id": "a", "value": "42"}}}}, want: "want int64"},
		{name: "missing attribute", values: []provider.SeedValue{{Field: "rows", Value: []map[string]any{{"id": "a"}}}}, want: "row fields"},
		{name: "extra attribute", values: []provider.SeedValue{{Field: "rows", Value: []map[string]any{{"id": "a", "value": nil, "extra": "x"}}}}, want: "row fields"},
		{name: "null list", values: []provider.SeedValue{{Field: "rows", Value: nil}}, want: "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid seed wrote data") }))
			defer server.Close()
			p := newSeedTestProvider(t, server, seedTestField("rows", "table", "item_list"))
			result, err := p.Seed(t.Context(), provider.SeedRequest{Values: tt.values})
			require.ErrorContains(t, err, tt.want)
			require.Equal(t, provider.SeedResult{}, result)
		})
	}
}

func TestProvider_CleanupAttemptDeletesAllPagesWithoutDecodingValues(t *testing.T) {
	var deleted []map[string]any
	scans := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".Scan") {
			scans++
			if body["ExclusiveStartKey"] == nil {
				fmt.Fprint(w, `{"Items":[{"id":{"S":"old"},"invalid":{"N":"1"}}],"LastEvaluatedKey":{"id":{"S":"old"}}}`)
			} else {
				if scans != 2 {
					t.Error("unexpected extra scan")
				}
				fmt.Fprint(w, `{"Items":[{"id":{"S":"unexpected"}}]}`)
			}
		} else {
			if scans != 2 {
				t.Error("deleted before finishing scan")
			}
			deleted = append(deleted, body)
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	p := newSeedTestProvider(t, server, seedTestField("rows", "table", "item_list"))
	require.NoError(t, p.PrepareAttempt(t.Context()))
	require.Zero(t, scans)
	require.Empty(t, deleted)
	require.NoError(t, p.CleanupAttempt(t.Context()))
	require.Equal(t, []map[string]any{
		{"TableName": "table", "Key": map[string]any{"id": map[string]any{"S": "old"}}},
		{"TableName": "table", "Key": map[string]any{"id": map[string]any{"S": "unexpected"}}},
	}, deleted)
}

func TestProvider_CleanupAttemptRemovesPartialSeedAfterPutFailure(t *testing.T) {
	writes := 0
	var deleted []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		switch {
		case strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".PutItem"):
			writes++
			if writes == 2 {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"__type":"ValidationException","message":"put failed"}`)
				return
			}
			fmt.Fprint(w, `{}`)
		case strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".Scan"):
			fmt.Fprint(w, `{"Items":[{"id":{"S":"a"}}]}`)
		default:
			deleted = append(deleted, body)
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	p := newSeedTestProvider(t, server, seedTestField("rows", "table", "item_list"))
	result, err := p.Seed(t.Context(), provider.SeedRequest{Values: []provider.SeedValue{
		{Field: "rows", Value: []map[string]any{{"id": "a", "value": nil}, {"id": "b", "value": nil}}},
	}})
	require.ErrorContains(t, err, "put failed")
	require.Equal(t, provider.SeedResult{}, result)
	require.NoError(t, p.CleanupAttempt(t.Context()))
	require.Equal(t, []map[string]any{{"TableName": "table", "Key": map[string]any{"id": map[string]any{"S": "a"}}}}, deleted)
}

func TestNewProvider_RejectsSharedSeedTable(t *testing.T) {
	result, err := NewProvider(Config{}, testProviderConfig("unused", "same"), []provider.FieldRequest{
		seedTestField("a", "same", "item_map"), seedTestField("b", "same", "item_list"),
	})
	require.ErrorContains(t, err, "reference the same table")
	require.Nil(t, result)
}

func TestProvider_CleanupAttemptContinuesAfterScanOrDeleteFailure(t *testing.T) {
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			TableName string
			Key       map[string]struct{ S string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".Scan") {
			if body.TableName == "broken" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"__type":"ResourceNotFoundException","message":"scan failed"}`)
				return
			}
			fmt.Fprint(w, `{"Items":[{"id":{"S":"a"}},{"id":{"S":"b"}}]}`)
		} else {
			deleted = append(deleted, body.Key["id"].S)
			if body.Key["id"].S == "a" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"__type":"ValidationException","message":"delete failed"}`)
				return
			}
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	p := newSeedTestProvider(t, server, seedTestField("broken", "broken", "item_map"), seedTestField("rows", "table", "item_list"))
	err := p.CleanupAttempt(t.Context())
	require.ErrorContains(t, err, "scan failed")
	require.ErrorContains(t, err, "delete failed")
	require.Equal(t, []string{"a", "b"}, deleted)
}

func TestProvider_SeedRequiresEachAssignedFieldOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid seed wrote data") }))
	defer server.Close()
	p := newSeedTestProvider(t, server, seedTestField("a", "a", "item_map"), seedTestField("b", "b", "item_map"))
	result, err := p.Seed(t.Context(), provider.SeedRequest{Values: []provider.SeedValue{
		{Field: "a", Value: map[string]map[string]any{}},
		{Field: "a", Value: map[string]map[string]any{}},
	}})
	require.ErrorContains(t, err, "unknown or duplicate DynamoDB seed field")
	require.Equal(t, provider.SeedResult{}, result)
}
