package grpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/reflect/protoregistry"

	"pebblepost/internal/scripting"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

var registerTestProtoOnce sync.Once

// setupFakeServer starts an in-process gRPC test server with reflection enabled.
func setupFakeServer(t *testing.T) (string, func()) {
	t.Helper()

	parser := protoparse.Parser{
		ImportPaths: []string{"testdata"},
	}
	fds, err := parser.ParseFiles("test.proto")
	if err != nil {
		t.Fatalf("failed to parse test.proto: %v", err)
	}
	if len(fds) == 0 {
		t.Fatalf("no file descriptors parsed")
	}
	fd := fds[0]

	// Register in global registry for reflection service once
	registerTestProtoOnce.Do(func() {
		_ = protoregistry.GlobalFiles.RegisterFile(fd.UnwrapFile())
	})

	svc := fd.FindService("test.TestService")
	if svc == nil {
		t.Fatalf("test.TestService not found in test.proto")
	}

	reqDesc := fd.FindMessage("test.TestRequest")
	resDesc := fd.FindMessage("test.TestResponse")
	sumDesc := fd.FindMessage("test.SummaryResponse")

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	unaryHandler := func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		reqMsg := dynamic.NewMessage(reqDesc)
		if err := dec(reqMsg); err != nil {
			return nil, err
		}

		_ = grpc.SetHeader(ctx, metadata.Pairs("x-server-meta", "meta-value-1"))
		_ = grpc.SetTrailer(ctx, metadata.Pairs("x-server-trailer", "trailer-value-1"))

		msgVal := reqMsg.GetFieldByName("message").(string)
		respMsg := dynamic.NewMessage(resDesc)
		respMsg.SetFieldByName("reply", "echo: "+msgVal)
		respMsg.SetFieldByName("status", "SUCCESS")
		return respMsg, nil
	}

	serverStreamHandler := func(srv any, ss grpc.ServerStream) error {
		reqMsg := dynamic.NewMessage(reqDesc)
		if err := ss.RecvMsg(reqMsg); err != nil {
			return err
		}

		_ = ss.SetHeader(metadata.Pairs("x-stream-header", "stream-started"))
		ss.SetTrailer(metadata.Pairs("x-stream-trailer", "stream-finished"))

		countVal := int(reqMsg.GetFieldByName("count").(int32))
		if countVal <= 0 {
			countVal = 3
		}

		for i := 0; i < countVal; i++ {
			select {
			case <-ss.Context().Done():
				return ss.Context().Err()
			default:
			}
			time.Sleep(5 * time.Millisecond)

			respMsg := dynamic.NewMessage(resDesc)
			respMsg.SetFieldByName("reply", fmt.Sprintf("stream item %d", i))
			respMsg.SetFieldByName("status", "STREAMING")
			if err := ss.SendMsg(respMsg); err != nil {
				return err
			}
		}
		return nil
	}

	clientStreamHandler := func(srv any, ss grpc.ServerStream) error {
		total := int32(0)
		for {
			reqMsg := dynamic.NewMessage(reqDesc)
			err := ss.RecvMsg(reqMsg)
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			total++
		}

		sumMsg := dynamic.NewMessage(sumDesc)
		sumMsg.SetFieldByName("total_received", total)
		sumMsg.SetFieldByName("summary", fmt.Sprintf("collected %d messages", total))
		return ss.SendMsg(sumMsg)
	}

	bidiStreamHandler := func(srv any, ss grpc.ServerStream) error {
		for {
			reqMsg := dynamic.NewMessage(reqDesc)
			err := ss.RecvMsg(reqMsg)
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			msgVal := reqMsg.GetFieldByName("message").(string)
			respMsg := dynamic.NewMessage(resDesc)
			respMsg.SetFieldByName("reply", "bidi: "+msgVal)
			respMsg.SetFieldByName("status", "ACTIVE")
			if err := ss.SendMsg(respMsg); err != nil {
				return err
			}
		}
		return nil
	}

	serviceDesc := grpc.ServiceDesc{
		ServiceName: "test.TestService",
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "UnaryCall",
				Handler:    unaryHandler,
			},
		},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "ServerStreamCall",
				Handler:       serverStreamHandler,
				ServerStreams: true,
				ClientStreams: false,
			},
			{
				StreamName:    "ClientStreamCall",
				Handler:       clientStreamHandler,
				ServerStreams: false,
				ClientStreams: true,
			},
			{
				StreamName:    "BidiStreamCall",
				Handler:       bidiStreamHandler,
				ServerStreams: true,
				ClientStreams: true,
			},
		},
		Metadata: "test.proto",
	}

	grpcServer.RegisterService(&serviceDesc, struct{}{})
	reflection.Register(grpcServer)

	serverDone := make(chan struct{})
	go func() {
		_ = grpcServer.Serve(lis)
		close(serverDone)
	}()

	addr := lis.Addr().String()
	cleanup := func() {
		grpcServer.Stop()
		select {
		case <-serverDone:
		case <-time.After(3 * time.Second):
		}
	}

	return addr, cleanup
}

