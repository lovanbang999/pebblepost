package scripting

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"pebblepost/internal/types"
)

// PreRequestResult holds changes produced by a pre-request script execution.
type PreRequestResult struct {
	Request          *types.RequestDefinition `json:"request"`
	Logs             []string                 `json:"logs"`
	ExtractedEnvVars map[string]string        `json:"extractedEnvVars"`
}

// PostResponseResult holds test outputs and extracted variables from a post-response script.
type PostResponseResult struct {
	Tests            []types.TestAssertionResult `json:"tests"`
	Logs             []string                    `json:"logs"`
	ExtractedEnvVars map[string]string           `json:"extractedEnvVars"`
}

// Engine executes JavaScript scripts within isolated sandboxes.
type Engine struct {
	crypto *CryptoModule
}

// NewEngine creates a new Scripting Engine.
func NewEngine() *Engine {
	return &Engine{
		crypto: NewCryptoModule(),
	}
}

// ExecutePreRequest runs a pre-request script and applies mutations to the request definition and environment.
func (e *Engine) ExecutePreRequest(
	script string,
	req *types.RequestDefinition,
	vars map[string]string,
) (*PreRequestResult, error) {
	if strings.TrimSpace(script) == "" {
		return &PreRequestResult{
			Request:          req,
			Logs:             []string{},
			ExtractedEnvVars: make(map[string]string),
		}, nil
	}

	vm := goja.New()

	var (
		logsMutex sync.Mutex
		logs      = make([]string, 0)
		envVars   = make(map[string]string)
	)

	// Copy incoming variables
	for k, v := range vars {
		envVars[k] = v
	}

	// 1. Setup Console
	e.setupConsole(vm, &logs, &logsMutex)

	// 2. Setup Assertion Library JS
	if _, err := vm.RunString(AssertionLibraryJS); err != nil {
		return nil, fmt.Errorf("failed to initialize assertion library: %w", err)
	}

	// 3. Setup Request Bridge
	reqObj := e.setupRequestBridge(vm, req)

	// 4. Setup Environment Bridge
	envObj := e.setupEnvironmentBridge(vm, envVars)

	// 5. Setup Crypto Bridge
	cryptoObj := e.setupCryptoBridge(vm)

	// 6. Bind pb and pm objects
	pbObj := vm.NewObject()
	_ = pbObj.Set("request", reqObj)
	_ = pbObj.Set("environment", envObj)
	_ = pbObj.Set("variables", envObj)
	_ = pbObj.Set("crypto", cryptoObj)
	_ = pbObj.Set("expect", vm.Get("expect"))

	_ = vm.Set("pb", pbObj)
	_ = vm.Set("pm", pbObj) // Postman compatibility

	// 7. Run Script with Timeout
	errChan := make(chan error, 1)
	go func() {
		_, runErr := vm.RunString(script)
		errChan <- runErr
	}()

	select {
	case err := <-errChan:
		if err != nil {
			logsMutex.Lock()
			logs = append(logs, fmt.Sprintf("[Pre-request Error] %v", err))
			logsMutex.Unlock()
			return &PreRequestResult{
				Request:          req,
				Logs:             logs,
				ExtractedEnvVars: envVars,
			}, fmt.Errorf("pre-request script error: %w", err)
		}
	case <-time.After(3 * time.Second):
		vm.Interrupt("Script execution timed out (3s)")
		return nil, fmt.Errorf("pre-request script timed out")
	}

	return &PreRequestResult{
		Request:          req,
		Logs:             logs,
		ExtractedEnvVars: envVars,
	}, nil
}

