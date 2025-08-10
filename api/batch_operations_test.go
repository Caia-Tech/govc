package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchExecutor_Creation(t *testing.T) {
	executor := NewBatchExecutor()
	assert.NotNil(t, executor)
	assert.Equal(t, 1000, executor.maxBatchSize)
	assert.Equal(t, 100, executor.maxParallel)
	assert.Equal(t, 30*time.Second, executor.timeout)
	assert.True(t, executor.enableTransaction)
}

func TestBatchExecutor_Validate(t *testing.T) {
	executor := NewBatchExecutor()

	tests := []struct {
		name    string
		request *BatchRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid batch request",
			request: &BatchRequest{
				Operations: []BatchOperation{
					{ID: "1", Type: OpRead, Params: json.RawMessage(`{}`)},
					{ID: "2", Type: OpWrite, Params: json.RawMessage(`{}`)},
				},
			},
			wantErr: false,
		},
		{
			name: "Empty batch request",
			request: &BatchRequest{
				Operations: []BatchOperation{},
			},
			wantErr: true,
			errMsg:  "batch request must contain at least one operation",
		},
		{
			name: "Missing operation ID",
			request: &BatchRequest{
				Operations: []BatchOperation{
					{Type: OpRead, Params: json.RawMessage(`{}`)},
				},
			},
			wantErr: true,
			errMsg:  "operation 0 missing ID",
		},
		{
			name: "Missing operation type",
			request: &BatchRequest{
				Operations: []BatchOperation{
					{ID: "1", Params: json.RawMessage(`{}`)},
				},
			},
			wantErr: true,
			errMsg:  "operation 0 missing type",
		},
		{
			name: "Transaction and parallel conflict",
			request: &BatchRequest{
				Operations: []BatchOperation{
					{ID: "1", Type: OpRead, Params: json.RawMessage(`{}`)},
				},
				Transaction: true,
				Parallel:    true,
			},
			wantErr: true,
			errMsg:  "cannot execute transactional batch in parallel",
		},
		{
			name: "Exceeds max batch size",
			request: &BatchRequest{
				Operations: make([]BatchOperation, 1001),
			},
			wantErr: true,
			errMsg:  "batch size 1001 exceeds maximum 1000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.Validate(tt.request)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBatchExecutor_Execute_Empty(t *testing.T) {
	executor := NewBatchExecutor()
	
	response, err := executor.Execute(&BatchRequest{
		Operations: []BatchOperation{},
	})
	
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Empty(t, response.Results)
	assert.Equal(t, 0, response.Succeeded)
	assert.Equal(t, 0, response.Failed)
}

func TestBatchExecutor_Execute_Sequential(t *testing.T) {
	executor := NewBatchExecutor()
	
	request := &BatchRequest{
		Operations: []BatchOperation{
			{ID: "op1", Type: OpRead, Params: json.RawMessage(`{"path": "file1.txt"}`)},
			{ID: "op2", Type: OpWrite, Params: json.RawMessage(`{"content": "test"}`)},
			{ID: "op3", Type: OpQuery, Params: json.RawMessage(`{"pattern": "*.txt"}`)},
		},
		Parallel: false,
	}
	
	response, err := executor.Execute(request)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Results, 3)
	
	// Check that all operations have results
	for i, result := range response.Results {
		assert.Equal(t, request.Operations[i].ID, result.ID)
		assert.True(t, result.Success) // These are placeholder implementations
	}
	
	assert.Equal(t, 3, response.Succeeded)
	assert.Equal(t, 0, response.Failed)
}

func TestBatchExecutor_Execute_Parallel(t *testing.T) {
	executor := NewBatchExecutor()
	
	// Create a larger batch to test parallelism
	ops := make([]BatchOperation, 50)
	for i := 0; i < 50; i++ {
		ops[i] = BatchOperation{
			ID:     string(rune('A' + i)),
			Type:   OpRead,
			Params: json.RawMessage(`{}`),
		}
	}
	
	request := &BatchRequest{
		Operations: ops,
		Parallel:   true,
	}
	
	start := time.Now()
	response, err := executor.Execute(request)
	duration := time.Since(start)
	
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Results, 50)
	
	// Verify all operations completed
	assert.Equal(t, 50, response.Succeeded)
	assert.Equal(t, 0, response.Failed)
	
	// Parallel execution should be faster than sequential
	// (though with placeholder implementations, the difference might be minimal)
	t.Logf("Parallel execution of 50 operations took: %v", duration)
}