func TestReflectServices(t *testing.T) {
	addr, cleanup := setupFakeServer(t)
	defer cleanup()

	client := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	services, err := client.ReflectServices(ctx, addr, false, false, "")
	if err != nil {
		t.Fatalf("ReflectServices failed: %v", err)
	}

	found := false
	for _, s := range services {
		if s.Name == "test.TestService" {
			found = true
			if len(s.Methods) != 4 {
				t.Errorf("expected 4 methods, got %d", len(s.Methods))
			}
		}
	}
	if !found {
		t.Errorf("test.TestService not found in reflection response: %+v", services)
	}
}

func TestLoadProtoServices(t *testing.T) {
	client := NewClient("")

	services, err := client.LoadProtoServices([]string{"test.proto"}, []string{"testdata"})
	if err != nil {
		t.Fatalf("LoadProtoServices failed: %v", err)
	}

	if len(services) == 0 {
		t.Fatalf("expected services from test.proto, got 0")
	}
	if services[0].Name != "test.TestService" {
		t.Errorf("expected test.TestService, got %s", services[0].Name)
	}
}

func TestGenerateSampleJSON(t *testing.T) {
	client := NewClient("")
	ctx := context.Background()

	g := &types.GrpcDefinition{
		ProtoSource: "file",
		ProtoFiles:  []string{"test.proto"},
		ImportPaths: []string{"testdata"},
		Service:     "test.TestService",
		Method:      "UnaryCall",
	}

	sample, err := client.GenerateSampleMessage(ctx, g)
	if err != nil {
		t.Fatalf("GenerateSampleMessage failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(sample), &parsed); err != nil {
		t.Fatalf("sample is not valid JSON: %v, raw: %s", err, sample)
	}
	if _, ok := parsed["message"]; !ok {
		t.Errorf("sample JSON missing 'message' field: %s", sample)
	}
}

func TestUnaryCall(t *testing.T) {
	addr, cleanup := setupFakeServer(t)
	defer cleanup()

	client := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Test Unary",
		Protocol:      "grpc",
		Grpc: &types.GrpcDefinition{
			Address:     addr,
			ProtoSource: "reflection",
			Service:     "test.TestService",
			Method:      "UnaryCall",
			Message:     `{"message": "antigravity"}`,
			Metadata: []types.KeyValue{
				{Key: "x-client-meta", Value: "123", Enabled: true},
			},
		},
	}

	result, err := client.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute unary failed: %v", err)
	}

	if result.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}
	if result.GrpcStatus == nil || *result.GrpcStatus != 0 {
		t.Errorf("expected gRPC status 0, got %v", result.GrpcStatus)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result.Body), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v, body: %s", err, result.Body)
	}
	if resp["reply"] != "echo: antigravity" {
		t.Errorf("unexpected reply: %v", resp["reply"])
	}

	// Verify metadata and trailers
	if len(result.GrpcMetadata["x-server-meta"]) == 0 || result.GrpcMetadata["x-server-meta"][0] != "meta-value-1" {
		t.Errorf("missing or incorrect x-server-meta in GrpcMetadata: %+v", result.GrpcMetadata)
	}
	if len(result.GrpcTrailers["x-server-trailer"]) == 0 || result.GrpcTrailers["x-server-trailer"][0] != "trailer-value-1" {
		t.Errorf("missing or incorrect x-server-trailer in GrpcTrailers: %+v", result.GrpcTrailers)
	}
}

func TestServerStreaming(t *testing.T) {
	addr, cleanup := setupFakeServer(t)
	defer cleanup()

	client := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var receivedMessages []types.GrpcStreamMessage
	var mu sync.Mutex

	req := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Test Server Streaming",
		Protocol:      "grpc",
		Grpc: &types.GrpcDefinition{
			Address:     addr,
			ProtoSource: "reflection",
			Service:     "test.TestService",
			Method:      "ServerStreamCall",
			Message:     `{"message": "stream-test", "count": 3}`,
		},
	}

	result, err := client.ExecuteStream(ctx, req, func(msg types.GrpcStreamMessage) {
		mu.Lock()
		defer mu.Unlock()
		receivedMessages = append(receivedMessages, msg)
	})
	if err != nil {
		t.Fatalf("ExecuteStream failed: %v", err)
	}

	if result.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}

	// Should have 1 send message + 3 receive messages = 4 stream messages
	if len(result.GrpcMessages) != 4 {
		t.Errorf("expected 4 stream messages in result, got %d", len(result.GrpcMessages))
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(result.Body), &items); err != nil {
		t.Fatalf("failed to parse array body: %v, raw: %s", err, result.Body)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items in body array, got %d", len(items))
	}
	if items[0]["reply"] != "stream item 0" {
		t.Errorf("unexpected first item: %+v", items[0])
	}
}

