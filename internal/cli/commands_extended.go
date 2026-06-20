package cli

import (
	"fmt"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/approval"
	"github.com/sebihoermann/devdb-go/internal/domain/catalog"
	"github.com/sebihoermann/devdb-go/internal/domain/features"
	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/goals"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/reminders"
	"github.com/sebihoermann/devdb-go/internal/domain/tasks"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/spf13/cobra"
)

func cmdGoal(open opener) *cobra.Command {
	goal := &cobra.Command{Use: "goal", Short: "Project goals"}
	goal.AddCommand(cmdGoalAdd(open), cmdGoalList(open), cmdGoalSet(open))
	return goal
}

func cmdGoalAdd(open opener) *cobra.Command {
	var kind, body string
	c := &cobra.Command{
		Use:   "add TITLE",
		Short: "Add a goal",
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
			if kind == "" {
				kind = "goal"
			}
			id, err := goals.Add(ctx.DB, kind, strings.Join(args, " "), body, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "goal"})
		},
	}
	c.Flags().StringVar(&kind, "kind", "goal", "goal|do|dont")
	c.Flags().StringVar(&body, "body", "", "goal body")
	return c
}

func cmdGoalList(open opener) *cobra.Command {
	var st string
	c := &cobra.Command{
		Use:   "list",
		Short: "List goals",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rows, err := goals.List(ctx.DB, st, effectiveListLimit(20))
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(rows)
		},
	}
	c.Flags().StringVar(&st, "status", "active", "filter status (active|done|all)")
	return c
}

func cmdGoalSet(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "set ID STATUS",
		Short: "Set goal status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			id, err := goals.SetStatus(ctx.DB, args[0], args[1], ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "goal", "action": "set"})
		},
	}
}

func cmdFeature(open opener) *cobra.Command {
	feature := &cobra.Command{Use: "feature", Short: "Shipped features"}
	feature.AddCommand(cmdFeatureAdd(open), cmdFeatureList(open))
	return feature
}

func cmdFeatureAdd(open opener) *cobra.Command {
	var desc, sha, branch string
	c := &cobra.Command{
		Use:   "add TITLE",
		Short: "Record a feature",
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
			id, err := features.Add(ctx.DB, strings.Join(args, " "), desc, sha, branch, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "feature"})
		},
	}
	c.Flags().StringVar(&desc, "description", "", "feature description")
	c.Flags().StringVar(&sha, "commit", "", "commit sha")
	c.Flags().StringVar(&branch, "branch", "", "branch name")
	return c
}

func cmdFeatureList(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List features",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rows, err := features.List(ctx.DB, effectiveListLimit(20))
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(rows)
		},
	}
}

func cmdFeedbackAnnotate(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "annotate ID NOTE",
		Short: "Append annotation to feedback context",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			id, err := feedback.Annotate(ctx.DB, args[0], strings.Join(args[1:], " "), ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "feedback", "action": "annotate"})
		},
	}
}

func cmdTask(open opener) *cobra.Command {
	task := &cobra.Command{Use: "task", Short: "Lightweight tasks"}
	task.AddCommand(cmdTaskAdd(open), cmdTaskList(open), cmdTaskShow(open), cmdTaskDone(open), cmdTaskStatus(open))
	return task
}

func cmdTaskAdd(open opener) *cobra.Command {
	var body, priority, due string
	c := &cobra.Command{
		Use:   "add TITLE",
		Short: "Add a task",
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
			id, err := tasks.Add(ctx.DB, strings.Join(args, " "), body, priority, due, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "task"})
		},
	}
	c.Flags().StringVar(&body, "body", "", "task body")
	c.Flags().StringVar(&priority, "priority", "med", "low|med|high")
	c.Flags().StringVar(&due, "due", "", "due date ISO")
	return c
}

