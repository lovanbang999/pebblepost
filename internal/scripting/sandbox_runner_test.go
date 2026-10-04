package scripting

import (
	"testing"
	"time"

	"pebblepost/internal/types"
)

func TestEngine_RunnerFlowControlAndData(t *testing.T) {
	engine := NewEngine()

	req := &types.RequestDefinition{
		Name:   "Get User",
		Method: "GET",
		URL:    "https://api.example.com/users",
	}
	resp := &types.ExecutionResult{
		StatusCode: 200,
		StatusText: "OK",
		Body:       `{"id": 1, "name": "Alice"}`,
	}

	iterCtx := &IterationContext{
		Iteration:      2,
		IterationCount: 5,
		DataRow: map[string]any{
			"email": "alice@example.com",
			"role":  "admin",
		},
	}

	script := `
		// Check pb.data and pb.info
		expect(pb.info.iteration).to.equal(2);
		expect(pb.info.iterationCount).to.equal(5);
		expect(pb.data.get("email")).to.equal("alice@example.com");
		expect(pb.data.role).to.equal("admin");
		expect(pm.iterationData.get("role")).to.equal("admin");

		// Test pb.runner.setNextRequest
		pb.runner.setNextRequest("Update User");
	`

	res, err := engine.ExecutePostResponseWithContext("test", script, req, resp, nil, iterCtx, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected script execution error: %v", err)
	}

	if res.NextRequest == nil || *res.NextRequest != "Update User" {
		t.Fatalf("expected NextRequest 'Update User', got: %v", res.NextRequest)
	}
	if res.StopAll {
		t.Fatalf("expected StopAll to be false")
	}
}

func TestEngine_RunnerSetNextRequestNull(t *testing.T) {
	engine := NewEngine()

	req := &types.RequestDefinition{Name: "Check"}
	resp := &types.ExecutionResult{StatusCode: 200}

	script := `
		pb.runner.setNextRequest(null);
	`

	res, err := engine.ExecutePostResponseWithContext("test", script, req, resp, nil, nil, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected script error: %v", err)
	}

	if res.NextRequest == nil || *res.NextRequest != "" {
		t.Fatalf("expected NextRequest to be empty string for null, got: %v", res.NextRequest)
	}
}

func TestEngine_RunnerStop(t *testing.T) {
	engine := NewEngine()

	req := &types.RequestDefinition{Name: "Check"}
	resp := &types.ExecutionResult{StatusCode: 200}

	script := `
		pb.runner.stop();
	`

	res, err := engine.ExecutePostResponseWithContext("test", script, req, resp, nil, nil, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected script error: %v", err)
	}

	if !res.StopAll {
		t.Fatalf("expected StopAll to be true")
	}
}

func TestEngine_PostmanSetNextRequestCompat(t *testing.T) {
	engine := NewEngine()

	req := &types.RequestDefinition{Name: "Legacy Postman"}
	resp := &types.ExecutionResult{StatusCode: 200}

	script := `
		postman.setNextRequest("Next Request");
	`

	res, err := engine.ExecutePostResponseWithContext("test", script, req, resp, nil, nil, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected script error: %v", err)
	}

	if res.NextRequest == nil || *res.NextRequest != "Next Request" {
		t.Fatalf("expected NextRequest 'Next Request', got: %v", res.NextRequest)
	}
}
