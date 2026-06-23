package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/domain/verification"
	"github.com/sebihoermann/devdb-go/internal/git"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/spf13/cobra"
)

func cmdReview(open opener) *cobra.Command {
	r := &cobra.Command{Use: "review", Short: "Code review ledger"}
	r.AddCommand(
		cmdReviewStart(open),
		cmdReviewFinding(open),
		cmdReviewList(open),
		cmdReviewResolve(open),
		cmdReviewFinish(open),
		cmdReviewReport(open),
		cmdReviewImport(open),
		cmdReviewPrinciples(open),
	)
	return r
}

func cmdReviewStart(open opener) *cobra.Command {
	var paths []string
	var tier, gitSHA string
	c := &cobra.Command{
		Use:   "start",
		Short: "Open a review run",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if gitSHA == "" {
				gitSHA = git.HeadSHA(ctx.Project.RepoRoot)
			}
			if len(paths) == 0 {
				paths = []string{"."}
			}
			id, err := review.StartRun(ctx.DB, paths, tier, gitSHA, ctx.ModelID)
			if err != nil {
				return err
			}
			ctx.Out.Hint("review run %s · tier %s · scope %s", id[:8], tier, strings.Join(paths, ","))
			return ctx.Out.WriteResult(id, map[string]any{"kind": "review_run", "tier": tier})
		},
	}
	c.Flags().StringSliceVar(&paths, "paths", nil, "scope paths (default: entire repo)")
	c.Flags().StringVar(&tier, "tier", "default", "default|extended|grass-cutter")
	c.Flags().StringVar(&gitSHA, "git-sha", "", "git revision (default: HEAD)")
	return c
}

func cmdReviewFinding(open opener) *cobra.Command {
	var (
		runID, filePath, principle, title, recommendation string
		severity, confidence, effort                      string
		lineStart, lineEnd                              int
		hasLineStart, hasLineEnd                          bool
	)
	c := &cobra.Command{
		Use:   "finding",
		Short: "Add a finding to an open review run",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if runID == "" || principle == "" || title == "" || recommendation == "" {
				return usageError("required: --run, --principle, --title, --recommendation")
			}
			if severity == "" {
				severity = "med"
			}
			if confidence == "" {
				confidence = "medium"
			}
			if effort == "" {
				effort = "small"
			}
			in := review.FindingInput{
				FilePath: filePath, Principle: principle, Title: title,
				Recommendation: recommendation, Severity: severity,
				Confidence: confidence, Effort: effort,
			}
			if hasLineStart {
				in.LineStart = &lineStart
			}
			if hasLineEnd {
				in.LineEnd = &lineEnd
			}
			id, err := review.AddFinding(ctx.DB, runID, in, ctx.ModelID)
			if errors.Is(err, review.ErrRunNotFound) {
				return notFoundError("run not found")
			}
			if errors.Is(err, review.ErrRunFinished) {
				return &CLIError{Code: 5, Message: "run already finished", Kind: "invalid_state"}
			}
			if errors.Is(err, review.ErrCapExceeded) {
				return &CLIError{Code: 10, Message: "finding cap exceeded for file", Kind: "cap_exceeded"}
			}
			if err != nil {
				return &CLIError{Code: ExitInvalidValue, Message: err.Error(), Kind: "invalid_value"}
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "review_finding"})
		},
	}
	c.Flags().StringVar(&runID, "run", "", "review run id")
	c.Flags().StringVar(&filePath, "file", "", "file path")
	c.Flags().IntVar(&lineStart, "line-start", 0, "starting line")
	c.Flags().IntVar(&lineEnd, "line-end", 0, "ending line")
	c.Flags().StringVar(&principle, "principle", "", "review principle")
	c.Flags().StringVar(&title, "title", "", "finding title")
	c.Flags().StringVar(&recommendation, "recommendation", "", "recommended fix")
	c.Flags().StringVar(&severity, "severity", "", "info|low|med|high|critical")
	c.Flags().StringVar(&confidence, "confidence", "", "low|medium|high")
	c.Flags().StringVar(&effort, "effort", "", "trivial|small|med|large")
	c.PreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("line-start") {
			hasLineStart = true
		}
		if cmd.Flags().Changed("line-end") {
			hasLineEnd = true
		}
		return nil
	}
	return c
}

