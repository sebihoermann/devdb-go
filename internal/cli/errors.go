package cli

import (
	"fmt"
	"os"
	"strings"
)

// CLIError carries an exit code and optional suggestion for missed-call logging.
type CLIError struct {
	Code       int
	Message    string
	Suggestion string
	Kind       string
}

func (e *CLIError) Error() string {
	return e.Message
}

const (
	ExitOK            = 0
	ExitGeneral       = 1
	ExitUsage         = 2
	ExitNotFound      = 3
	ExitSchema        = 4
	ExitInvalidValue  = 5
)

func usageError(msg string) *CLIError {
	return &CLIError{Code: ExitUsage, Message: msg, Kind: "missing_argument"}
}

func notFoundError(msg string) *CLIError {
	return &CLIError{Code: ExitNotFound, Message: msg, Kind: "not_found"}
}

func unknownCommandError(argv []string) *CLIError {
	cmd := strings.Join(argv, " ")
	suggestion := suggestCommand(argv)
	return &CLIError{
		Code:       ExitUsage,
		Message:    fmt.Sprintf("unknown command: %s", cmd),
		Suggestion: suggestion,
		Kind:       "unknown_command",
	}
}

func suggestCommand(argv []string) string {
	if len(argv) == 0 {
		return "devdb help"
	}
	legacy := map[string]string{
		"init":                  "devdb init",
		"work-on":               "devdb plan item start",
		"pause-on":              "devdb plan item pause",
		"resume":                "devdb resume",
		"feedback-user":         "devdb feedback add --role user",
		"feedback-model":        "devdb feedback add --role model",
		"feedback-codebase":     "devdb feedback add --role codebase",
		"import-feedback-md":    "devdb feedback import markdown",
		"import-branch-commits": "devdb feedback import commits",
		"create-plan":           "devdb plan create",
		"scaffold-plan":         "devdb plan scaffold",
		"promote-plan":          "devdb plan promote",
		"reconcile-plans":       "devdb plan reconcile",
		"backfill-acceptance":   "devdb plan acceptance backfill",
		"show-plan-item":        "devdb plan item show",
		"list-missed-calls":     "devdb analytics missed",
		"missed-calls-summary":  "devdb analytics summary",
		"gc":                    "devdb archive gc",
		"restore":               "devdb archive restore",
		"restore-list":          "devdb archive list",
		"scan":                  "devdb inventory scan",
		"context":               "devdb inventory context",
		"diff-since":            "devdb inventory diff",
		"suggest-cuts":          "devdb inventory suggest-cuts",
		"add-arch-note":         "devdb arch add",
		"list-arch-notes":       "devdb arch list",
		"verify-arch-note":      "devdb arch verify",
		"arch-render":           "devdb arch render",
		"review-start":          "devdb review start",
		"review-add-finding":    "devdb review finding",
		"review-finish":         "devdb review finish",
		"review-list":           "devdb review list",
		"review-resolve":        "devdb review resolve",
		"review-report":         "devdb review report",
		"record-verification-run": "devdb verify record",
		"query-verification":    "devdb verify query",
		"show-verification-run": "devdb verify show",
		"dismiss-verification":  "devdb verify dismiss",
		"register":              "devdb hub register",
		"hub-sync":              "devdb hub sync",
		"hub-dashboard":         "devdb hub dashboard",
		"hub-project":           "devdb hub project",
		"doctor-sync":           "devdb hub doctor",
		"list-projects":         "devdb hub list",
		"across":                "devdb hub across",
	}
	if sug, ok := legacy[argv[0]]; ok {
		return sug
	}
	return "devdb help"
}

func printCLIError(err error, stderr *os.File) int {
	ce, ok := err.(*CLIError)
	if !ok {
		fmt.Fprintln(stderr, err.Error())
		return ExitGeneral
	}
	fmt.Fprintln(stderr, ce.Message)
	if ce.Suggestion != "" {
		fmt.Fprintf(stderr, "try: %s\n", ce.Suggestion)
	}
	return ce.Code
}
