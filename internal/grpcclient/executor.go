package grpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"pebblepost/internal/types"
)

// Execute runs the gRPC request (unary or streaming) and returns an ExecutionResult.
func (c *Client) Execute(ctx context.Context, req *types.RequestDefinition) (*types.ExecutionResult, error) {
	return c.ExecuteStream(ctx, req, nil)
}

// ExecuteStream runs the gRPC request and emits stream messages to onMessage as they arrive.
func (c *Client) ExecuteStream(ctx context.Context, req *types.RequestDefinition, onMessage func(msg types.GrpcStreamMessage)) (*types.ExecutionResult, error) {
	if req == nil || req.Grpc == nil {
		return nil, fmt.Errorf("grpc request definition is nil")
	}
	g := req.Grpc
	if g.Address == "" {
		return nil, fmt.Errorf("grpc address is required")
	}
	if g.Service == "" || g.Method == "" {
		return nil, fmt.Errorf("grpc service and method are required")
	}

	start := time.Now()

	methodDesc, conn, cleanup, err := c.ResolveMethodDescriptor(ctx, g)
	if err != nil {
		return &types.ExecutionResult{
			StatusCode:     500,
			StatusText:     "Method Resolution Failed",
			Error:          err.Error(),
			ExecutedAt:     time.Now(),
			Timing:         types.TimingMetrics{TotalDurationMs: float64(time.Since(start).Milliseconds())},
			Logs:           []string{"[ERROR] " + err.Error()},
			GrpcStatusText: "UNKNOWN",
		}, nil
	}
	defer cleanup()

	// Build outgoing metadata
	outMD := metadata.MD{}
	for _, kv := range g.Metadata {
		if kv.Enabled && kv.Key != "" {
			outMD.Append(strings.ToLower(kv.Key), kv.Value)
		}
	}
	callCtx := metadata.NewOutgoingContext(ctx, outMD)

	stub := grpcdynamic.NewStub(conn)
	inputDesc := methodDesc.GetInputType()

	var streamMessages []types.GrpcStreamMessage
	msgIndex := 0

	recordMessage := func(dir string, payload string, isErr bool) {
		sm := types.GrpcStreamMessage{
			Index:     msgIndex,
			Direction: dir,
			Timestamp: time.Now(),
			Payload:   payload,
			IsError:   isErr,
		}
		msgIndex++
		streamMessages = append(streamMessages, sm)
		if onMessage != nil {
			onMessage(sm)
		}
	}

	var headerMD, trailerMD metadata.MD
	var body string
	var callErr error
	var grpcStatusCode int
	var grpcStatusText string
	var httpStatusCode int

	isClientStream := methodDesc.IsClientStreaming()
	isServerStream := methodDesc.IsServerStreaming()

	// ── 1. Unary RPC ─────────────────────────────────────────────────────────────
	if !isClientStream && !isServerStream {
		var reqMsg *dynamic.Message
		reqMsg, err = createMessageFromJSON(inputDesc, g.Message)
		if err != nil {
			return errorExecutionResult(err, start), nil
		}

		rawJSON := g.Message
		if strings.TrimSpace(rawJSON) == "" {
			rawJSON = "{}"
		}
		recordMessage("send", rawJSON, false)

		respMsg, err := stub.InvokeRpc(callCtx, methodDesc, reqMsg, grpc.Header(&headerMD), grpc.Trailer(&trailerMD))
		callErr = err

		if respMsg != nil {
			if dynMsg, ok := respMsg.(*dynamic.Message); ok {
				b, _ := dynMsg.MarshalJSONIndent()
				body = string(b)
			} else {
				b, _ := json.MarshalIndent(respMsg, "", "  ")
				body = string(b)
			}
			recordMessage("receive", body, false)
		}
	} else if !isClientStream && isServerStream {
		// ── 2. Server Streaming RPC ──────────────────────────────────────────────
		var reqMsg *dynamic.Message
		reqMsg, err = createMessageFromJSON(inputDesc, g.Message)
		if err != nil {
			return errorExecutionResult(err, start), nil
		}

		rawJSON := g.Message
		if strings.TrimSpace(rawJSON) == "" {
			rawJSON = "{}"
		}
		recordMessage("send", rawJSON, false)

		var serverStream *grpcdynamic.ServerStream
		serverStream, callErr = stub.InvokeRpcServerStream(callCtx, methodDesc, reqMsg)
		if callErr == nil {
			var receivedJSONs []string
			for {
				msg, recvErr := serverStream.RecvMsg()
				if recvErr == io.EOF {
					break
				}
				if recvErr != nil {
					callErr = recvErr
					recordMessage("receive", fmt.Sprintf(`{"error": %q}`, recvErr.Error()), true)
					break
				}
				var msgJSON string
				if dynMsg, ok := msg.(*dynamic.Message); ok {
					b, _ := dynMsg.MarshalJSON()
					msgJSON = string(b)
				} else {
					b, _ := json.Marshal(msg)
					msgJSON = string(b)
				}
				receivedJSONs = append(receivedJSONs, msgJSON)
				recordMessage("receive", msgJSON, false)
			}
			headerMD, _ = serverStream.Header()
			trailerMD = serverStream.Trailer()
			body = "[" + strings.Join(receivedJSONs, ",\n") + "]"
		}
	} else if isClientStream && !isServerStream {
		// ── 3. Client Streaming RPC ──────────────────────────────────────────────
		messagesToSend := g.Messages
		if len(messagesToSend) == 0 && g.Message != "" {
			messagesToSend = []string{g.Message}
		}

		var clientStream *grpcdynamic.ClientStream
		clientStream, callErr = stub.InvokeRpcClientStream(callCtx, methodDesc)
		if callErr == nil {
			for _, rawMsg := range messagesToSend {
				msg, sendErr := createMessageFromJSON(inputDesc, rawMsg)
				if sendErr != nil {
					callErr = sendErr
					break
				}
				recordMessage("send", rawMsg, false)
				if err := clientStream.SendMsg(msg); err != nil {
					callErr = err
					break
				}
			}

			if callErr == nil {
				respMsg, recvErr := clientStream.CloseAndReceive()
				callErr = recvErr
				if respMsg != nil {
					if dynMsg, ok := respMsg.(*dynamic.Message); ok {
						b, _ := dynMsg.MarshalJSONIndent()
						body = string(b)
					} else {
						b, _ := json.MarshalIndent(respMsg, "", "  ")
						body = string(b)
					}
					recordMessage("receive", body, false)
				}
			}
			headerMD, _ = clientStream.Header()
			trailerMD = clientStream.Trailer()
		}
	} else {
		// ── 4. Bidirectional Streaming RPC ───────────────────────────────────────
		messagesToSend := g.Messages
		if len(messagesToSend) == 0 && g.Message != "" {
			messagesToSend = []string{g.Message}
		}

		var bidiStream *grpcdynamic.BidiStream
		bidiStream, callErr = stub.InvokeRpcBidiStream(callCtx, methodDesc)
		if callErr == nil {
			// Send outgoing messages
			for _, rawMsg := range messagesToSend {
				msg, sendErr := createMessageFromJSON(inputDesc, rawMsg)
				if sendErr != nil {
					callErr = sendErr
					break
				}
				recordMessage("send", rawMsg, false)
				if err := bidiStream.SendMsg(msg); err != nil {
					callErr = err
					break
				}
			}
			_ = bidiStream.CloseSend()

			// Receive incoming messages until EOF
			var receivedJSONs []string
			for {
				msg, recvErr := bidiStream.RecvMsg()
				if recvErr == io.EOF {
					break
				}
				if recvErr != nil {
					callErr = recvErr
					recordMessage("receive", fmt.Sprintf(`{"error": %q}`, recvErr.Error()), true)
					break
				}
				var msgJSON string
				if dynMsg, ok := msg.(*dynamic.Message); ok {
					b, _ := dynMsg.MarshalJSON()
					msgJSON = string(b)
				} else {
					b, _ := json.Marshal(msg)
					msgJSON = string(b)
				}
				receivedJSONs = append(receivedJSONs, msgJSON)
				recordMessage("receive", msgJSON, false)
			}
			headerMD, _ = bidiStream.Header()
			trailerMD = bidiStream.Trailer()
			body = "[" + strings.Join(receivedJSONs, ",\n") + "]"
		}
	}

	totalDuration := time.Since(start)
	st, _ := status.FromError(callErr)
	if st == nil {
		grpcStatusCode = 0
		grpcStatusText = "OK"
		httpStatusCode = 200
	} else {
		grpcStatusCode = int(st.Code())
		grpcStatusText = st.Code().String()
		httpStatusCode, _ = mapGrpcStatus(st)
	}

	resHeaders := make(map[string][]string)
	for k, v := range headerMD {
		resHeaders[k] = v
	}
	resTrailers := make(map[string][]string)
	for k, v := range trailerMD {
		resTrailers[k] = v
	}

	var errMsg string
	if callErr != nil && st.Code() != codes.OK {
		errMsg = st.Message()
		if errMsg == "" {
			errMsg = callErr.Error()
		}
	}

	return &types.ExecutionResult{
		StatusCode:     httpStatusCode,
		StatusText:     fmt.Sprintf("%d %s", grpcStatusCode, grpcStatusText),
		Headers:        resHeaders,
		Body:           body,
		Size:           int64(len(body)),
		GrpcStatus:     &grpcStatusCode,
		GrpcStatusText: grpcStatusText,
		GrpcMetadata:   resHeaders,
		GrpcTrailers:   resTrailers,
		GrpcMessages:   streamMessages,
		Timing: types.TimingMetrics{
			TotalDurationMs: float64(totalDuration.Milliseconds()),
		},
		ExecutedAt: time.Now(),
		Error:      errMsg,
	}, nil
}