func cmdReviewList(open opener) *cobra.Command {
	var status, runID, principle, filePath, severity string
	var limit int
	c := &cobra.Command{
		Use:   "list",
		Short: "List review findings",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			st := status
			if st == "all" {
				st = ""
			}
			findings, err := review.ListFindings(ctx.DB, review.ListFilter{
				Status: st, RunID: runID, Principle: principle,
				FilePath: filePath, Severity: severity, Limit: applyAllLimit(limit),
			})
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(findings)
			}
			if len(findings) == 0 {
				return ctx.Out.PrintData(output.HumanLines{Lines: []string{"no findings"}})
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: review.CompactLines(findings)})
		},
	}
	c.Flags().StringVar(&status, "status", "open", "filter status (open|resolved|wontfix|all)")
	c.Flags().StringVar(&runID, "run", "", "filter by run id")
	c.Flags().StringVar(&principle, "principle", "", "filter by principle")
	c.Flags().StringVar(&filePath, "file", "", "filter by file path")
	c.Flags().StringVar(&severity, "severity", "", "filter by severity")
	c.Flags().IntVar(&limit, "limit", 20, "max rows")
	return c
}

func cmdReviewResolve(open opener) *cobra.Command {
	var status, commit, evidence string
	c := &cobra.Command{
		Use:   "resolve ID",
		Short: "Resolve a review finding",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if status == "" {
				status = "resolved"
			}
			ok, err := review.ResolveFinding(ctx.DB, args[0], commit, status, evidence)
			if err != nil && strings.Contains(err.Error(), "commit or evidence") {
				return &CLIError{Code: 8, Message: err.Error(), Kind: "missing_argument"}
			}
			if err != nil {
				return err
			}
			if !ok {
				return notFoundError("finding not found")
			}
			ctx.Out.Hint("finding resolved")
			return ctx.Out.WriteResult(args[0], map[string]any{"kind": "review_finding", "status": status})
		},
	}
	c.Flags().StringVar(&status, "status", "resolved", "resolved|wontfix|accepted|duplicate|open")
	c.Flags().StringVar(&commit, "commit", "", "commit SHA where fixed")
	c.Flags().StringVar(&evidence, "evidence", "", "evidence note when no commit yet")
	return c
}

func cmdReviewFinish(open opener) *cobra.Command {
	var summary string
	c := &cobra.Command{
		Use:   "finish RUN_ID",
		Short: "Finish a review run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			ok, err := review.FinishRun(ctx.DB, args[0], summary)
			if errors.Is(err, review.ErrRunFinished) {
				return &CLIError{Code: 7, Message: "run already finished", Kind: "invalid_state"}
			}
			if err != nil {
				return err
			}
			if !ok {
				return notFoundError("run not found")
			}
			ctx.Out.Hint("review run finished")
			return ctx.Out.WriteResult(args[0], map[string]any{"kind": "review_run", "action": "finish"})
		},
	}
	c.Flags().StringVar(&summary, "summary", "", "run summary")
	return c
}

func cmdReviewReport(open opener) *cobra.Command {
	var outPath string
	c := &cobra.Command{
		Use:   "report RUN_ID",
		Short: "Render a review run report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			text, err := review.RenderReport(ctx.DB, args[0])
			if errors.Is(err, review.ErrRunNotFound) {
				return notFoundError("run not found")
			}
			if err != nil {
				return err
			}
			if outPath == "" {
				outPath = filepath.Join(ctx.Project.RepoRoot, ".devdb", "reviews", args[0]+".md")
			}
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(outPath, []byte(text), 0o644); err != nil {
				return err
			}
			return ctx.Out.WriteResult(outPath, map[string]any{"kind": "review_report", "run_id": args[0]})
		},
	}
	c.Flags().StringVar(&outPath, "output", "", "output path (default: .devdb/reviews/<run_id>.md)")
	return c
}