func TestClientStreaming(t *testing.T) {
	addr, cleanup := setupFakeServer(t)
	defer cleanup()

	client := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Test Client Streaming",
		Protocol:      "grpc",
		Grpc: &types.GrpcDefinition{
			Address:     addr,
			ProtoSource: "reflection",
			Service:     "test.TestService",
			Method:      "ClientStreamCall",
			Messages: []string{
				`{"message": "first"}`,
				`{"message": "second"}`,
				`{"message": "third"}`,
			},
		},
	}

	result, err := client.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute client stream failed: %v", err)
	}

	var sum map[string]interface{}
	if err := json.Unmarshal([]byte(result.Body), &sum); err != nil {
		t.Fatalf("failed to parse summary JSON: %v, body: %s", err, result.Body)
	}
	if sum["total_received"] != float64(3) && sum["totalReceived"] != float64(3) {
		t.Errorf("unexpected total_received: %+v", sum)
	}
}

func TestBidiStreaming(t *testing.T) {
	addr, cleanup := setupFakeServer(t)
	defer cleanup()

	client := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Test Bidi Streaming",
		Protocol:      "grpc",
		Grpc: &types.GrpcDefinition{
			Address:     addr,
			ProtoSource: "reflection",
			Service:     "test.TestService",
			Method:      "BidiStreamCall",
			Messages: []string{
				`{"message": "alpha"}`,
				`{"message": "beta"}`,
			},
		},
	}

	result, err := client.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute bidi stream failed: %v", err)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(result.Body), &items); err != nil {
		t.Fatalf("failed to parse bidi body: %v, raw: %s", err, result.Body)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if items[0]["reply"] != "bidi: alpha" {
		t.Errorf("unexpected first bidi item: %+v", items[0])
	}
}

func TestVariableInterpolation(t *testing.T) {
	addr, cleanup := setupFakeServer(t)
	defer cleanup()

	interpolator := workspace.NewInterpolator()
	client := NewClient("")

	host, port, _ := net.SplitHostPort(addr)
	vars := map[string]string{
		"grpc_host": host,
		"grpc_port": port,
		"greeting":  "world",
		"meta_val":  "token_secret",
	}

	req := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Interpolated gRPC",
		Protocol:      "grpc",
		Grpc: &types.GrpcDefinition{
			Address:     "{{grpc_host}}:{{grpc_port}}",
			ProtoSource: "reflection",
			Service:     "test.TestService",
			Method:      "UnaryCall",
			Message:     `{"message": "Hello {{greeting}}"}`,
			Metadata: []types.KeyValue{
				{Key: "Authorization", Value: "Bearer {{meta_val}}", Enabled: true},
			},
		},
	}

	interpolated, err := interpolator.InterpolateRequestWithError(req, vars)
	if err != nil {
		t.Fatalf("Interpolation error: %v", err)
	}

	if interpolated.Grpc.Address != addr {
		t.Errorf("expected interpolated address %s, got %s", addr, interpolated.Grpc.Address)
	}
	if interpolated.Grpc.Message != `{"message": "Hello world"}` {
		t.Errorf("expected message `{\"message\": \"Hello world\"}`, got %s", interpolated.Grpc.Message)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Execute(ctx, interpolated)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal([]byte(result.Body), &resp)
	if resp["reply"] != "echo: Hello world" {
		t.Errorf("unexpected reply: %v", resp["reply"])
	}
}

func TestStreamingCancel(t *testing.T) {
	addr, cleanup := setupFakeServer(t)
	defer cleanup()

	client := NewClient("")
	ctx, cancel := context.WithCancel(context.Background())

	req := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Cancel Streaming",
		Protocol:      "grpc",
		Grpc: &types.GrpcDefinition{
			Address:     addr,
			ProtoSource: "reflection",
			Service:     "test.TestService",
			Method:      "ServerStreamCall",
			Message:     `{"message": "cancel-test", "count": 100}`,
		},
	}

	receivedCount := 0
	result, err := client.ExecuteStream(ctx, req, func(msg types.GrpcStreamMessage) {
		if msg.Direction == "receive" {
			receivedCount++
			if receivedCount == 1 {
				cancel() // cancel stream mid-flight
			}
		}
	})

	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}

	// Result should reflect cancellation or stop gracefully
	if result.GrpcStatus == nil {
		t.Errorf("expected GrpcStatus to be populated")
	}
	if receivedCount > 10 {
		t.Errorf("expected stream to stop shortly after cancel, but received %d messages", receivedCount)
	}
}

