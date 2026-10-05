package scripting

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"pebblepost/internal/security"
	"pebblepost/internal/types"
)

// DefaultScriptTimeout is used when the request's ScriptTimeoutMs is 0.
const DefaultScriptTimeout = 5 * time.Second

// maxConsoleLogs caps the number of console.log entries per script run.
const maxConsoleLogs = 500

// maxLogEntryLen caps the character length of a single console log line.
const maxLogEntryLen = 4096

// FlowControlAction tracks dynamic execution changes requested by pb.runner.
type FlowControlAction struct {
	NextRequest *string `json:"nextRequest,omitempty"`
	StopAll     bool    `json:"stopAll,omitempty"`
}

// IterationContext holds iteration parameters for data-driven runs and flow control.
type IterationContext struct {
	Iteration      int            `json:"iteration"`
	IterationCount int            `json:"iterationCount"`
	DataRow        map[string]any `json:"dataRow,omitempty"`
}

// PreRequestResult holds changes produced by a pre-request script execution.
type PreRequestResult struct {
	Request          *types.RequestDefinition `json:"request"`
	Logs             []string                 `json:"logs"`
	ConsoleLogs      []types.ConsoleLogEntry  `json:"consoleLogs"`
	ExtractedEnvVars map[string]string        `json:"extractedEnvVars"`
	LocalVars        map[string]string        `json:"localVars,omitempty"`
	NextRequest      *string                  `json:"nextRequest,omitempty"`
	StopAll          bool                     `json:"stopAll,omitempty"`
}

// PostResponseResult holds test outputs and extracted variables from a post-response script.
type PostResponseResult struct {
	Tests            []types.TestAssertionResult `json:"tests"`
	Logs             []string                    `json:"logs"`
	ConsoleLogs      []types.ConsoleLogEntry     `json:"consoleLogs"`
	ExtractedEnvVars map[string]string           `json:"extractedEnvVars"`
	LocalVars        map[string]string           `json:"localVars,omitempty"`
	NextRequest      *string                     `json:"nextRequest,omitempty"`
	StopAll          bool                        `json:"stopAll,omitempty"`
}

// Engine executes JavaScript scripts within isolated sandboxes.
type Engine struct {
	crypto    *CryptoModule
	date      *DateModule
	random    *RandomModule
	auxiliary *AuxiliaryClient
}

// NewEngine creates a new Scripting Engine.
func NewEngine() *Engine {
	return &Engine{
		crypto:    NewCryptoModule(),
		date:      NewDateModule(),
		random:    NewRandomModule(),
		auxiliary: NewAuxiliaryClient(),
	}
}

// ExecutePreRequest executes a pre-request script with default source name "Pre-request".
func (e *Engine) ExecutePreRequest(
	script string,
	req *types.RequestDefinition,
	vars map[string]string,
	timeout time.Duration,
) (*PreRequestResult, error) {
	return e.ExecutePreRequestNamed("Pre-request", script, req, vars, timeout)
}

// ExecutePreRequestNamed executes a pre-request script with a specific source name.
func (e *Engine) ExecutePreRequestNamed(
	sourceName string,
	script string,
	req *types.RequestDefinition,
	vars map[string]string,
	timeout time.Duration,
) (*PreRequestResult, error) {
	return e.ExecutePreRequestWithContext(sourceName, script, req, vars, nil, timeout)
}