func cmdReviewImport(open opener) *cobra.Command {
	var runID, filePath string
	var forceCap bool
	c := &cobra.Command{
		Use:   "import",
		Short: "Batch-import findings from JSONL",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if runID == "" || filePath == "" {
				return usageError("required: --run and --file")
			}
			f, err := os.Open(filePath)
			if err != nil {
				return notFoundError(fmt.Sprintf("file not found: %s", filePath))
			}
			defer f.Close()
			var items []review.FindingInput
			scanner := bufio.NewScanner(f)
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				var raw map[string]any
				if err := json.Unmarshal([]byte(line), &raw); err != nil {
					return &CLIError{Code: 2, Message: fmt.Sprintf("invalid JSON on line %d: %v", lineNo, err), Kind: "invalid_value"}
				}
				items = append(items, parseFindingImport(raw))
			}
			if err := scanner.Err(); err != nil {
				return err
			}
			result, err := review.ImportFindings(ctx.DB, runID, items, forceCap, ctx.ModelID)
			if errors.Is(err, review.ErrRunNotFound) {
				return notFoundError("run not found")
			}
			if errors.Is(err, review.ErrRunFinished) {
				return &CLIError{Code: 5, Message: "run already finished", Kind: "invalid_state"}
			}
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(result)
			}
			ctx.Out.Hint("imported %d finding(s)", len(result.Imported))
			if len(result.SkippedCap) > 0 {
				ctx.Out.Hint("skipped (cap): %d", len(result.SkippedCap))
			}
			if len(result.Errors) > 0 {
				ctx.Out.Hint("errors: %d", len(result.Errors))
			}
			if len(result.Imported) == 1 {
				return ctx.Out.WriteResult(result.Imported[0], map[string]any{"kind": "review_import", "count": 1})
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: []string{
				fmt.Sprintf("imported %d finding(s)", len(result.Imported)),
			}})
		},
	}
	c.Flags().StringVar(&runID, "run", "", "review run id")
	c.Flags().StringVar(&filePath, "file", "", "JSONL file path")
	c.Flags().BoolVar(&forceCap, "force-cap", false, "bypass per-file finding cap")
	return c
}