func TestGrpcAssertionsAndScripts(t *testing.T) {
	addr, cleanup := setupFakeServer(t)
	defer cleanup()

	client := NewClient("")
	scriptEngine := scripting.NewEngine()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "gRPC with Assertions",
		Protocol:      "grpc",
		Grpc: &types.GrpcDefinition{
			Address:     addr,
			ProtoSource: "reflection",
			Service:     "test.TestService",
			Method:      "UnaryCall",
			Message:     `{"message": "assert-test"}`,
		},
		Scripts: types.ScriptDefinition{
			PostResponse: `
				const res = pb.response.json();
				pb.test("Status check", () => {
					pb.expect(res.status).to.eql("SUCCESS");
				});
				pb.test("Reply contains assert-test", () => {
					pb.expect(res.reply).to.include("assert-test");
				});
				pb.test("Metadata header check", () => {
					pb.expect(pb.response.headers["x-server-meta"]).to.be.ok;
				});
			`,
		},
	}

	result, err := client.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	postResult, err := scriptEngine.ExecutePostResponseNamed("request", req.Scripts.PostResponse, req, result, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("script execution error: %v", err)
	}

	if len(postResult.Tests) != 3 {
		t.Fatalf("expected 3 tests, got %d", len(postResult.Tests))
	}
	for _, tc := range postResult.Tests {
		if !tc.Passed {
			t.Errorf("assertion failed: %s (%s)", tc.Name, tc.Message)
		}
	}
}

func TestGrpcClient_ProtoPathGuard(t *testing.T) {
	wsDir := t.TempDir()
	secretDir := t.TempDir()

	// Copy test.proto into workspace
	protoData, err := os.ReadFile("testdata/test.proto")
	if err != nil {
		t.Fatalf("failed to read test.proto: %v", err)
	}
	wsProto := filepath.Join(wsDir, "test.proto")
	if err := os.WriteFile(wsProto, protoData, 0644); err != nil {
		t.Fatalf("failed to write ws proto: %v", err)
	}

	outsideProto := filepath.Join(secretDir, "outside.proto")
	if err := os.WriteFile(outsideProto, protoData, 0644); err != nil {
		t.Fatalf("failed to write outside proto: %v", err)
	}

	client := NewClient(wsDir)

	// 1. Valid workspace proto succeeds
	t.Run("Valid workspace proto loads", func(t *testing.T) {
		services, err := client.LoadProtoServices([]string{"test.proto"}, nil)
		if err != nil {
			t.Fatalf("expected proto loading to succeed, got: %v", err)
		}
		if len(services) == 0 {
			t.Fatalf("expected at least 1 service loaded")
		}
	})

	// 2. Traversal in import path rejected
	t.Run("Import path traversal rejected", func(t *testing.T) {
		_, err := client.LoadProtoServices([]string{"test.proto"}, []string{"../../outside"})
		if err == nil {
			t.Fatalf("expected traversal import path to fail, got nil")
		}
		if !strings.Contains(err.Error(), "rejected") && !strings.Contains(err.Error(), "forbidden") {
			t.Errorf("expected security rejection error, got: %v", err)
		}
	})

	// 3. Absolute proto file outside workspace rejected
	t.Run("External absolute proto file rejected", func(t *testing.T) {
		_, err := client.LoadProtoServices([]string{outsideProto}, nil)
		if err == nil {
			t.Fatalf("expected external proto path to fail, got nil")
		}
		if !strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "rejected") {
			t.Errorf("expected security rejection error, got: %v", err)
		}
	})

	// 4. Root CA path traversal in Dial rejected
	t.Run("Root CA path traversal in Dial rejected", func(t *testing.T) {
		_, err := client.Dial(context.Background(), "127.0.0.1:50051", true, false, "../../etc/ca.pem")
		if err == nil {
			t.Fatalf("expected traversal CA path to fail, got nil")
		}
		if !strings.Contains(err.Error(), "rejected") && !strings.Contains(err.Error(), "forbidden") {
			t.Errorf("expected security rejection error, got: %v", err)
		}
	})
}