// ExecutePostResponse runs a post-response test script, executing assertions and tests.
func (e *Engine) ExecutePostResponse(
	script string,
	req *types.RequestDefinition,
	resp *types.ExecutionResult,
	vars map[string]string,
) (*PostResponseResult, error) {
	if strings.TrimSpace(script) == "" {
		return &PostResponseResult{
			Tests:            make([]types.TestAssertionResult, 0),
			Logs:             make([]string, 0),
			ExtractedEnvVars: make(map[string]string),
		}, nil
	}

	vm := goja.New()

	var (
		logsMutex sync.Mutex
		logs      = make([]string, 0)
		tests     = make([]types.TestAssertionResult, 0)
		envVars   = make(map[string]string)
	)

	// Copy incoming variables
	for k, v := range vars {
		envVars[k] = v
	}

	// 1. Setup Console
	e.setupConsole(vm, &logs, &logsMutex)

	// 2. Setup Assertion Library JS
	if _, err := vm.RunString(AssertionLibraryJS); err != nil {
		return nil, fmt.Errorf("failed to initialize assertion library: %w", err)
	}

	// 3. Setup Response Bridge
	respObj := e.setupResponseBridge(vm, resp)

	// 4. Setup Environment Bridge
	envObj := e.setupEnvironmentBridge(vm, envVars)

	// 5. Setup Crypto Bridge
	cryptoObj := e.setupCryptoBridge(vm)

	// 6. Setup Test function
	testFn := func(name string, fn goja.Callable) {
		if fn == nil {
			tests = append(tests, types.TestAssertionResult{
				Name:    name,
				Passed:  false,
				Message: "Test function cannot be null",
			})
			return
		}

		_, err := fn(goja.Undefined())
		if err != nil {
			tests = append(tests, types.TestAssertionResult{
				Name:    name,
				Passed:  false,
				Message: err.Error(),
			})
		} else {
			tests = append(tests, types.TestAssertionResult{
				Name:   name,
				Passed: true,
			})
		}
	}

	// 7. Bind pb and pm objects
	pbObj := vm.NewObject()
	_ = pbObj.Set("response", respObj)
	_ = pbObj.Set("environment", envObj)
	_ = pbObj.Set("variables", envObj)
	_ = pbObj.Set("crypto", cryptoObj)
	_ = pbObj.Set("test", testFn)
	_ = pbObj.Set("expect", vm.Get("expect"))

	_ = vm.Set("pb", pbObj)
	_ = vm.Set("pm", pbObj)

	// 8. Run Script with Timeout
	errChan := make(chan error, 1)
	go func() {
		_, runErr := vm.RunString(script)
		errChan <- runErr
	}()

	select {
	case err := <-errChan:
		if err != nil {
			logsMutex.Lock()
			logs = append(logs, fmt.Sprintf("[Test Script Error] %v", err))
			logsMutex.Unlock()
			return &PostResponseResult{
				Tests:            tests,
				Logs:             logs,
				ExtractedEnvVars: envVars,
			}, fmt.Errorf("post-response script error: %w", err)
		}
	case <-time.After(3 * time.Second):
		vm.Interrupt("Script execution timed out (3s)")
		return nil, fmt.Errorf("post-response script timed out")
	}

	return &PostResponseResult{
		Tests:            tests,
		Logs:             logs,
		ExtractedEnvVars: envVars,
	}, nil
}

func (e *Engine) setupConsole(vm *goja.Runtime, logs *[]string, mutex *sync.Mutex) {
	console := vm.NewObject()

	logFn := func(call goja.FunctionCall) goja.Value {
		var parts []string
		for _, arg := range call.Arguments {
			parts = append(parts, arg.String())
		}
		msg := strings.Join(parts, " ")
		mutex.Lock()
		*logs = append(*logs, msg)
		mutex.Unlock()
		return goja.Undefined()
	}

	_ = console.Set("log", logFn)
	_ = console.Set("info", logFn)
	_ = console.Set("warn", logFn)
	_ = console.Set("error", logFn)

	_ = vm.Set("console", console)
}

func (e *Engine) setupRequestBridge(vm *goja.Runtime, req *types.RequestDefinition) *goja.Object {
	reqObj := vm.NewObject()

	if req == nil {
		return reqObj
	}

	_ = reqObj.Set("url", req.URL)
	_ = reqObj.Set("method", req.Method)

	// Headers manager
	headersObj := vm.NewObject()
	_ = headersObj.Set("add", func(key, value string) {
		req.Headers = append(req.Headers, types.KeyValue{
			Key:     key,
			Value:   value,
			Enabled: true,
		})
	})
	_ = headersObj.Set("set", func(key, value string) {
		found := false
		for i, h := range req.Headers {
			if strings.EqualFold(h.Key, key) {
				req.Headers[i].Value = value
				req.Headers[i].Enabled = true
				found = true
				break
			}
		}
		if !found {
			req.Headers = append(req.Headers, types.KeyValue{
				Key:     key,
				Value:   value,
				Enabled: true,
			})
		}
	})
	_ = headersObj.Set("get", func(key string) string {
		for _, h := range req.Headers {
			if h.Enabled && strings.EqualFold(h.Key, key) {
				return h.Value
			}
		}
		return ""
	})
	_ = headersObj.Set("remove", func(key string) {
		filtered := make([]types.KeyValue, 0)
		for _, h := range req.Headers {
			if !strings.EqualFold(h.Key, key) {
				filtered = append(filtered, h)
			}
		}
		req.Headers = filtered
	})
	_ = reqObj.Set("headers", headersObj)

	// Body manager
	bodyObj := vm.NewObject()
	_ = bodyObj.Set("raw", req.Body.Raw)
	_ = bodyObj.Set("setRaw", func(val string) {
		req.Body.Raw = val
	})
	_ = reqObj.Set("body", bodyObj)

	return reqObj
}

