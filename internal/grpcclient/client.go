package grpcclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"pebblepost/internal/types"
)

// Client handles gRPC dynamic execution, reflection, and proto discovery.
type Client struct {
	workspaceRoot string
	mu            sync.Mutex
}

// NewClient creates a new gRPC client instance.
func NewClient(workspaceRoot string) *Client {
	return &Client{
		workspaceRoot: workspaceRoot,
	}
}

// SetWorkspace updates the workspace root path.
func (c *Client) SetWorkspace(ws string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workspaceRoot = ws
}

// Dial creates a new gRPC connection configured with TLS or plaintext.
func (c *Client) Dial(ctx context.Context, address string, useTLS bool, insecureSkipVerify bool, rootCAPath string) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	if useTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		}
		if rootCAPath != "" {
			caPath := rootCAPath
			if !filepath.IsAbs(caPath) && c.workspaceRoot != "" {
				caPath = filepath.Join(c.workspaceRoot, caPath)
			}
			caCert, err := os.ReadFile(caPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read root CA certificate: %w", err)
			}
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to append root CA certificate to pool")
			}
			tlsConfig.RootCAs = caCertPool
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial gRPC address %q: %w", address, err)
	}
	return conn, nil
}

// ReflectServices inspects the target address using gRPC reflection (v1 and v1alpha)
// and returns all available services and methods.
func (c *Client) ReflectServices(ctx context.Context, address string, useTLS bool, insecureSkipVerify bool, rootCAPath string) ([]types.GrpcServiceInfo, error) {
	conn, err := c.Dial(ctx, address, useTLS, insecureSkipVerify, rootCAPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	refClient := grpcreflect.NewClientAuto(ctx, conn)
	defer refClient.Reset()

	services, err := refClient.ListServices()
	if err != nil {
		return nil, fmt.Errorf("reflection failed on %q: %w", address, err)
	}

	var result []types.GrpcServiceInfo
	for _, svcName := range services {
		// Filter out standard internal reflection service names
		if svcName == "grpc.reflection.v1alpha.ServerReflection" || svcName == "grpc.reflection.v1.ServerReflection" {
			continue
		}

		svcDesc, err := refClient.ResolveService(svcName)
		if err != nil {
			continue
		}

		info := types.GrpcServiceInfo{
			Name: svcDesc.GetFullyQualifiedName(),
		}
		for _, m := range svcDesc.GetMethods() {
			info.Methods = append(info.Methods, types.GrpcMethodInfo{
				Name:            m.GetName(),
				FullMethod:      fmt.Sprintf("/%s/%s", svcDesc.GetFullyQualifiedName(), m.GetName()),
				ClientStreaming: m.IsClientStreaming(),
				ServerStreaming: m.IsServerStreaming(),
				InputType:       m.GetInputType().GetFullyQualifiedName(),
				OutputType:      m.GetOutputType().GetFullyQualifiedName(),
			})
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// LoadProtoServices parses .proto files from the workspace (or absolute paths)
// using protoparse with configured import paths.
func (c *Client) LoadProtoServices(protoFiles []string, importPaths []string) ([]types.GrpcServiceInfo, error) {
	if len(protoFiles) == 0 {
		return nil, fmt.Errorf("no .proto files specified")
	}

	var resolvedImportPaths []string
	if c.workspaceRoot != "" {
		resolvedImportPaths = append(resolvedImportPaths, c.workspaceRoot)
	}
	for _, ip := range importPaths {
		if filepath.IsAbs(ip) {
			resolvedImportPaths = append(resolvedImportPaths, ip)
		} else if c.workspaceRoot != "" {
			resolvedImportPaths = append(resolvedImportPaths, filepath.Join(c.workspaceRoot, ip))
		} else {
			resolvedImportPaths = append(resolvedImportPaths, ip)
		}
	}
	for _, pf := range protoFiles {
		dir := filepath.Dir(pf)
		if dir != "." && dir != "" {
			if filepath.IsAbs(dir) {
				resolvedImportPaths = append(resolvedImportPaths, dir)
			} else if c.workspaceRoot != "" {
				resolvedImportPaths = append(resolvedImportPaths, filepath.Join(c.workspaceRoot, dir))
			} else {
				resolvedImportPaths = append(resolvedImportPaths, dir)
			}
		}
	}
	if len(resolvedImportPaths) == 0 {
		resolvedImportPaths = []string{"."}
	}

	parser := protoparse.Parser{
		ImportPaths:           resolvedImportPaths,
		InferImportPaths:      true,
		IncludeSourceCodeInfo: true,
	}

	resolvedFiles, err := protoparse.ResolveFilenames(resolvedImportPaths, protoFiles...)
	if err != nil {
		resolvedFiles = protoFiles
	}

	fds, err := parser.ParseFiles(resolvedFiles...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proto files: %w", err)
	}

	var result []types.GrpcServiceInfo
	for _, fd := range fds {
		for _, svc := range fd.GetServices() {
			info := types.GrpcServiceInfo{
				Name: svc.GetFullyQualifiedName(),
			}
			for _, m := range svc.GetMethods() {
				info.Methods = append(info.Methods, types.GrpcMethodInfo{
					Name:            m.GetName(),
					FullMethod:      fmt.Sprintf("/%s/%s", svc.GetFullyQualifiedName(), m.GetName()),
					ClientStreaming: m.IsClientStreaming(),
					ServerStreaming: m.IsServerStreaming(),
					InputType:       m.GetInputType().GetFullyQualifiedName(),
					OutputType:      m.GetOutputType().GetFullyQualifiedName(),
				})
			}
			result = append(result, info)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// ResolveMethodDescriptor resolves a desc.MethodDescriptor for the given gRPC definition,
// either via reflection (connecting to server) or by parsing workspace .proto files.
// It returns the method descriptor, the ClientConn, and a cleanup function.
func (c *Client) ResolveMethodDescriptor(ctx context.Context, g *types.GrpcDefinition) (*desc.MethodDescriptor, *grpc.ClientConn, func(), error) {
	if g == nil {
		return nil, nil, nil, fmt.Errorf("grpc definition is nil")
	}

	var conn *grpc.ClientConn
	cleanup := func() {
		if conn != nil {
			_ = conn.Close()
		}
	}

	if g.ProtoSource == "file" && len(g.ProtoFiles) > 0 {
		if g.Address != "" {
			var dialErr error
			conn, dialErr = c.Dial(ctx, g.Address, g.UseTLS, g.InsecureSkipVerify, g.RootCAPath)
			if dialErr != nil {
				return nil, nil, nil, dialErr
			}
		}

		var resolvedImportPaths []string
		if c.workspaceRoot != "" {
			resolvedImportPaths = append(resolvedImportPaths, c.workspaceRoot)
		}
		for _, ip := range g.ImportPaths {
			if filepath.IsAbs(ip) {
				resolvedImportPaths = append(resolvedImportPaths, ip)
			} else if c.workspaceRoot != "" {
				resolvedImportPaths = append(resolvedImportPaths, filepath.Join(c.workspaceRoot, ip))
			} else {
				resolvedImportPaths = append(resolvedImportPaths, ip)
			}
		}
		for _, pf := range g.ProtoFiles {
			dir := filepath.Dir(pf)
			if dir != "." && dir != "" {
				if filepath.IsAbs(dir) {
					resolvedImportPaths = append(resolvedImportPaths, dir)
				} else if c.workspaceRoot != "" {
					resolvedImportPaths = append(resolvedImportPaths, filepath.Join(c.workspaceRoot, dir))
				} else {
					resolvedImportPaths = append(resolvedImportPaths, dir)
				}
			}
		}
		if len(resolvedImportPaths) == 0 {
			resolvedImportPaths = []string{"."}
		}

		parser := protoparse.Parser{
			ImportPaths:      resolvedImportPaths,
			InferImportPaths: true,
		}
		resolvedFiles, err := protoparse.ResolveFilenames(resolvedImportPaths, g.ProtoFiles...)
		if err != nil {
			resolvedFiles = g.ProtoFiles
		}

		fds, err := parser.ParseFiles(resolvedFiles...)
		if err != nil {
			cleanup()
			return nil, nil, nil, fmt.Errorf("failed to parse proto files: %w", err)
		}
		for _, fd := range fds {
			for _, svc := range fd.GetServices() {
				if svc.GetFullyQualifiedName() == g.Service || svc.GetName() == g.Service {
					for _, m := range svc.GetMethods() {
						if m.GetName() == g.Method || fmt.Sprintf("/%s/%s", svc.GetFullyQualifiedName(), m.GetName()) == g.Method {
							return m, conn, cleanup, nil
						}
					}
				}
			}
		}
		cleanup()
		return nil, nil, nil, fmt.Errorf("method %q on service %q not found in proto files", g.Method, g.Service)
	}

	// Default: Server Reflection (address required)
	if g.Address == "" {
		return nil, nil, nil, fmt.Errorf("grpc address is required for reflection")
	}
	var dialErr error
	conn, dialErr = c.Dial(ctx, g.Address, g.UseTLS, g.InsecureSkipVerify, g.RootCAPath)
	if dialErr != nil {
		return nil, nil, nil, dialErr
	}

	refClient := grpcreflect.NewClientAuto(ctx, conn)
	svcDesc, err := refClient.ResolveService(g.Service)
	if err != nil {
		cleanup()
		return nil, nil, nil, fmt.Errorf("failed to resolve service %q via reflection: %w", g.Service, err)
	}

	for _, m := range svcDesc.GetMethods() {
		if m.GetName() == g.Method || fmt.Sprintf("/%s/%s", svcDesc.GetFullyQualifiedName(), m.GetName()) == g.Method {
			return m, conn, cleanup, nil
		}
	}

	cleanup()
	return nil, nil, nil, fmt.Errorf("method %q not found in service %q via reflection", g.Method, g.Service)
}

// GenerateSampleMessage returns a sample JSON message for the specified method.
func (c *Client) GenerateSampleMessage(ctx context.Context, g *types.GrpcDefinition) (string, error) {
	methodDesc, _, cleanup, err := c.ResolveMethodDescriptor(ctx, g)
	if err != nil {
		return "{}", err
	}
	defer cleanup()

	inputDesc := methodDesc.GetInputType()
	return GenerateSampleJSON(inputDesc)
}