// ExecutePreRequestWithContext executes a pre-request script with iteration context and flow control.
func (e *Engine) ExecutePreRequestWithContext(
	sourceName string,
	script string,
	req *types.RequestDefinition,
	vars map[string]string,
	iterCtx *IterationContext,
	timeout time.Duration,
) (*PreRequestResult, error) {
	if strings.TrimSpace(script) == "" {
		return &PreRequestResult{
			Request:          req,
			Logs:             []string{},
			ConsoleLogs:      []types.ConsoleLogEntry{},
			ExtractedEnvVars: make(map[string]string),
			LocalVars:        make(map[string]string),
		}, nil
	}

	if sourceName == "" {
		sourceName = "Pre-request"
	}

	// 1. Static syntax & Goja limitations validation
	if err := ValidateScriptSyntax(sourceName, script); err != nil {
		syntaxErrLog := types.ConsoleLogEntry{
			Timestamp: time.Now(),
			Level:     "error",
			Source:    sourceName,
			Message:   err.Error(),
		}
		return &PreRequestResult{
			Request:          req,
			Logs:             []string{fmt.Sprintf("[%s Error] %s", sourceName, err.Error())},
			ConsoleLogs:      []types.ConsoleLogEntry{syntaxErrLog},
			ExtractedEnvVars: make(map[string]string),
			LocalVars:        make(map[string]string),
		}, fmt.Errorf("pre-request script syntax error: %w", err)
	}

	if timeout <= 0 {
		timeout = DefaultScriptTimeout
	}

	vm := goja.New()

	var (
		logsMutex   sync.Mutex
		logs        = make([]string, 0)
		consoleLogs = make([]types.ConsoleLogEntry, 0)
		envVars     = make(map[string]string)
		localVars   = make(map[string]string)
		action      FlowControlAction
	)

	// Copy incoming variables
	for k, v := range vars {
		envVars[k] = v
	}

	// 2. Setup Console
	e.setupConsole(vm, &logs, &consoleLogs, &logsMutex, sourceName)

	// 3. Setup Assertion Library JS
	if _, err := vm.RunString(AssertionLibraryJS); err != nil {
		return nil, fmt.Errorf("failed to initialize assertion library: %w", err)
	}

	// 4. Setup Bridges
	reqObj := e.setupRequestBridge(vm, req)
	envObj := e.setupEnvironmentBridge(vm, envVars)
	varObj := e.setupVariablesBridge(vm, localVars, envVars)
	cryptoObj := e.setupCryptoBridge(vm)
	runnerObj := e.setupRunnerBridge(vm, &action)
	dataObj := e.setupDataBridge(vm, iterCtx)
	infoObj := e.setupInfoBridge(vm, iterCtx)

	// 5. Bind pb and pm objects
	pbObj := vm.NewObject()
	_ = pbObj.Set("request", reqObj)
	_ = pbObj.Set("environment", envObj)
	_ = pbObj.Set("variables", varObj)
	_ = pbObj.Set("crypto", cryptoObj)
	_ = pbObj.Set("expect", vm.Get("expect"))
	_ = pbObj.Set("runner", runnerObj)
	_ = pbObj.Set("data", dataObj)
	_ = pbObj.Set("info", infoObj)
	_ = pbObj.Set("iterationData", dataObj) // Postman compatibility

	// postman.setNextRequest compatibility
	postmanObj := vm.NewObject()
	_ = postmanObj.Set("setNextRequest", runnerObj.Get("setNextRequest"))
	_ = vm.Set("postman", postmanObj)

	var secretsToMask []string
	if req != nil {
		if req.Auth.Token != "" {
			secretsToMask = append(secretsToMask, req.Auth.Token)
		}
		if req.Auth.Password != "" {
			secretsToMask = append(secretsToMask, req.Auth.Password)
		}
	}
	for _, v := range vars {
		if len(v) >= 3 {
			secretsToMask = append(secretsToMask, v)
		}
	}

	// Bind extended pb utilities
	e.bindExtendedPbAPIs(vm, pbObj, timeout, secretsToMask)

	_ = vm.Set("pb", pbObj)
	_ = vm.Set("pm", pbObj) // Postman compatibility

	// 6. Run Script with Timeout
	errChan := make(chan error, 1)
	go func() {
		_, runErr := vm.RunString(script)
		errChan <- runErr
	}()

	select {
	case err := <-errChan:
		if err != nil {
			formattedErr := formatScriptError(sourceName, err)
			logsMutex.Lock()
			logs = appendLog(logs, fmt.Sprintf("[%s Error] %s", sourceName, formattedErr))
			consoleLogs = append(consoleLogs, types.ConsoleLogEntry{
				Timestamp: time.Now(),
				Level:     "error",
				Source:    sourceName,
				Message:   formattedErr,
			})
			logsMutex.Unlock()

			return &PreRequestResult{
				Request:          req,
				Logs:             logs,
				ConsoleLogs:      consoleLogs,
				ExtractedEnvVars: envVars,
				LocalVars:        localVars,
				NextRequest:      action.NextRequest,
				StopAll:          action.StopAll,
			}, fmt.Errorf("pre-request script error: %w", err)
		}
	case <-time.After(timeout):
		vm.Interrupt(fmt.Sprintf("Script execution timed out (%s)", timeout))
		timeoutMsg := fmt.Sprintf("[%s] execution timed out after %s", sourceName, timeout)
		consoleLogs = append(consoleLogs, types.ConsoleLogEntry{
			Timestamp: time.Now(),
			Level:     "error",
			Source:    sourceName,
			Message:   timeoutMsg,
		})
		return nil, errors.New(timeoutMsg)
	}

	return &PreRequestResult{
		Request:          req,
		Logs:             logs,
		ConsoleLogs:      consoleLogs,
		ExtractedEnvVars: envVars,
		LocalVars:        localVars,
		NextRequest:      action.NextRequest,
		StopAll:          action.StopAll,
	}, nil
}

