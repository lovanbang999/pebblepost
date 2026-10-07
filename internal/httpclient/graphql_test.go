package httpclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pebblepost/internal/scripting"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

// setupFakeGraphQLServer creates a test HTTP server supporting queries, mutations, errors, and introspection.
func setupFakeGraphQLServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Could not read body", http.StatusBadRequest)
			return
		}

		var payload struct {
			Query         string         `json:"query"`
			Variables     map[string]any `json:"variables"`
			OperationName string         `json:"operationName"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "Malformed JSON", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// 1. Introspection query
		if strings.Contains(payload.Query, "__schema") {
			schemaJSON := `{
				"data": {
					"__schema": {
						"queryType": { "name": "Query" },
						"mutationType": { "name": "Mutation" },
						"subscriptionType": { "name": "Subscription" },
						"types": [
							{
								"kind": "OBJECT",
								"name": "Query",
								"description": "Root query type",
								"fields": [
									{
										"name": "user",
										"description": "Retrieve user by ID",
										"args": [
											{
												"name": "id",
												"description": "User identifier",
												"type": { "kind": "NON_NULL", "name": null, "ofType": { "kind": "SCALAR", "name": "ID", "ofType": null } }
											}
										],
										"type": { "kind": "OBJECT", "name": "User", "ofType": null },
										"isDeprecated": false
									},
									{
										"name": "legacyField",
										"description": "Old field to be deprecated",
										"args": [],
										"type": { "kind": "SCALAR", "name": "String", "ofType": null },
										"isDeprecated": true,
										"deprecationReason": "Use modernField instead"
									}
								]
							},
							{
								"kind": "OBJECT",
								"name": "User",
								"description": "User entity",
								"fields": [
									{
										"name": "id",
										"type": { "kind": "NON_NULL", "name": null, "ofType": { "kind": "SCALAR", "name": "ID", "ofType": null } },
										"args": [],
										"isDeprecated": false
									},
									{
										"name": "name",
										"type": { "kind": "SCALAR", "name": "String", "ofType": null },
										"args": [],
										"isDeprecated": false
									}
								]
							}
						],
						"directives": []
					}
				}
			}`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(schemaJSON))
			return
		}

		// 2. Multi-operation document: check operationName
		if payload.OperationName != "" {
			switch payload.OperationName {
			case "GetUser":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":{"user":{"id":"42","name":"Arthur Dent"}}}`))
				return
			case "UpdateUser":
				name, _ := payload.Variables["name"].(string)
				w.WriteHeader(http.StatusOK)
				resp, _ := json.Marshal(map[string]any{
					"data": map[string]any{
						"updateUser": map[string]any{
							"id":   "42",
							"name": name,
						},
					},
				})
				_, _ = w.Write(resp)
				return
			default:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"errors":[{"message":"Unknown operation: ` + payload.OperationName + `"}]}`))
				return
			}
		}

		// 3. Mutation query
		if strings.HasPrefix(strings.TrimSpace(payload.Query), "mutation") {
			title, _ := payload.Variables["title"].(string)
			w.WriteHeader(http.StatusOK)
			resp, _ := json.Marshal(map[string]any{
				"data": map[string]any{
					"createPost": map[string]any{
						"id":    "101",
						"title": title,
					},
				},
			})
			_, _ = w.Write(resp)
			return
		}

		// 4. Query with syntax error / field error
		if strings.Contains(payload.Query, "triggerError") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errors":[{"message":"Cannot query field triggerError on type Query"}]}`))
			return
		}

		// 5. Default Query
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"user":{"id":"1","name":"Alice"}}}`))
	}))
}

func TestGraphQL_QueryExecutionAndOperationName(t *testing.T) {
	server := setupFakeGraphQLServer(t)
	defer server.Close()

	client := NewClient()

	// 1. Single Query
	req := &types.RequestDefinition{
		Method: "POST",
		URL:    server.URL,
		Body: types.BodyDefinition{
			Type: "graphql",
			GraphQL: &types.GraphQL{
				Query: "query { user { id name } }",
			},
		},
	}
	res, err := client.Execute(context.Background(), req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(res.Body, "Alice") {
		t.Errorf("expected response to contain Alice, got %s", res.Body)
	}

	// 2. Multi-operation with OperationName GetUser
	multiDoc := `
		query GetUser { user { id name } }
		mutation UpdateUser($name: String!) { updateUser(name: $name) { id name } }
	`
	reqGetUser := &types.RequestDefinition{
		Method: "POST",
		URL:    server.URL,
		Body: types.BodyDefinition{
			Type: "graphql",
			GraphQL: &types.GraphQL{
				Query:         multiDoc,
				OperationName: "GetUser",
			},
		},
	}
	resGetUser, err := client.Execute(context.Background(), reqGetUser)
	if err != nil || resGetUser.StatusCode != 200 {
		t.Fatalf("GetUser failed: %v", err)
	}
	if !strings.Contains(resGetUser.Body, "Arthur Dent") {
		t.Errorf("expected Arthur Dent in response, got %s", resGetUser.Body)
	}

	// 3. Multi-operation with OperationName UpdateUser and Variables
	reqUpdateUser := &types.RequestDefinition{
		Method: "POST",
		URL:    server.URL,
		Body: types.BodyDefinition{
			Type: "graphql",
			GraphQL: &types.GraphQL{
				Query:         multiDoc,
				OperationName: "UpdateUser",
				Variables:     `{"name": "Ford Prefect"}`,
			},
		},
	}
	resUpdateUser, err := client.Execute(context.Background(), reqUpdateUser)
	if err != nil || resUpdateUser.StatusCode != 200 {
		t.Fatalf("UpdateUser failed: %v", err)
	}
	if !strings.Contains(resUpdateUser.Body, "Ford Prefect") {
		t.Errorf("expected Ford Prefect in response, got %s", resUpdateUser.Body)
	}

	// 4. Mutation
	reqMutation := &types.RequestDefinition{
		Method: "POST",
		URL:    server.URL,
		Body: types.BodyDefinition{
			Type: "graphql",
			GraphQL: &types.GraphQL{
				Query:     "mutation CreatePost($title: String!) { createPost(title: $title) { id title } }",
				Variables: `{"title": "Deep Space Travel"}`,
			},
		},
	}
	resMutation, err := client.Execute(context.Background(), reqMutation)
	if err != nil || resMutation.StatusCode != 200 {
		t.Fatalf("Mutation failed: %v", err)
	}
	if !strings.Contains(resMutation.Body, "Deep Space Travel") {
		t.Errorf("expected Deep Space Travel in response, got %s", resMutation.Body)
	}

	// 5. GraphQL Error handling
	reqError := &types.RequestDefinition{
		Method: "POST",
		URL:    server.URL,
		Body: types.BodyDefinition{
			Type: "graphql",
			GraphQL: &types.GraphQL{
				Query: "query { triggerError }",
			},
		},
	}
	resError, err := client.Execute(context.Background(), reqError)
	if err != nil || resError.StatusCode != 200 {
		t.Fatalf("Error query failed: %v", err)
	}
	if !strings.Contains(resError.Body, "Cannot query field triggerError") {
		t.Errorf("expected GraphQL error in response, got %s", resError.Body)
	}
}

func TestGraphQL_IntrospectionEndpoint(t *testing.T) {
	server := setupFakeGraphQLServer(t)
	defer server.Close()

	client := NewClient()
	wsSvc := workspace.NewWorkspaceService()
	envSvc := workspace.NewEnvironmentService()
	in := workspace.NewInterpolator()
	scriptEngine := scripting.NewEngine()

	handler := NewHandler(client, wsSvc, envSvc, in, scriptEngine, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	introspectPayload := ExecutePayload{
		Request: &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Headers: []types.KeyValue{
				{Key: "X-Custom-Header", Value: "IntrospectCheck", Enabled: true},
			},
		},
	}

	bodyBytes, _ := json.Marshal(introspectPayload)
	req := httptest.NewRequest("POST", "/api/graphql/introspect", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for introspection, got %d: %s", w.Code, w.Body.String())
	}

	var schemaResp struct {
		Data struct {
			Schema struct {
				QueryType struct {
					Name string `json:"name"`
				} `json:"queryType"`
				Types []struct {
					Name string `json:"name"`
				} `json:"types"`
			} `json:"__schema"`
		} `json:"data"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &schemaResp); err != nil {
		t.Fatalf("failed to parse schema JSON: %v", err)
	}

	if schemaResp.Data.Schema.QueryType.Name != "Query" {
		t.Errorf("expected QueryType Query, got %s", schemaResp.Data.Schema.QueryType.Name)
	}
	if len(schemaResp.Data.Schema.Types) == 0 {
		t.Errorf("expected non-empty types list in schema")
	}
}