func cmdTaskList(open opener) *cobra.Command {
	var st, priority string
	c := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rows, err := tasks.List(ctx.DB, st, priority, effectiveListLimit(30))
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(rows)
		},
	}
	c.Flags().StringVar(&st, "status", "open", "open|done|all")
	c.Flags().StringVar(&priority, "priority", "", "filter priority")
	return c
}

func cmdTaskShow(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show a task",
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
			row, err := tasks.Show(ctx.DB, args[0])
			if err != nil {
				return notFoundError(err.Error())
			}
			return ctx.Out.PrintData(row)
		},
	}
}

func cmdTaskDone(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "done ID",
		Short: "Mark task done",
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
			id, err := tasks.SetStatus(ctx.DB, args[0], "done", ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "task", "action": "done"})
		},
	}
}

func cmdTaskStatus(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "status ID STATUS",
		Short: "Set task status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			id, err := tasks.SetStatus(ctx.DB, args[0], args[1], ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "task", "action": "status"})
		},
	}
}

func cmdApproval(open opener) *cobra.Command {
	ap := &cobra.Command{Use: "approval", Short: "Approval workflow"}
	ap.AddCommand(cmdApprovalRequest(open), cmdApprovalApprove(open), cmdApprovalReject(open),
		cmdApprovalWithdraw(open), cmdApprovalList(open), cmdApprovalLog(open))
	return ap
}

func cmdApprovalRequest(open opener) *cobra.Command {
	var table, note string
	c := &cobra.Command{
		Use:   "request ENTITY_ID",
		Short: "Request approval",
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
			if table == "" {
				table = "tasks"
			}
			id, err := approval.Request(ctx.DB, table, args[0], note, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "approval", "action": "request"})
		},
	}
	c.Flags().StringVar(&table, "entity", "tasks", "tasks|plan_items")
	c.Flags().StringVar(&note, "note", "", "optional note")
	return c
}

func cmdApprovalApprove(open opener) *cobra.Command {
	var table, note string
	c := &cobra.Command{
		Use:   "approve ENTITY_ID",
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
			if table == "" {
				table = "tasks"
			}
			id, err := approval.Approve(ctx.DB, table, args[0], note, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "approval", "action": "approve"})
		},
	}
	c.Flags().StringVar(&table, "entity", "tasks", "tasks|plan_items")
	c.Flags().StringVar(&note, "note", "", "optional note")
	return c
}

func cmdApprovalReject(open opener) *cobra.Command {
	var table, note string
	c := &cobra.Command{
		Use:   "reject ENTITY_ID",
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
			if table == "" {
				table = "tasks"
			}
			id, err := approval.Reject(ctx.DB, table, args[0], note, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "approval", "action": "reject"})
		},
	}
	c.Flags().StringVar(&table, "entity", "tasks", "tasks|plan_items")
	c.Flags().StringVar(&note, "note", "", "optional note")
	return c
}

func cmdApprovalWithdraw(open opener) *cobra.Command {
	var table, note string
	c := &cobra.Command{
		Use:   "withdraw ENTITY_ID",
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
			if table == "" {
				table = "tasks"
			}
			id, err := approval.Withdraw(ctx.DB, table, args[0], note, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "approval", "action": "withdraw"})
		},
	}
	c.Flags().StringVar(&table, "entity", "tasks", "tasks|plan_items")
	c.Flags().StringVar(&note, "note", "", "optional note")
	return c
}

func cmdApprovalList(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List pending approvals",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rows, err := approval.ListPending(ctx.DB)
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(rows)
		},
	}
}

func cmdApprovalLog(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "log",
		Short: "Approval audit log",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rows, err := approval.Log(ctx.DB, effectiveListLimit(30))
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(rows)
		},
	}
}

func cmdReminder(open opener) *cobra.Command {
	r := &cobra.Command{Use: "reminder", Short: "Reminders"}
	r.AddCommand(cmdReminderAdd(open), cmdReminderList(open), cmdReminderShow(open),
		cmdReminderDismiss(open), cmdReminderSnooze(open), cmdReminderUnsnooze(open))
	return r
}