func (e *Engine) setupResponseBridge(vm *goja.Runtime, resp *types.ExecutionResult) *goja.Object {
	respObj := vm.NewObject()

	if resp == nil {
		return respObj
	}

	_ = respObj.Set("code", resp.StatusCode)
	_ = respObj.Set("status", resp.StatusCode)
	_ = respObj.Set("statusText", resp.StatusText)
	_ = respObj.Set("responseTime", resp.Timing.TotalDurationMs)
	_ = respObj.Set("time", resp.Timing.TotalDurationMs)
	_ = respObj.Set("duration", resp.Timing.TotalDurationMs)
	_ = respObj.Set("body", resp.Body)

	_ = respObj.Set("text", func() string {
		return resp.Body
	})

	_ = respObj.Set("json", func() goja.Value {
		var parsed any
		if err := json.Unmarshal([]byte(resp.Body), &parsed); err != nil {
			panic(vm.ToValue(fmt.Sprintf("Failed to parse response body as JSON: %v", err)))
		}
		return vm.ToValue(parsed)
	})

	// Response Headers manager
	headersObj := vm.NewObject()
	_ = headersObj.Set("get", func(key string) string {
		for hKey, values := range resp.Headers {
			if strings.EqualFold(hKey, key) && len(values) > 0 {
				return values[0]
			}
		}
		return ""
	})
	_ = headersObj.Set("has", func(key string) bool {
		for hKey := range resp.Headers {
			if strings.EqualFold(hKey, key) {
				return true
			}
		}
		return false
	})
	_ = respObj.Set("headers", headersObj)

	return respObj
}

func (e *Engine) setupEnvironmentBridge(vm *goja.Runtime, envVars map[string]string) *goja.Object {
	envObj := vm.NewObject()

	_ = envObj.Set("get", func(key string) string {
		return envVars[key]
	})

	_ = envObj.Set("set", func(key string, value any) {
		envVars[key] = fmt.Sprintf("%v", value)
	})

	_ = envObj.Set("has", func(key string) bool {
		_, exists := envVars[key]
		return exists
	})

	_ = envObj.Set("unset", func(key string) {
		delete(envVars, key)
	})

	_ = envObj.Set("toObject", func() map[string]string {
		return envVars
	})

	return envObj
}

func (e *Engine) setupCryptoBridge(vm *goja.Runtime) *goja.Object {
	cryptoObj := vm.NewObject()

	_ = cryptoObj.Set("md5", func(input string) string {
		return e.crypto.MD5(input)
	})
	_ = cryptoObj.Set("sha256", func(input string) string {
		return e.crypto.SHA256(input)
	})
	_ = cryptoObj.Set("sha512", func(input string) string {
		return e.crypto.SHA512(input)
	})
	_ = cryptoObj.Set("hmac", func(algo, secret, message string) string {
		res, err := e.crypto.HMAC(algo, secret, message)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return res
	})
	_ = cryptoObj.Set("uuid", func() string {
		return e.crypto.UUID()
	})
	_ = cryptoObj.Set("uuidv4", func() string {
		return e.crypto.UUID()
	})
	_ = cryptoObj.Set("base64Encode", func(input string) string {
		return e.crypto.Base64Encode(input)
	})
	_ = cryptoObj.Set("base64Decode", func(input string) string {
		res, err := e.crypto.Base64Decode(input)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return res
	})

	return cryptoObj
}