// ExecutePostResponse runs a post-response test script with default source "Post-response".
func (e *Engine) ExecutePostResponse(
	script string,
	req *types.RequestDefinition,
	resp *types.ExecutionResult,
	vars map[string]string,
	timeout time.Duration,
) (*PostResponseResult, error) {
	return e.ExecutePostResponseNamed("Post-response", script, req, resp, vars, timeout)
}

// ExecutePostResponseNamed runs a post-response test script with a specific source name.
func (e *Engine) ExecutePostResponseNamed(
	sourceName string,
	script string,
	req *types.RequestDefinition,
	resp *types.ExecutionResult,
	vars map[string]string,
	timeout time.Duration,
) (*PostResponseResult, error) {
	return e.ExecutePostResponseWithContext(sourceName, script, req, resp, vars, nil, timeout)
}

// ExecutePostResponseWithContext runs a post-response test script with iteration context and flow control.
func (e *Engine) ExecutePostResponseWithContext(
	sourceName string,
	script string,
	req *types.RequestDefinition,
	resp *types.ExecutionResult,
	vars map[string]string,
	iterCtx *IterationContext,
	timeout time.Duration,
) (*PostResponseResult, error) {
	if strings.TrimSpace(script) == "" {
		return &PostResponseResult{
			Tests:            make([]types.TestAssertionResult, 0),
			Logs:             make([]string, 0),
			ConsoleLogs:      make([]types.ConsoleLogEntry, 0),
			ExtractedEnvVars: make(map[string]string),
			LocalVars:        make(map[string]string),
		}, nil
	}

	if sourceName == "" {
		sourceName = "Post-response"
	}

	// 1. Static syntax validation
	if err := ValidateScriptSyntax(sourceName, script); err != nil {
		syntaxErrLog := types.ConsoleLogEntry{
			Timestamp: time.Now(),
			Level:     "error",
			Source:    sourceName,
			Message:   err.Error(),
		}
		return &PostResponseResult{
			Tests: []types.TestAssertionResult{{
				Name:    fmt.Sprintf("%s Syntax Validation", sourceName),
				Passed:  false,
				Message: err.Error(),
			}},
			Logs:             []string{fmt.Sprintf("[%s Error] %s", sourceName, err.Error())},
			ConsoleLogs:      []types.ConsoleLogEntry{syntaxErrLog},
			ExtractedEnvVars: make(map[string]string),
			LocalVars:        make(map[string]string),
		}, fmt.Errorf("post-response script syntax error: %w", err)
	}

	if timeout <= 0 {
		timeout = DefaultScriptTimeout
	}

	vm := goja.New()

	var (
		logsMutex   sync.Mutex
		logs        = make([]string, 0)
		consoleLogs = make([]types.ConsoleLogEntry, 0)
		tests       = make([]types.TestAssertionResult, 0)
		envVars     = make(map[string]string)
		localVars   = make(map[string]string)
		action      FlowControlAction
	)

	// Copy incoming variables
	for k, v := range vars {
		envVars[k] = v
	}

	// 2. Setup Console
	e.setupConsole(vm, &logs, &consoleLogs, &logsMutex, sourceName)

	// 3. Setup Assertion Library JS
	if _, err := vm.RunString(AssertionLibraryJS); err != nil {
		return nil, fmt.Errorf("failed to initialize assertion library: %w", err)
	}

	// 4. Setup Bridges
	respObj := e.setupResponseBridge(vm, resp)
	envObj := e.setupEnvironmentBridge(vm, envVars)
	varObj := e.setupVariablesBridge(vm, localVars, envVars)
	cryptoObj := e.setupCryptoBridge(vm)
	runnerObj := e.setupRunnerBridge(vm, &action)
	dataObj := e.setupDataBridge(vm, iterCtx)
	infoObj := e.setupInfoBridge(vm, iterCtx)

	// 5. Setup Test runner function
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

	// 6. Bind pb and pm objects
	pbObj := vm.NewObject()
	_ = pbObj.Set("response", respObj)
	_ = pbObj.Set("environment", envObj)
	_ = pbObj.Set("variables", varObj)
	_ = pbObj.Set("crypto", cryptoObj)
	_ = pbObj.Set("test", testFn)
	_ = pbObj.Set("expect", vm.Get("expect"))
	_ = pbObj.Set("stream", e.setupStreamBridge(vm, resp))
	_ = pbObj.Set("runner", runnerObj)
	_ = pbObj.Set("data", dataObj)
	_ = pbObj.Set("info", infoObj)
	_ = pbObj.Set("iterationData", dataObj) // Postman compatibility

	// postman.setNextRequest compatibility
	postmanObj := vm.NewObject()
	_ = postmanObj.Set("setNextRequest", runnerObj.Get("setNextRequest"))
	_ = vm.Set("postman", postmanObj)

	var secretsToMask []string
	if req != nil {
		if req.Auth.Token != "" {
			secretsToMask = append(secretsToMask, req.Auth.Token)
		}
		if req.Auth.Password != "" {
			secretsToMask = append(secretsToMask, req.Auth.Password)
		}
	}
	for _, v := range vars {
		if len(v) >= 3 {
			secretsToMask = append(secretsToMask, v)
		}
	}

	// Bind extended pb utilities
	e.bindExtendedPbAPIs(vm, pbObj, timeout, secretsToMask)

	_ = vm.Set("pb", pbObj)
	_ = vm.Set("pm", pbObj)

	// 7. Run Script with Timeout
	errChan := make(chan error, 1)
	go func() {
		_, runErr := vm.RunString(script)
		errChan <- runErr
	}()

	select {
	case err := <-errChan:
		if err != nil {
			formattedErr := formatScriptError(sourceName, err)
			logsMutex.Lock()
			logs = appendLog(logs, fmt.Sprintf("[%s Error] %s", sourceName, formattedErr))
			consoleLogs = append(consoleLogs, types.ConsoleLogEntry{
				Timestamp: time.Now(),
				Level:     "error",
				Source:    sourceName,
				Message:   formattedErr,
			})
			logsMutex.Unlock()

			return &PostResponseResult{
				Tests:            tests,
				Logs:             logs,
				ConsoleLogs:      consoleLogs,
				ExtractedEnvVars: envVars,
				LocalVars:        localVars,
				NextRequest:      action.NextRequest,
				StopAll:          action.StopAll,
			}, fmt.Errorf("post-response script error: %w", err)
		}
	case <-time.After(timeout):
		vm.Interrupt(fmt.Sprintf("Script execution timed out (%s)", timeout))
		timeoutMsg := fmt.Sprintf("[%s] execution timed out after %s", sourceName, timeout)
		consoleLogs = append(consoleLogs, types.ConsoleLogEntry{
			Timestamp: time.Now(),
			Level:     "error",
			Source:    sourceName,
			Message:   timeoutMsg,
		})
		return nil, errors.New(timeoutMsg)
	}

	return &PostResponseResult{
		Tests:            tests,
		Logs:             logs,
		ConsoleLogs:      consoleLogs,
		ExtractedEnvVars: envVars,
		LocalVars:        localVars,
		NextRequest:      action.NextRequest,
		StopAll:          action.StopAll,
	}, nil
}