func cmdReviewPrinciples(open opener) *cobra.Command {
	var tier string
	c := &cobra.Command{
		Use:   "principles",
		Short: "List valid review principles for a tier",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if tier == "" {
				tier = "default"
			}
			principles := review.PrinciplesForTier(tier)
			if ctx.Out.JSON {
				return ctx.Out.PrintData(map[string]any{"tier": tier, "principles": principles})
			}
			lines := []string{fmt.Sprintf("principles for tier %s:", tier)}
			for _, p := range principles {
				lines = append(lines, "  - "+p)
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringVar(&tier, "tier", "default", "review tier")
	return c
}

func cmdVerify(open opener) *cobra.Command {
	v := &cobra.Command{Use: "verify", Short: "Verification reuse ledger"}
	v.AddCommand(cmdVerifyRecord(open), cmdVerifyQuery(open), cmdVerifyShow(open), cmdVerifyDismiss(open))
	return v
}

func cmdVerifyRecord(open opener) *cobra.Command {
	var (
		scope, gitSHA, status, notes, output string
		exitCode                             int
		hasExitCode                          bool
		finished                             bool
		inputs                               []string
	)
	c := &cobra.Command{
		Use:   "record COMMAND",
		Short: "Record a verification run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if scope == "" {
				return usageError("--scope is required")
			}
			if gitSHA == "" {
				gitSHA = git.HeadSHA(ctx.Project.RepoRoot)
			}
			if status == "" {
				status = "pending"
			}
			var exitPtr *int
			if hasExitCode {
				exitPtr = &exitCode
			}
			runID, err := verification.RecordRun(ctx.DB, args[0], scope, gitSHA, status, exitPtr, output, notes, ctx.ModelID)
			if err != nil {
				return err
			}
			triples, autoInputs, err := resolveVerificationInputs(ctx, scope, inputs)
			if err != nil {
				return err
			}
			if len(triples) > 0 {
				if err := verification.AddInputs(ctx.DB, runID, triples, ctx.ModelID); err != nil {
					return err
				}
			} else if autoInputs {
				ctx.Out.Hint("no repo_files inputs collected for scope; run devdb inventory scan first")
			}
			if finished {
				if err := verification.FinishRun(ctx.DB, runID, status, exitPtr, output); err != nil {
					return err
				}
			}
			ctx.Out.Hint("recorded verification run %s", runID[:8])
			return ctx.Out.WriteResult(runID, map[string]any{
				"kind": "verification_run", "inputs": len(triples), "auto_inputs": autoInputs,
			})
		},
	}
	c.Flags().StringVar(&scope, "scope", "", "verification scope path(s)")
	c.Flags().StringVar(&gitSHA, "git-sha", "", "git revision")
	c.Flags().StringVar(&status, "status", "pending", "pending|passed|failed")
	c.Flags().IntVar(&exitCode, "exit-code", 0, "process exit code")
	c.Flags().StringVar(&output, "output", "", "captured verifier output")
	c.Flags().StringVar(&notes, "notes", "", "short human summary")
	c.Flags().BoolVar(&finished, "finished", false, "mark run finished immediately")
	c.Flags().StringSliceVar(&inputs, "inputs", nil, "explicit path:role:hash triples")
	c.PreRunE = func(cmd *cobra.Command, args []string) error {
		hasExitCode = cmd.Flags().Changed("exit-code")
		return nil
	}
	return c
}

func cmdVerifyQuery(open opener) *cobra.Command {
	var command, scope string
	var inputs []string
	c := &cobra.Command{
		Use:   "query",
		Short: "Check whether a prior passing run is still fresh",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if command == "" || scope == "" {
				return usageError("required: --command and --scope")
			}
			triples, autoInputs, err := resolveVerificationInputs(ctx, scope, inputs)
			if err != nil {
				return err
			}
			result := verification.Query(ctx.DB, command, scope, triples, autoInputs)
			if ctx.Out.JSON {
				return ctx.Out.PrintData(result)
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: []string{verification.CompactQueryLine(result)}})
		},
	}
	c.Flags().StringVar(&command, "command", "", "verification command string")
	c.Flags().StringVar(&scope, "scope", "", "verification scope")
	c.Flags().StringSliceVar(&inputs, "inputs", nil, "explicit path:role:hash triples")
	return c
}

