package runner

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/reflect/protoregistry"

	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

var registerRunnerProtoOnce sync.Once

func setupRunnerFakeServer(t *testing.T) (string, func()) {
	t.Helper()

	parser := protoparse.Parser{
		ImportPaths: []string{"../grpcclient/testdata"},
	}
	fds, err := parser.ParseFiles("test.proto")
	if err != nil {
		t.Fatalf("failed to parse test.proto: %v", err)
	}
	fd := fds[0]

	registerRunnerProtoOnce.Do(func() {
		_ = protoregistry.GlobalFiles.RegisterFile(fd.UnwrapFile())
	})

	reqDesc := fd.FindMessage("test.TestRequest")
	resDesc := fd.FindMessage("test.TestResponse")

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
		_ = grpc.SetHeader(ctx, metadata.Pairs("x-server-meta", "runner-meta"))
		msgVal := reqMsg.GetFieldByName("message").(string)
		respMsg := dynamic.NewMessage(resDesc)
		respMsg.SetFieldByName("reply", "runner: "+msgVal)
		respMsg.SetFieldByName("status", "SUCCESS")
		return respMsg, nil
	}

	serverStreamHandler := func(srv any, ss grpc.ServerStream) error {
		reqMsg := dynamic.NewMessage(reqDesc)
		if err := ss.RecvMsg(reqMsg); err != nil {
			return err
		}
		for i := 0; i < 2; i++ {
			respMsg := dynamic.NewMessage(resDesc)
			respMsg.SetFieldByName("reply", fmt.Sprintf("stream %d", i))
			respMsg.SetFieldByName("status", "STREAMING")
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
			{MethodName: "UnaryCall", Handler: unaryHandler},
		},
		Streams: []grpc.StreamDesc{
			{StreamName: "ServerStreamCall", Handler: serverStreamHandler, ServerStreams: true, ClientStreams: false},
		},
		Metadata: "test.proto",
	}

	grpcServer.RegisterService(&serviceDesc, struct{}{})
	reflection.Register(grpcServer)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	addr := lis.Addr().String()
	cleanup := func() {
		grpcServer.Stop()
		_ = lis.Close()
	}
	return addr, cleanup
}

func TestRunner_GrpcCollection(t *testing.T) {
	addr, cleanup := setupRunnerFakeServer(t)
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "pebblepost_runner_grpc_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	wsSvc := workspace.NewWorkspaceService()

	// 1. Unary gRPC request with tests
	req1 := types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "01 Unary gRPC",
		Protocol:      "grpc",
		Grpc: &types.GrpcDefinition{
			Address:     addr,
			ProtoSource: "reflection",
			Service:     "test.TestService",
			Method:      "UnaryCall",
			Message:     `{"message": "cli-test"}`,
		},
		Scripts: types.ScriptDefinition{
			PostResponse: `
				const res = pb.response.json();
				pb.test("Status is SUCCESS", () => {
					pb.expect(res.status).to.eql("SUCCESS");
				});
				pb.test("Reply contains cli-test", () => {
					pb.expect(res.reply).to.eql("runner: cli-test");
				});
			`,
		},
	}
	_ = wsSvc.SaveRequest(filepath.Join(tmpDir, "01-unary.pebble.json"), &req1)

	// 2. Server streaming gRPC request with tests
	req2 := types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "02 Streaming gRPC",
		Protocol:      "grpc",
		Grpc: &types.GrpcDefinition{
			Address:     addr,
			ProtoSource: "reflection",
			Service:     "test.TestService",
			Method:      "ServerStreamCall",
			Message:     `{"message": "stream", "count": 2}`,
		},
		Scripts: types.ScriptDefinition{
			PostResponse: `
				const items = pb.response.json();
				pb.test("Received 2 stream items", () => {
					pb.expect(items.length).to.eql(2);
				});
				pb.test("First item status is STREAMING", () => {
					pb.expect(items[0].status).to.eql("STREAMING");
				});
			`,
		},
	}
	_ = wsSvc.SaveRequest(filepath.Join(tmpDir, "02-streaming.pebble.json"), &req2)

	r := NewRunner()
	var out bytes.Buffer

	summary, err := r.Run(context.Background(), RunOptions{
		TargetPath:   tmpDir,
		ReportFormat: "terminal",
		Writer:       &out,
	})
	if err != nil {
		t.Fatalf("Runner.Run failed: %v", err)
	}

	if summary.TotalRequests != 2 {
		t.Errorf("expected 2 requests, got %d", summary.TotalRequests)
	}
	if summary.PassedRequests != 2 {
		t.Errorf("expected 2 passed requests, got %d. Output: %s", summary.PassedRequests, out.String())
	}
	if summary.PassedTests != 4 {
		t.Errorf("expected 4 passed tests, got %d", summary.PassedTests)
	}
}