func (e *Engine) setupRunnerBridge(vm *goja.Runtime, action *FlowControlAction) *goja.Object {
	runnerObj := vm.NewObject()
	_ = runnerObj.Set("setNextRequest", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || goja.IsNull(call.Arguments[0]) || goja.IsUndefined(call.Arguments[0]) {
			empty := ""
			action.NextRequest = &empty
			return goja.Undefined()
		}
		target := strings.TrimSpace(call.Arguments[0].String())
		action.NextRequest = &target
		return goja.Undefined()
	})
	_ = runnerObj.Set("stop", func() {
		action.StopAll = true
	})
	return runnerObj
}

func (e *Engine) setupDataBridge(vm *goja.Runtime, iterCtx *IterationContext) *goja.Object {
	dataObj := vm.NewObject()
	var row map[string]any
	if iterCtx != nil && iterCtx.DataRow != nil {
		row = iterCtx.DataRow
		for k, v := range row {
			_ = dataObj.Set(k, v)
		}
	}
	_ = dataObj.Set("get", func(key string) goja.Value {
		if row != nil {
			if v, ok := row[key]; ok {
				return vm.ToValue(v)
			}
		}
		return goja.Undefined()
	})
	_ = dataObj.Set("toObject", func() goja.Value {
		if row == nil {
			return vm.NewObject()
		}
		return vm.ToValue(row)
	})
	return dataObj
}