func TestGraphQL_AssertionsAndVariables(t *testing.T) {
	server := setupFakeGraphQLServer(t)
	defer server.Close()

	client := NewClient()
	wsSvc := workspace.NewWorkspaceService()
	envSvc := workspace.NewEnvironmentService()
	in := workspace.NewInterpolator()
	scriptEngine := scripting.NewEngine()

	handler := NewHandler(client, wsSvc, envSvc, in, scriptEngine, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test request with {{variable}} interpolation and pb.test assertion script
	execPayload := ExecutePayload{
		Overrides: map[string]string{
			"userName": "Ford Prefect",
			"op":       "UpdateUser",
		},
		Request: &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Body: types.BodyDefinition{
				Type: "graphql",
				GraphQL: &types.GraphQL{
					Query: `
						query GetUser { user { id name } }
						mutation UpdateUser($name: String!) { updateUser(name: $name) { id name } }
					`,
					OperationName: "{{op}}",
					Variables:     `{"name": "{{userName}}"}`,
				},
			},
			Scripts: types.ScriptDefinition{
				PostResponse: `
					pb.test("status is 200", function() {
						pb.expect(pb.response.status).to.equal(200);
					});
					pb.test("returns updated user name", function() {
						var res = pb.response.json();
						pb.expect(res.data.updateUser.name).to.equal("Ford Prefect");
						pb.expect(res.data.updateUser.id).to.equal("42");
					});
				`,
			},
		},
	}

	bodyBytes, _ := json.Marshal(execPayload)
	req := httptest.NewRequest("POST", "/api/request/execute", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var execResult types.ExecutionResult
	if err := json.Unmarshal(w.Body.Bytes(), &execResult); err != nil {
		t.Fatalf("failed to decode execution result: %v", err)
	}

	if len(execResult.Tests) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(execResult.Tests))
	}
	for _, tc := range execResult.Tests {
		if !tc.Passed {
			t.Errorf("test %s failed: %s", tc.Name, tc.Message)
		}
	}
}
