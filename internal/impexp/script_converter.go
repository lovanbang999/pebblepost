package impexp

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
)

var (
	rePmEnvGet        = regexp.MustCompile(`pm\.environment\.get\s*\(`)
	rePmEnvSet        = regexp.MustCompile(`pm\.environment\.set\s*\(`)
	rePmVarsGet       = regexp.MustCompile(`pm\.variables\.get\s*\(`)
	rePmCollVarsGet   = regexp.MustCompile(`pm\.collectionVariables\.get\s*\(`)
	rePmCollVarsSet   = regexp.MustCompile(`pm\.collectionVariables\.set\s*\(`)
	rePmGlobalsGet    = regexp.MustCompile(`pm\.globals\.get\s*\(`)
	rePmGlobalsSet    = regexp.MustCompile(`pm\.globals\.set\s*\(`)
	rePmRespJson      = regexp.MustCompile(`pm\.response\.json\s*\(\)`)
	rePmRespText      = regexp.MustCompile(`pm\.response\.text\s*\(\)`)
	rePmRespCode      = regexp.MustCompile(`pm\.response\.code`)
	rePmRespStatus    = regexp.MustCompile(`pm\.response\.status`)
	rePmTest          = regexp.MustCompile(`pm\.test\s*\(`)
	rePmConsoleLog    = regexp.MustCompile(`console\.(log|info|warn|error)\s*\(`)
	reOldTests        = regexp.MustCompile(`^(\s*)tests\[\s*["']([^"']+)["']\s*\]\s*=\s*(.+);?$`)
	reUnconvertiblePm = regexp.MustCompile(`pm\.(sendRequest|visualizer|iterationData|cookies|info)\b`)
)

// ConvertPostmanScript transforms Postman JavaScript (`pm.*`) into PebblePost script (`pb.*`).
// Lines that cannot be fully converted are annotated with a comment.
func ConvertPostmanScript(script string) (string, []string) {
	if strings.TrimSpace(script) == "" {
		return "", nil
	}

	var convertedLines []string
	var warnings []string

	scanner := bufio.NewScanner(strings.NewReader(script))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			convertedLines = append(convertedLines, line)
			continue
		}

		// Check for unconvertible Postman APIs
		if matches := reUnconvertiblePm.FindStringSubmatch(line); len(matches) > 1 {
			warning := fmt.Sprintf("Line %d: Unsupported Postman API 'pm.%s' cannot be automatically executed", lineNum, matches[1])
			warnings = append(warnings, warning)
			convertedLines = append(convertedLines, fmt.Sprintf("// [PebblePost] Could not auto-convert: %s", strings.TrimSpace(line)))
			convertedLines = append(convertedLines, "// "+line)
			continue
		}

		// Old style Postman test: tests["Status is 200"] = responseCode.code === 200;
		if matches := reOldTests.FindStringSubmatch(line); len(matches) == 4 {
			indent := matches[1]
			testName := matches[2]
			assertion := strings.TrimRight(matches[3], ";")
			converted := fmt.Sprintf("%spb.test(%q, () => { pb.expect(%s).toBe(true); });", indent, testName, assertion)
			convertedLines = append(convertedLines, converted)
			continue
		}

		// Environment & Variables
		converted := rePmEnvGet.ReplaceAllString(line, "pb.environment.get(")
		converted = rePmEnvSet.ReplaceAllString(converted, "pb.environment.set(")
		converted = rePmVarsGet.ReplaceAllString(converted, "pb.variables.get(")
		converted = rePmCollVarsGet.ReplaceAllString(converted, "pb.variables.get(")
		converted = rePmCollVarsSet.ReplaceAllString(converted, "pb.environment.set(")
		converted = rePmGlobalsGet.ReplaceAllString(converted, "pb.environment.get(")
		converted = rePmGlobalsSet.ReplaceAllString(converted, "pb.environment.set(")

		// Response APIs
		converted = rePmRespJson.ReplaceAllString(converted, "pb.response.json()")
		converted = rePmRespText.ReplaceAllString(converted, "pb.response.text()")
		converted = rePmRespCode.ReplaceAllString(converted, "pb.response.status")
		converted = rePmRespStatus.ReplaceAllString(converted, "pb.response.status")

		// Tests & Logging
		converted = rePmTest.ReplaceAllString(converted, "pb.test(")
		converted = rePmConsoleLog.ReplaceAllString(converted, "pb.console.$1(")

		// Common Postman expect conversions:
		// pm.expect(x).to.eql(y) -> pb.expect(x).toEqual(y)
		// pm.expect(x).to.equal(y) -> pb.expect(x).toBe(y)
		converted = strings.ReplaceAll(converted, "pm.expect(", "pb.expect(")
		converted = strings.ReplaceAll(converted, ").to.eql(", ").toEqual(")
		converted = strings.ReplaceAll(converted, ").to.equal(", ").toBe(")
		converted = strings.ReplaceAll(converted, ").to.be.true", ").toBe(true)")
		converted = strings.ReplaceAll(converted, ").to.be.false", ").toBe(false)")

		// Catch any remaining unconverted pm.* calls
		if strings.Contains(converted, "pm.") && !strings.HasPrefix(strings.TrimSpace(converted), "//") {
			warnings = append(warnings, fmt.Sprintf("Line %d: Unrecognized 'pm.*' call may require manual adjustment: %s", lineNum, trimmed))
			converted = "// [PebblePost] Note: Partial conversion — verify pm.* call:\n" + converted
		}

		convertedLines = append(convertedLines, converted)
	}

	return strings.Join(convertedLines, "\n"), warnings
}