func (e *Engine) setupInfoBridge(vm *goja.Runtime, iterCtx *IterationContext) *goja.Object {
	infoObj := vm.NewObject()
	if iterCtx != nil {
		_ = infoObj.Set("iteration", iterCtx.Iteration)
		_ = infoObj.Set("iterationCount", iterCtx.IterationCount)
	} else {
		_ = infoObj.Set("iteration", 0)
		_ = infoObj.Set("iterationCount", 1)
	}
	return infoObj
}

func (e *Engine) setupConsole(
	vm *goja.Runtime,
	logs *[]string,
	consoleLogs *[]types.ConsoleLogEntry,
	mutex *sync.Mutex,
	source string,
) {
	console := vm.NewObject()

	makeLogFn := func(level string) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			var parts []string
			for _, arg := range call.Arguments {
				if arg == nil || goja.IsUndefined(arg) {
					parts = append(parts, "undefined")
				} else if goja.IsNull(arg) {
					parts = append(parts, "null")
				} else {
					parts = append(parts, arg.String())
				}
			}
			msg := strings.Join(parts, " ")
			now := time.Now()

			mutex.Lock()
			formattedLog := fmt.Sprintf("[%s] [%s] [%s] %s", now.Format("15:04:05.000"), source, strings.ToUpper(level), msg)
			*logs = appendLog(*logs, formattedLog)
			*consoleLogs = append(*consoleLogs, types.ConsoleLogEntry{
				Timestamp: now,
				Level:     level,
				Source:    source,
				Message:   msg,
			})
			mutex.Unlock()
			return goja.Undefined()
		}
	}

	_ = console.Set("log", makeLogFn("log"))
	_ = console.Set("info", makeLogFn("info"))
	_ = console.Set("warn", makeLogFn("warn"))
	_ = console.Set("error", makeLogFn("error"))

	_ = vm.Set("console", console)
}