func TestBatchExecutor_Execute_UnsupportedOperation(t *testing.T) {
	executor := NewBatchExecutor()
	
	request := &BatchRequest{
		Operations: []BatchOperation{
			{ID: "op1", Type: "unsupported_op", Params: json.RawMessage(`{}`)},
		},
	}
	
	response, err := executor.Execute(request)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Results, 1)
	
	result := response.Results[0]
	assert.Equal(t, "op1", result.ID)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "unsupported operation type")
	
	assert.Equal(t, 0, response.Succeeded)
	assert.Equal(t, 1, response.Failed)
}

func TestBatchExecutor_Execute_MixedResults(t *testing.T) {
	executor := NewBatchExecutor()
	
	request := &BatchRequest{
		Operations: []BatchOperation{
			{ID: "op1", Type: OpRead, Params: json.RawMessage(`{}`)},
			{ID: "op2", Type: "invalid", Params: json.RawMessage(`{}`)},
			{ID: "op3", Type: OpWrite, Params: json.RawMessage(`{}`)},
			{ID: "op4", Type: "another_invalid", Params: json.RawMessage(`{}`)},
			{ID: "op5", Type: OpQuery, Params: json.RawMessage(`{}`)},
		},
	}
	
	response, err := executor.Execute(request)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Results, 5)
	
	// Check mixed results
	assert.Equal(t, 3, response.Succeeded) // op1, op3, op5
	assert.Equal(t, 2, response.Failed)    // op2, op4
	
	// Verify specific results
	assert.True(t, response.Results[0].Success)  // op1
	assert.False(t, response.Results[1].Success) // op2
	assert.True(t, response.Results[2].Success)  // op3
	assert.False(t, response.Results[3].Success) // op4
	assert.True(t, response.Results[4].Success)  // op5
}

func TestBatchExecutor_Execute_Transaction(t *testing.T) {
	executor := NewBatchExecutor()
	
	request := &BatchRequest{
		Operations: []BatchOperation{
			{ID: "op1", Type: OpWrite, Params: json.RawMessage(`{"content": "file1"}`)},
			{ID: "op2", Type: OpWrite, Params: json.RawMessage(`{"content": "file2"}`)},
			{ID: "op3", Type: OpCommit, Params: json.RawMessage(`{"message": "batch commit"}`)},
		},
		Transaction: true,
	}
	
	response, err := executor.Execute(request)
	require.NoError(t, err)
	assert.NotNil(t, response)
	
	// Currently returns "not implemented" for all ops in transaction mode
	// This will be updated when transaction support is added
	for _, result := range response.Results {
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "transactional batch not yet implemented")
	}
}

func BenchmarkBatchExecutor_Sequential(b *testing.B) {
	executor := NewBatchExecutor()
	
	ops := make([]BatchOperation, 100)
	for i := 0; i < 100; i++ {
		ops[i] = BatchOperation{
			ID:     string(rune(i)),
			Type:   OpRead,
			Params: json.RawMessage(`{}`),
		}
	}
	
	request := &BatchRequest{
		Operations: ops,
		Parallel:   false,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.Execute(request)
	}
}

func BenchmarkBatchExecutor_Parallel(b *testing.B) {
	executor := NewBatchExecutor()
	
	ops := make([]BatchOperation, 100)
	for i := 0; i < 100; i++ {
		ops[i] = BatchOperation{
			ID:     string(rune(i)),
			Type:   OpRead,
			Params: json.RawMessage(`{}`),
		}
	}
	
	request := &BatchRequest{
		Operations: ops,
		Parallel:   true,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.Execute(request)
	}
}