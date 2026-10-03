package scripting

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dop251/goja/parser"
)

// SyntaxValidationError describes an issue found during static script validation.
type SyntaxValidationError struct {
	ScriptName string
	Line       int
	Column     int
	Message    string
	Snippet    string
}

func (e *SyntaxValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("[%s] Syntax error at line %d, column %d: %s", e.ScriptName, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("[%s] Syntax error: %s", e.ScriptName, e.Message)
}

var (
	// Patterns for unsupported ES6+ / module syntax in Goja
	asyncFuncPattern = regexp.MustCompile(`\basync\s+(function|\(|\w)`)
	awaitPattern     = regexp.MustCompile(`\bawait\s+[\w\(\{\[]`)
	requirePattern   = regexp.MustCompile(`\brequire\s*\(`)
	importPattern    = regexp.MustCompile(`(?m)^\s*import\s+`)
	exportPattern    = regexp.MustCompile(`(?m)^\s*export\s+`)
	generatorPattern = regexp.MustCompile(`\bfunction\s*\*|\byield\s+`)
)

// ValidateScriptSyntax performs static checks for unsupported syntax and syntax errors.
func ValidateScriptSyntax(scriptName, script string) error {
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(script, "\n")

	// 1. Check for 'async' / 'await'
	for lineIdx, line := range lines {
		// Skip single-line comments
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "//") || strings.HasPrefix(trimmedLine, "/*") {
			continue
		}

		if loc := asyncFuncPattern.FindStringIndex(line); loc != nil {
			return &SyntaxValidationError{
				ScriptName: scriptName,
				Line:       lineIdx + 1,
				Column:     loc[0] + 1,
				Message:    "'async' functions are not supported by the Goja JavaScript engine. Scripts execute synchronously; use pb.sendRequest() for auxiliary requests.",
				Snippet:    line,
			}
		}

		if loc := awaitPattern.FindStringIndex(line); loc != nil {
			return &SyntaxValidationError{
				ScriptName: scriptName,
				Line:       lineIdx + 1,
				Column:     loc[0] + 1,
				Message:    "'await' expressions are not supported by the Goja JavaScript engine. Scripts execute synchronously.",
				Snippet:    line,
			}
		}

		if loc := requirePattern.FindStringIndex(line); loc != nil {
			return &SyntaxValidationError{
				ScriptName: scriptName,
				Line:       lineIdx + 1,
				Column:     loc[0] + 1,
				Message:    "'require()' is not available in the sandbox. Use built-in pb.* utilities (e.g. pb.uuid, pb.crypto, pb.hash).",
				Snippet:    line,
			}
		}

		if loc := importPattern.FindStringIndex(line); loc != nil {
			return &SyntaxValidationError{
				ScriptName: scriptName,
				Line:       lineIdx + 1,
				Column:     loc[0] + 1,
				Message:    "ES module 'import' statements are not supported in the sandbox. Built-ins are provided on the global 'pb' object.",
				Snippet:    line,
			}
		}

		if loc := exportPattern.FindStringIndex(line); loc != nil {
			return &SyntaxValidationError{
				ScriptName: scriptName,
				Line:       lineIdx + 1,
				Column:     loc[0] + 1,
				Message:    "ES module 'export' statements are not supported in the sandbox.",
				Snippet:    line,
			}
		}

		if loc := generatorPattern.FindStringIndex(line); loc != nil {
			return &SyntaxValidationError{
				ScriptName: scriptName,
				Line:       lineIdx + 1,
				Column:     loc[0] + 1,
				Message:    "Generator functions (function*) and 'yield' are not supported in the Goja runtime.",
				Snippet:    line,
			}
		}
	}

	// 2. Parse using Goja's native parser
	if _, err := parser.ParseFile(nil, scriptName, script, 0); err != nil {
		return fmt.Errorf("[%s] %w", scriptName, err)
	}

	return nil
}