func (e *Engine) bindExtendedPbAPIs(vm *goja.Runtime, pbObj *goja.Object, timeout time.Duration, secretsToMask []string) {
	// 1. pb.uuid()
	_ = pbObj.Set("uuid", func() string {
		return e.crypto.UUID()
	})

	// 2. pb.base64
	base64Obj := vm.NewObject()
	_ = base64Obj.Set("encode", func(input string) string {
		return e.crypto.Base64Encode(input)
	})
	_ = base64Obj.Set("decode", func(input string) string {
		res, err := e.crypto.Base64Decode(input)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("Base64 decode failed: %v", err)))
		}
		return res
	})
	_ = pbObj.Set("base64", base64Obj)

	// 3. pb.hash
	hashObj := vm.NewObject()
	_ = hashObj.Set("sha256", func(input string) string {
		return e.crypto.SHA256(input)
	})
	_ = hashObj.Set("md5", func(input string) string {
		return e.crypto.MD5(input)
	})
	_ = pbObj.Set("hash", hashObj)

	// 4. pb.hmac
	hmacObj := vm.NewObject()
	_ = hmacObj.Set("sha256", func(secret, message string) string {
		res, err := e.crypto.HMAC("sha256", secret, message)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("HMAC SHA256 failed: %v", err)))
		}
		return res
	})
	_ = pbObj.Set("hmac", hmacObj)

	// 5. pb.jwt
	jwtObj := vm.NewObject()
	_ = jwtObj.Set("decode", func(token string) goja.Value {
		claims, err := e.crypto.JWTDecode(token)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("JWT decode failed: %v", err)))
		}
		return vm.ToValue(claims)
	})
	_ = pbObj.Set("jwt", jwtObj)

	// 6. pb.date
	dateObj := vm.NewObject()
	_ = dateObj.Set("now", func() int64 {
		return e.date.Now()
	})
	_ = dateObj.Set("nowISO", func() string {
		return e.date.NowISO()
	})
	_ = dateObj.Set("format", func(dateVal any, layout string) string {
		formatted, err := e.date.Format(dateVal, layout)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("Date format failed: %v", err)))
		}
		return formatted
	})
	_ = dateObj.Set("add", func(dateVal any, amount int64, unit string) string {
		added, err := e.date.Add(dateVal, amount, unit)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("Date add failed: %v", err)))
		}
		return added
	})
	_ = pbObj.Set("date", dateObj)

	// 7. pb.random
	randomObj := vm.NewObject()
	_ = randomObj.Set("int", func(min, max int64) int64 {
		val, err := e.random.Int(min, max)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("Random int failed: %v", err)))
		}
		return val
	})
	_ = randomObj.Set("string", func(length int, charset string) string {
		val, err := e.random.String(length, charset)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("Random string failed: %v", err)))
		}
		return val
	})
	_ = pbObj.Set("random", randomObj)

	// 8. pb.sendRequest(config) with call limit (max 5) and secret masking
	var sendRequestCount int
	_ = pbObj.Set("sendRequest", func(call goja.FunctionCall) goja.Value {
		sendRequestCount++
		if sendRequestCount > 5 {
			panic(vm.ToValue("pb.sendRequest limit exceeded (maximum 5 requests allowed per script execution)"))
		}

		if len(call.Arguments) == 0 {
			panic(vm.ToValue("pb.sendRequest requires a request configuration object"))
		}
		cfgVal := call.Arguments[0].Export()
		cfgMap, ok := cfgVal.(map[string]any)
		if !ok {
			// If string URL was provided
			if urlStr, isStr := cfgVal.(string); isStr {
				cfgMap = map[string]any{"url": urlStr}
			} else {
				panic(vm.ToValue("pb.sendRequest config must be an object or URL string"))
			}
		}

		res, err := e.auxiliary.SendRequest(cfgMap, timeout)
		if err != nil {
			maskedErr := security.MaskSecrets(err.Error(), secretsToMask)
			panic(vm.ToValue(fmt.Sprintf("pb.sendRequest failed: %v", maskedErr)))
		}

		resObj := vm.NewObject()
		_ = resObj.Set("status", res["status"])
		_ = resObj.Set("statusCode", res["status"])
		_ = resObj.Set("statusText", res["statusText"])
		_ = resObj.Set("duration", res["duration"])
		_ = resObj.Set("time", res["time"])
		_ = resObj.Set("body", res["body"])
		_ = resObj.Set("headers", res["headers"])
		_ = resObj.Set("text", func() string {
			return fmt.Sprintf("%v", res["body"])
		})
		_ = resObj.Set("json", func() goja.Value {
			var parsed any
			bodyStr := fmt.Sprintf("%v", res["body"])
			if err := json.Unmarshal([]byte(bodyStr), &parsed); err != nil {
				panic(vm.ToValue(fmt.Sprintf("Failed to parse auxiliary response body as JSON: %v", err)))
			}
			return vm.ToValue(parsed)
		})

		return resObj
	})
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
	_ = respObj.Set("size", resp.Size)
	_ = respObj.Set("body", resp.Body)

	_ = respObj.Set("text", func() string {
		return resp.Body
	})

	_ = respObj.Set("json", func() goja.Value {
		var parsed any
		if resp.Body != "" {
			if err := json.Unmarshal([]byte(resp.Body), &parsed); err == nil {
				return vm.ToValue(parsed)
			}
		}
		if len(resp.StreamLogs) > 0 {
			for i := len(resp.StreamLogs) - 1; i >= 0; i-- {
				if resp.StreamLogs[i].Direction == "receive" {
					if err := json.Unmarshal([]byte(resp.StreamLogs[i].Payload), &parsed); err == nil {
						return vm.ToValue(parsed)
					}
				}
			}
		}
		if resp.Body == "" {
			return vm.ToValue(nil)
		}
		if err := json.Unmarshal([]byte(resp.Body), &parsed); err != nil {
			panic(vm.ToValue(fmt.Sprintf("Failed to parse response body as JSON: %v", err)))
		}
		return vm.ToValue(parsed)
	})

	// Response Headers manager
	headersObj := vm.NewObject()
	for hKey, values := range resp.Headers {
		if len(values) > 0 {
			_ = headersObj.Set(strings.ToLower(hKey), values[0])
			_ = headersObj.Set(hKey, values[0])
		}
	}
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

	if len(resp.StreamLogs) > 0 {
		_ = respObj.Set("streamLogs", resp.StreamLogs)
		_ = respObj.Set("closeCode", resp.StreamCloseCode)
		_ = respObj.Set("closeReason", resp.StreamCloseReason)
		_ = respObj.Set("evicted", resp.StreamEvicted)
	}

	return respObj
}