func cmdVerifyShow(open opener) *cobra.Command {
	var view string
	c := &cobra.Command{
		Use:   "show RUN_ID",
		Short: "Show a verification run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			switch view {
			case "inputs":
				inputs, err := verification.GetInputs(ctx.DB, args[0])
				if err != nil {
					return err
				}
				if ctx.Out.JSON {
					return ctx.Out.PrintData(map[string]any{"inputs": inputs})
				}
				if len(inputs) == 0 {
					return ctx.Out.PrintData(output.HumanLines{Lines: []string{"inputs: none"}})
				}
				lines := make([]string, 0, len(inputs))
				for _, in := range inputs {
					lines = append(lines, fmt.Sprintf("%s [%s] %s", in.FilePath, in.Role, in.ContentHash))
				}
				return ctx.Out.PrintData(output.HumanLines{Lines: lines})
			case "failures":
				failures, err := verification.GetFailures(ctx.DB, args[0], 0)
				if err != nil {
					return err
				}
				if ctx.Out.JSON {
					return ctx.Out.PrintData(map[string]any{"failures": failures})
				}
				if len(failures) == 0 {
					return ctx.Out.PrintData(output.HumanLines{Lines: []string{"failures: none"}})
				}
				lines := make([]string, 0, len(failures))
				for _, f := range failures {
					lines = append(lines, "- "+f.Headline)
				}
				return ctx.Out.PrintData(output.HumanLines{Lines: lines})
			default:
				summary, err := verification.Show(ctx.DB, args[0])
				if err != nil {
					return err
				}
				if summary == nil {
					return notFoundError("verification run not found")
				}
				if ctx.Out.JSON {
					return ctx.Out.PrintData(summary)
				}
				lines := []string{
					fmt.Sprintf("verification run %s...", summary.Run.ID[:8]),
					fmt.Sprintf("- command: %s", summary.Run.Command),
					fmt.Sprintf("- scope: %s", summary.Run.Scope),
					fmt.Sprintf("- status: %s", summary.Run.Status),
					fmt.Sprintf("- git_sha: %s", summary.Run.GitSHA),
					fmt.Sprintf("- inputs: %d", summary.InputCount),
				}
				if summary.Run.FinishedAt != "" {
					lines = append(lines, fmt.Sprintf("- finished_at: %s", summary.Run.FinishedAt))
				}
				if summary.Run.Notes != "" {
					lines = append(lines, fmt.Sprintf("- notes: %s", summary.Run.Notes))
				}
				switch summary.Reuse.Decision {
				case "reusable":
					lines = append(lines, fmt.Sprintf("- reuse: fresh_pass (%s)", summary.Reuse.Reason))
				case "rerun_required":
					lines = append(lines, fmt.Sprintf("- reuse: rerun_required (%s)", summary.Reuse.Reason))
				case "failed_last_time":
					lines = append(lines, fmt.Sprintf("- reuse: failed_last_time (%s)", summary.Reuse.Reason))
				default:
					lines = append(lines, fmt.Sprintf("- reuse: %s (%s)", summary.Reuse.Decision, summary.Reuse.Reason))
				}
				return ctx.Out.PrintData(output.HumanLines{Lines: lines})
			}
		},
	}
	c.Flags().StringVar(&view, "view", "summary", "summary|inputs|failures|full")
	return c
}

func cmdVerifyDismiss(open opener) *cobra.Command {
	var reason string
	c := &cobra.Command{
		Use:   "dismiss RUN_ID",
		Short: "Dismiss a verification run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			ok, err := verification.Dismiss(ctx.DB, args[0], reason)
			if err != nil {
				return err
			}
			if !ok {
				return notFoundError("verification run not found or already dismissed")
			}
			return ctx.Out.WriteResult(args[0], map[string]any{"kind": "verification_run", "action": "dismiss"})
		},
	}
	c.Flags().StringVar(&reason, "reason", "", "dismissal reason")
	return c
}

func resolveVerificationInputs(ctx *app.Context, scope string, specs []string) ([][3]string, bool, error) {
	if len(specs) > 0 {
		var triples [][3]string
		for _, spec := range specs {
			parts := strings.Split(spec, ":")
			if len(parts) != 3 {
				return nil, false, usageError(fmt.Sprintf("invalid input format: %s (expected path:role:hash)", spec))
			}
			triples = append(triples, [3]string{parts[0], parts[1], parts[2]})
		}
		return triples, false, nil
	}
	triples, err := verification.CollectInputsForScope(ctx.DB, scope)
	return triples, true, err
}

func parseFindingImport(raw map[string]any) review.FindingInput {
	in := review.FindingInput{
		Principle:      strField(raw, "principle"),
		Title:          strField(raw, "title"),
		Recommendation: strField(raw, "recommendation"),
		Severity:       strField(raw, "severity"),
		Confidence:     strField(raw, "confidence"),
		Effort:         strField(raw, "effort"),
	}
	if v, ok := raw["file"].(string); ok {
		in.FilePath = v
	}
	if v, ok := raw["line_start"]; ok {
		if n, ok := toInt(v); ok {
			in.LineStart = &n
		}
	}
	if v, ok := raw["line_end"]; ok {
		if n, ok := toInt(v); ok {
			in.LineEnd = &n
		}
	}
	return in
}

func strField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	default:
		return 0, false
	}
}