func cmdReminderAdd(open opener) *cobra.Command {
	var body, due, file, planItem string
	c := &cobra.Command{
		Use:   "add TITLE",
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
			id, err := reminders.Add(ctx.DB, reminders.AddInput{
				Title: strings.Join(args, " "), Body: body, DueAt: due,
				FilePath: file, PlanItemID: planItem, ModelID: ctx.ModelID,
			})
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "reminder"})
		},
	}
	c.Flags().StringVar(&body, "body", "", "reminder body")
	c.Flags().StringVar(&due, "due", "", "due at ISO timestamp")
	c.Flags().StringVar(&file, "file", "", "tagged file path")
	c.Flags().StringVar(&planItem, "plan-item", "", "linked plan item id")
	return c
}

func cmdReminderList(open opener) *cobra.Command {
	var st string
	var overdue bool
	c := &cobra.Command{
		Use:   "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rows, err := reminders.List(ctx.DB, st, overdue, effectiveListLimit(30))
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(rows)
		},
	}
	c.Flags().StringVar(&st, "status", "open", "open|dismissed|all")
	c.Flags().BoolVar(&overdue, "overdue", false, "only overdue")
	return c
}

func cmdReminderShow(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
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
			row, err := reminders.Show(ctx.DB, args[0])
			if err != nil {
				return notFoundError(err.Error())
			}
			return ctx.Out.PrintData(row)
		},
	}
}

func cmdReminderDismiss(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "dismiss ID",
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
			id, err := reminders.Dismiss(ctx.DB, args[0])
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "reminder", "action": "dismiss"})
		},
	}
}

func cmdReminderSnooze(open opener) *cobra.Command {
	var until string
	c := &cobra.Command{
		Use:   "snooze ID",
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
			id, err := reminders.Snooze(ctx.DB, args[0], until)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "reminder", "action": "snooze"})
		},
	}
	c.Flags().StringVar(&until, "until", "", "snooze until ISO timestamp")
	return c
}

func cmdReminderUnsnooze(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "unsnooze ID",
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
			id, err := reminders.Unsnooze(ctx.DB, args[0])
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "reminder", "action": "unsnooze"})
		},
	}
}

func cmdList(open opener) *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "list TABLE",
		Short: "List rows from a ledger table",
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
			rows, err := catalog.ListRows(ctx.DB, args[0], applyAllLimit(limit))
			if err != nil {
				return &CLIError{Code: ExitInvalidValue, Message: err.Error(), Kind: "invalid_argument"}
			}
			return ctx.Out.PrintData(rows)
		},
	}
	c.Flags().IntVar(&limit, "limit", 20, "max rows")
	return c
}

func cmdShow(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "show TABLE ID",
		Short: "Show one row by id prefix",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			row, err := catalog.ShowRow(ctx.DB, args[0], args[1])
			if err != nil {
				return notFoundError(err.Error())
			}
			return ctx.Out.PrintData(row)
		},
	}
}

func cmdPlanShow(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "show SLUG_OR_ID",
		Short: "Show plan header and milestones",
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
			p, ms, err := planning.ShowPlan(ctx.DB, args[0])
			if err != nil {
				return notFoundError(err.Error())
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(map[string]any{"plan": p, "milestones": ms})
			}
			lines := []string{fmt.Sprintf("# %s (%s)", p.Title, p.Slug), fmt.Sprintf("status: %s", p.Status)}
			for _, m := range ms {
				lines = append(lines, fmt.Sprintf("- M%d %s [%s]", m.Number, m.Title, m.Status))
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
}

func cmdPlanTree(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "tree SLUG_OR_ID",
		Short: "Plan hierarchy",
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
			tree, err := planning.PlanTree(ctx.DB, args[0])
			if err != nil {
				return notFoundError(err.Error())
			}
			return ctx.Out.PrintData(tree)
		},
	}
}