func createMessageFromJSON(md *desc.MessageDescriptor, jsonStr string) (*dynamic.Message, error) {
	msg := dynamic.NewMessage(md)
	trimmed := strings.TrimSpace(jsonStr)
	if trimmed == "" {
		trimmed = "{}"
	}
	if err := msg.UnmarshalJSON([]byte(trimmed)); err != nil {
		return nil, fmt.Errorf("invalid JSON message for %s: %w", md.GetName(), err)
	}
	return msg, nil
}

func errorExecutionResult(err error, start time.Time) *types.ExecutionResult {
	code := int(codes.InvalidArgument)
	return &types.ExecutionResult{
		StatusCode:     400,
		StatusText:     "3 INVALID_ARGUMENT",
		Error:          err.Error(),
		ExecutedAt:     time.Now(),
		Timing:         types.TimingMetrics{TotalDurationMs: float64(time.Since(start).Milliseconds())},
		Logs:           []string{"[ERROR] " + err.Error()},
		GrpcStatus:     &code,
		GrpcStatusText: "INVALID_ARGUMENT",
	}
}

func mapGrpcStatus(st *status.Status) (int, string) {
	if st == nil || st.Code() == codes.OK {
		return 200, "0 OK"
	}
	code := int(st.Code())
	httpCode := 500
	switch st.Code() {
	case codes.OK:
		httpCode = 200
	case codes.Canceled:
		httpCode = 499
	case codes.InvalidArgument:
		httpCode = 400
	case codes.DeadlineExceeded:
		httpCode = 504
	case codes.NotFound:
		httpCode = 404
	case codes.AlreadyExists:
		httpCode = 409
	case codes.PermissionDenied:
		httpCode = 403
	case codes.Unauthenticated:
		httpCode = 401
	case codes.ResourceExhausted:
		httpCode = 429
	case codes.Unimplemented:
		httpCode = 501
	case codes.Unavailable:
		httpCode = 503
	}
	return httpCode, fmt.Sprintf("%d %s", code, st.Code().String())
}
