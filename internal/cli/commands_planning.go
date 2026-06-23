package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/spf13/cobra"
)

func cmdPlanScaffold(open opener) *cobra.Command {
	var slug, body, mode, outputPath string
	var milestoneCount int
	var skipAcceptance bool
	c := &cobra.Command{
		Use:   "scaffold TITLE",
		Short: "Create plan skeleton and HTML artifact",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			result, err := planning.ScaffoldPlan(ctx.DB, planning.ScaffoldPlanInput{
				Title: strings.Join(args, " "), Slug: slug, Body: body, Mode: mode,
				MilestoneCount: milestoneCount, OutputPath: outputPath,
				RepoRoot: ctx.Project.RepoRoot, SkipAcceptance: skipAcceptance,
				ModelID: ctx.ModelID,
			})
			if err != nil {
				if errors.Is(err, planning.ErrSlugExists) {
					return &CLIError{Code: ExitGeneral, Message: err.Error(), Kind: "invalid_argument"}
				}
				if errors.Is(err, planning.ErrInvalidMode) {
					return &CLIError{Code: ExitUsage, Message: err.Error(), Kind: "invalid_argument"}
				}
				return err
			}
			ctx.Out.Hint("artifact: %s", result.Artifact)
			ctx.Out.Hint("next: devdb plan item start <id> · artifact at %s", result.Artifact)
			meta := map[string]any{
				"kind": "plan", "action": "scaffold",
				"slug": result.Slug, "artifact": result.Artifact, "milestones": result.Milestones,
			}
			return ctx.Out.WriteResult(result.PlanID, meta)
		},
	}
	c.Flags().StringVar(&slug, "slug", "", "plan slug (auto from title)")
	c.Flags().StringVar(&body, "body", "", "plan description")
	c.Flags().StringVar(&mode, "mode", "implement", "design|implement")
	c.Flags().IntVar(&milestoneCount, "milestones", 4, "number of milestones")
	c.Flags().StringVar(&outputPath, "output", "", "HTML artifact path (default: docs/<slug>-implementation-plan.html)")
	c.Flags().BoolVar(&skipAcceptance, "skip-acceptance", false, "omit default acceptance criteria")
	return c
}

func cmdPlanPromote(open opener) *cobra.Command {
	var planRef string
	c := &cobra.Command{
		Use:   "promote",
		Short: "Promote design-scaffolded items to implement mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if planRef == "" {
				return usageError("--plan is required")
			}
			result, err := planning.PromotePlan(ctx.DB, planRef, ctx.ModelID)
			if err != nil {
				if errors.Is(err, planning.ErrPlanNotFound) {
					return notFoundError("plan not found")
				}
				return err
			}
			ctx.Out.Hint("promoted %d title(s) and %d acceptance row(s) to implement mode",
				result.TitlesUpdated, result.AcceptanceUpdated)
			return ctx.Out.WriteResult(result.PlanID, map[string]any{
				"kind": "plan", "action": "promote",
				"plan": planRef, "titles_updated": result.TitlesUpdated,
				"acceptance_updated": result.AcceptanceUpdated,
			})
		},
	}
	c.Flags().StringVar(&planRef, "plan", "", "plan slug or id")
	return c
}

func cmdPlanReconcile(open opener) *cobra.Command {
	var planRef string
	var apply bool
	c := &cobra.Command{
		Use:   "reconcile",
		Short: "Detect or repair plan-tree status drift",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			result, err := planning.ReconcilePlans(ctx.DB, planRef, apply)
			if err != nil {
				if errors.Is(err, planning.ErrPlanNotFound) {
					return notFoundError("plan not found")
				}
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(result)
			}
			if result.Drift.IsEmpty() {
				return ctx.Out.PrintData(output.HumanLines{Lines: []string{"no plan-tree drift detected"}})
			}
			if !apply {
				lines := []string{
					fmt.Sprintf("drift detected: %d milestone(s), %d plan(s)",
						len(result.Drift.Milestones), len(result.Drift.Plans)),
				}
				for _, item := range result.Drift.Milestones {
					lines = append(lines, fmt.Sprintf("- milestone M%d [%s] %s -> %s",
						item.Number, item.PlanSlug, item.CurrentStatus, item.ExpectedStatus))
				}
				for _, item := range result.Drift.Plans {
					lines = append(lines, fmt.Sprintf("- plan %s %s -> %s",
						item.Slug, item.CurrentStatus, item.ExpectedStatus))
				}
				lines = append(lines, "run with --apply to repair")
				return ctx.Out.PrintData(output.HumanLines{Lines: lines})
			}
			applied := result.Applied
			if applied == nil {
				applied = &planning.AppliedReconcile{}
			}
			lines := []string{
				fmt.Sprintf("reconciled %d milestone(s), %d plan(s)",
					len(applied.Milestones), len(applied.Plans)),
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringVar(&planRef, "plan", "", "limit to one plan slug or id")
	c.Flags().BoolVar(&apply, "apply", false, "repair drifted statuses")
	return c
}

func cmdPlanAcceptanceBackfill(open opener) *cobra.Command {
	var milestone, specPath string
	c := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill acceptance criteria from a Markdown spec",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if milestone == "" {
				return usageError("--milestone is required (e.g. M1)")
			}
			if specPath == "" {
				return usageError("--spec is required")
			}
			count, err := planning.BackfillAcceptanceFromSpec(ctx.DB, milestone, specPath, ctx.ModelID)
			if err != nil {
				if errors.Is(err, planning.ErrSpecFileNotFound) {
					return notFoundError(err.Error())
				}
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(map[string]int{"created": count})
			}
			return ctx.Out.PrintData(output.HumanLines{
				Lines: []string{fmt.Sprintf("created %d acceptance criteria", count)},
			})
		},
	}
	c.Flags().StringVar(&milestone, "milestone", "", "milestone heading (e.g. M1)")
	c.Flags().StringVar(&specPath, "spec", "", "path to Markdown spec file")
	return c
}