func cmdPlanStatus(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Plan delivery counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			var planned, inProg, done int
			_ = ctx.DB.QueryRow(`SELECT COUNT(*) FROM plan_items WHERE status='planned'`).Scan(&planned)
			_ = ctx.DB.QueryRow(`SELECT COUNT(*) FROM plan_items WHERE status='in_progress'`).Scan(&inProg)
			_ = ctx.DB.QueryRow(`SELECT COUNT(*) FROM plan_items WHERE status='done'`).Scan(&done)
			payload := map[string]int{"planned": planned, "in_progress": inProg, "done": done}
			return ctx.Out.PrintData(payload)
		},
	}
}

func cmdPlanMilestoneAdd(open opener) *cobra.Command {
	var planID, body string
	var number int
	c := &cobra.Command{
		Use:   "add TITLE",
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
			if planID == "" {
				return usageError("--plan is required")
			}
			resolved, err := planning.ResolvePlanID(ctx.DB, planID)
			if err != nil {
				return err
			}
			planID = resolved
			id, err := planning.AddMilestone(ctx.DB, planID, strings.Join(args, " "), body, ctx.ModelID, number)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "milestone"})
		},
	}
	c.Flags().StringVar(&planID, "plan", "", "plan id")
	c.Flags().StringVar(&body, "body", "", "milestone body")
	c.Flags().IntVar(&number, "number", 0, "milestone number (auto when 0)")
	return c
}

func cmdPlanMilestoneList(open opener) *cobra.Command {
	var planID string
	c := &cobra.Command{
		Use:   "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if planID == "" {
				return usageError("--plan is required")
			}
			rows, err := planning.ListMilestones(ctx.DB, planID)
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(rows)
		},
	}
	c.Flags().StringVar(&planID, "plan", "", "plan id")
	return c
}

func cmdPlanMilestoneStatus(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "status ID STATUS",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			_, err = ctx.DB.Exec(`UPDATE milestones SET status=? WHERE id=? OR id LIKE ?`,
				args[1], args[0], args[0]+"%")
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(args[0], map[string]any{"kind": "milestone", "action": "status"})
		},
	}
}

func cmdPlanFileAdd(open opener) *cobra.Command {
	var planItem, path, role string
	c := &cobra.Command{
		Use:   "add",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if planItem == "" || path == "" || role == "" {
				return usageError("--plan-item, --path, and --role are required")
			}
			id, err := planning.AddPlanFile(ctx.DB, planItem, path, role, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "plan_file"})
		},
	}
	c.Flags().StringVar(&planItem, "plan-item", "", "plan item id")
	c.Flags().StringVar(&path, "path", "", "file path")
	c.Flags().StringVar(&role, "role", "", "create|modify|forbidden|touched")
	return c
}

func cmdPlanItemList(open opener) *cobra.Command {
	var planID, milestoneID, st string
	var legacy bool
	c := &cobra.Command{
		Use:   "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rows, err := planning.ListItems(ctx.DB, planning.ItemFilter{
				PlanID: planID, MilestoneID: milestoneID, Status: st, LegacyOnly: legacy, Limit: effectiveListLimit(50),
			})
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(rows)
		},
	}
	c.Flags().StringVar(&planID, "plan", "", "plan id")
	c.Flags().StringVar(&milestoneID, "milestone", "", "milestone id")
	c.Flags().StringVar(&st, "status", "", "status filter")
	c.Flags().BoolVar(&legacy, "legacy", false, "only legacy flat items")
	return c
}

func cmdPlanItemStatus(open opener) *cobra.Command {
	var note string
	c := &cobra.Command{
		Use:   "status ID STATUS",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			id, err := planning.SetItemStatusExplicit(ctx.DB, args[0], args[1], note, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "plan_item", "action": "status"})
		},
	}
	c.Flags().StringVar(&note, "note", "", "status note")
	return c
}