func (e *Engine) setupStreamBridge(vm *goja.Runtime, resp *types.ExecutionResult) *goja.Object {
	streamObj := vm.NewObject()
	if resp == nil {
		return streamObj
	}

	_ = streamObj.Set("messages", func() []string {
		var msgs []string
		for _, l := range resp.StreamLogs {
			if l.Direction == "receive" {
				msgs = append(msgs, l.Payload)
			}
		}
		return msgs
	})

	_ = streamObj.Set("jsonMessages", func() goja.Value {
		var msgs []any
		for _, l := range resp.StreamLogs {
			if l.Direction == "receive" {
				var parsed any
				if err := json.Unmarshal([]byte(l.Payload), &parsed); err == nil {
					msgs = append(msgs, parsed)
				} else {
					msgs = append(msgs, l.Payload)
				}
			}
		}
		return vm.ToValue(msgs)
	})

	_ = streamObj.Set("logs", func() []types.StreamLogEntry {
		return resp.StreamLogs
	})

	_ = streamObj.Set("closeCode", func() int {
		return resp.StreamCloseCode
	})

	_ = streamObj.Set("closeReason", func() string {
		return resp.StreamCloseReason
	})

	_ = streamObj.Set("evicted", func() int {
		return resp.StreamEvicted
	})

	_ = streamObj.Set("waitFor", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		pred, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			return vm.ToValue(false)
		}
		for _, l := range resp.StreamLogs {
			if l.Direction == "receive" {
				var argVal goja.Value
				var parsed any
				if err := json.Unmarshal([]byte(l.Payload), &parsed); err == nil {
					argVal = vm.ToValue(parsed)
				} else {
					argVal = vm.ToValue(l.Payload)
				}
				res, err := pred(goja.Undefined(), argVal)
				if err == nil && res.ToBoolean() {
					return vm.ToValue(true)
				}
			}
		}
		return vm.ToValue(false)
	})

	return streamObj
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

func (e *Engine) setupVariablesBridge(vm *goja.Runtime, localVars map[string]string, envVars map[string]string) *goja.Object {
	varObj := vm.NewObject()

	_ = varObj.Set("get", func(key string) string {
		if val, exists := localVars[key]; exists {
			return val
		}
		return envVars[key]
	})
	_ = varObj.Set("set", func(key string, value any) {
		localVars[key] = fmt.Sprintf("%v", value)
	})
	_ = varObj.Set("has", func(key string) bool {
		if _, exists := localVars[key]; exists {
			return true
		}
		_, exists := envVars[key]
		return exists
	})
	_ = varObj.Set("unset", func(key string) {
		delete(localVars, key)
	})
	_ = varObj.Set("toObject", func() map[string]string {
		merged := make(map[string]string)
		for k, v := range envVars {
			merged[k] = v
		}
		for k, v := range localVars {
			merged[k] = v
		}
		return merged
	})

	return varObj
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

func appendLog(logs []string, msg string) []string {
	if len(logs) >= maxConsoleLogs {
		return logs
	}
	if len(msg) > maxLogEntryLen {
		msg = msg[:maxLogEntryLen] + "…[truncated]"
	}
	return append(logs, msg)
}

// formatScriptError extracts a clean, concise script error message with script name, line, column, and stack.
func formatScriptError(source string, err error) string {
	if err == nil {
		return ""
	}

	var gojaEx *goja.Exception
	if errors.As(err, &gojaEx) {
		val := gojaEx.Value()
		msg := gojaEx.Error()
		if val != nil {
			msg = val.String()
		}

		var stackLines []string
		for _, frame := range gojaEx.Stack() {
			src := frame.SrcName()
			line := frame.Position().Line
			col := frame.Position().Column
			if src == "" || src == "<native>" || strings.HasPrefix(src, "native") {
				continue
			}
			stackLines = append(stackLines, fmt.Sprintf("    at %s (%s:%d:%d)", frame.FuncName(), src, line, col))
		}

		if len(stackLines) > 0 {
			return fmt.Sprintf("[%s] %s\n%s", source, msg, strings.Join(stackLines, "\n"))
		}
		return fmt.Sprintf("[%s] %s", source, msg)
	}

	return fmt.Sprintf("[%s] %s", source, err.Error())
}
