package planning

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// Plan is a structured plan header.
type Plan struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// PlanItem is a unit of work.
type PlanItem struct {
	ID          string `json:"id"`
	PlanID      string `json:"plan_id,omitempty"`
	MilestoneID string `json:"milestone_id,omitempty"`
	ItemNumber  int    `json:"item_number,omitempty"`
	Title       string `json:"title"`
	Body        string `json:"body,omitempty"`
	Status      string `json:"status"`
	Approval    string `json:"approval_status"`
	CreatedAt   string `json:"created_at"`
	Phase       string `json:"phase,omitempty"`
	Step        string `json:"step,omitempty"`
	MemoryRef   string `json:"memory_ref,omitempty"`
}

// Acceptance is a plan item criterion.
type Acceptance struct {
	ID        string `json:"id"`
	Ordinal   int    `json:"ordinal"`
	Criterion string `json:"criterion"`
	Status    string `json:"status"`
	Evidence  string `json:"evidence,omitempty"`
}

// CreatePlanInput for plan create.
type CreatePlanInput struct {
	Slug    string
	Title   string
	Body    string
	ModelID string
}

// CreatePlan inserts a plan and returns its id.
func CreatePlan(db *sql.DB, in CreatePlanInput) (string, error) {
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	slug := in.Slug
	if slug == "" {
		slug = slugify(in.Title)
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO plans(id, slug, title, body, status, created_at, model_id)
		VALUES (?, ?, ?, ?, 'active', ?, ?)`,
		id, slug, in.Title, nullStr(in.Body), now, in.ModelID,
	)
	return id, err
}

// AddItemInput for plan item add.
type AddItemInput struct {
	PlanID      string
	MilestoneID string
	Title       string
	Body        string
	MemoryRef   string
	ModelID     string
}

// AddItem creates a plan item under a plan/milestone.
func AddItem(db *sql.DB, in AddItemInput) (string, error) {
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	ordinal, err := nextItemNumber(db, in.PlanID, in.MilestoneID)
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO plan_items(id, plan_id, milestone_id, item_number, title, body, memory_ref, status, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'planned', ?, ?)`,
		id, nullStr(in.PlanID), nullStr(in.MilestoneID), ordinal, in.Title, nullStr(in.Body), nullStr(in.MemoryRef), now, in.ModelID,
	)
	return id, err
}

func nextItemNumber(db *sql.DB, planID, milestoneID string) (int, error) {
	var n sql.NullInt64
	err := db.QueryRow(`
		SELECT MAX(item_number) FROM plan_items
		WHERE plan_id IS ? AND milestone_id IS ?`,
		nullStr(planID), nullStr(milestoneID),
	).Scan(&n)
	if err != nil {
		return 0, err
	}
	return int(n.Int64) + 1, nil
}

// AddAcceptance adds a criterion to a plan item.
func AddAcceptance(db *sql.DB, planItemID, criterion, modelID string, ordinal int) (string, error) {
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	if ordinal <= 0 {
		var max sql.NullInt64
		_ = db.QueryRow(`SELECT MAX(ordinal) FROM plan_item_acceptance WHERE plan_item_id=?`, planItemID).Scan(&max)
		ordinal = int(max.Int64) + 1
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO plan_item_acceptance(id, plan_item_id, ordinal, criterion, status, created_at, updated_at, model_id)
		VALUES (?, ?, ?, ?, 'open', ?, ?, ?)`,
		id, planItemID, ordinal, criterion, now, now, modelID,
	)
	return id, err
}

// SetItemStatus updates plan item status and logs it.
func SetItemStatus(db *sql.DB, itemID, status, note, modelID string) error {
	now := storage.NowUTC()
	if _, err := db.Exec(`UPDATE plan_items SET status=? WHERE id=?`, status, itemID); err != nil {
		return err
	}
	logID, err := storage.NewID()
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO status_log(id, plan_item_id, status, note, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?)`,
		logID, itemID, status, nullStr(note), now, modelID,
	)
	return err
}

// StartItem marks a plan item in progress.
func StartItem(db *sql.DB, idPrefix, modelID string) (string, error) {
	id, err := resolveItemID(db, idPrefix)
	if err != nil {
		return "", err
	}
	if err := SetItemStatus(db, id, "in_progress", "started", modelID); err != nil {
		return "", err
	}
	return id, nil
}

// PauseItem marks in-progress work paused with a required note.
func PauseItem(db *sql.DB, idPrefix, note, modelID string) (string, error) {
	if strings.TrimSpace(note) == "" {
		return "", fmt.Errorf("--note is required on pause")
	}
	id, err := resolveItemID(db, idPrefix)
	if err != nil {
		return "", err
	}
	if err := SetItemStatus(db, id, "in_progress", "paused: "+note, modelID); err != nil {
		return "", err
	}
	return id, nil
}

// InFlight returns the most recent in-progress plan item if any.
func InFlight(db *sql.DB) (*PlanItem, error) {
	var p PlanItem
	var body, planID, milestoneID sql.NullString
	var itemNum sql.NullInt64
	err := db.QueryRow(`
		SELECT id, COALESCE(plan_id,''), COALESCE(milestone_id,''), COALESCE(item_number,0),
		       title, COALESCE(body,''), status, approval_status, created_at
		FROM plan_items
		WHERE status='in_progress'
		ORDER BY created_at DESC
		LIMIT 1`,
	).Scan(&p.ID, &planID, &milestoneID, &itemNum, &p.Title, &body, &p.Status, &p.Approval, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.PlanID = planID.String
	p.MilestoneID = milestoneID.String
	p.ItemNumber = int(itemNum.Int64)
	p.Body = body.String
	return &p, nil
}

// ShowItem returns plan item detail with acceptance rows.
func ShowItem(db *sql.DB, idPrefix string) (PlanItem, []Acceptance, error) {
	id, err := resolveItemID(db, idPrefix)
	if err != nil {
		return PlanItem{}, nil, err
	}
	var p PlanItem
	var body, planID, milestoneID, memoryRef sql.NullString
	var itemNum sql.NullInt64
	err = db.QueryRow(`
		SELECT id, COALESCE(plan_id,''), COALESCE(milestone_id,''), COALESCE(item_number,0),
		       title, COALESCE(body,''), status, approval_status, created_at,
		       COALESCE(phase,''), COALESCE(step,''), COALESCE(memory_ref,'')
		FROM plan_items WHERE id=?`, id,
	).Scan(&p.ID, &planID, &milestoneID, &itemNum, &p.Title, &body, &p.Status, &p.Approval, &p.CreatedAt, &p.Phase, &p.Step, &memoryRef)
	if err != nil {
		return PlanItem{}, nil, err
	}
	p.PlanID = planID.String
	p.MilestoneID = milestoneID.String
	p.ItemNumber = int(itemNum.Int64)
	p.Body = body.String
	p.MemoryRef = memoryRef.String

	rows, err := db.Query(`
		SELECT id, ordinal, criterion, status, COALESCE(evidence,'')
		FROM plan_item_acceptance WHERE plan_item_id=? ORDER BY ordinal`, id)
	if err != nil {
		return p, nil, err
	}
	defer rows.Close()
	var acc []Acceptance
	for rows.Next() {
		var a Acceptance
		if err := rows.Scan(&a.ID, &a.Ordinal, &a.Criterion, &a.Status, &a.Evidence); err != nil {
			return p, nil, err
		}
		acc = append(acc, a)
	}
	return p, acc, rows.Err()
}

// MeetAcceptance marks a criterion met with evidence.
func MeetAcceptance(db *sql.DB, accPrefix, evidence, modelID string) (string, error) {
	rows, err := db.Query(`SELECT id FROM plan_item_acceptance`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	id, err := storage.ResolveID(accPrefix, ids)
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		UPDATE plan_item_acceptance SET status='met', evidence=?, updated_at=?, model_id=?
		WHERE id=?`, evidence, now, modelID, id)
	return id, err
}

// CloseItem closes a plan item when all acceptance criteria are met.
// As a side-effect, if the item belongs to a milestone and all sibling
// items in that milestone are now done, the milestone is also marked done.
func CloseItem(db *sql.DB, idPrefix, evidence, modelID string) (string, error) {
	id, err := resolveItemID(db, idPrefix)
	if err != nil {
		return "", err
	}
	var openCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM plan_item_acceptance
		WHERE plan_item_id=? AND status='open'`, id).Scan(&openCount)
	if err != nil {
		return "", err
	}
	if openCount > 0 {
		return "", fmt.Errorf("cannot close: %d open acceptance criteria — meet them first", openCount)
	}
	note := "closed"
	if evidence != "" {
		note = "closed: " + evidence
	}
	if err := SetItemStatus(db, id, "done", note, modelID); err != nil {
		return "", err
	}
	if err := maybeCompleteMilestone(db, id, modelID); err != nil {
		return id, fmt.Errorf("item closed but milestone rollup failed: %w", err)
	}
	return id, nil
}

// maybeCompleteMilestone marks the parent milestone done when no items
// in it are still open. Items without a milestone are skipped silently.
func maybeCompleteMilestone(db *sql.DB, itemID, modelID string) error {
	var milestoneID sql.NullString
	if err := db.QueryRow(`SELECT milestone_id FROM plan_items WHERE id=?`, itemID).Scan(&milestoneID); err != nil {
		return err
	}
	if !milestoneID.Valid {
		return nil
	}
	var openCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM plan_items
		WHERE milestone_id=? AND status NOT IN ('done','wontfix')`,
		milestoneID.String,
	).Scan(&openCount); err != nil {
		return err
	}
	if openCount > 0 {
		return nil
	}
	_, err := db.Exec(`UPDATE milestones SET status='done' WHERE id=? AND status<>'done'`, milestoneID.String)
	return err
}

// ListPlans returns active plans.
func ListPlans(db *sql.DB) ([]Plan, error) {
	rows, err := db.Query(`SELECT id, slug, title, COALESCE(body,''), status, created_at FROM plans ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Milestone is a plan milestone row.
type Milestone struct {
	ID        string `json:"id"`
	PlanID    string `json:"plan_id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// PlanFile is a scoped file on a plan item.
type PlanFile struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Role string `json:"role"`
}

// AddMilestone creates a milestone under a plan.
func AddMilestone(db *sql.DB, planID, title, body, modelID string, number int) (string, error) {
	if number <= 0 {
		var max sql.NullInt64
		_ = db.QueryRow(`SELECT MAX(number) FROM milestones WHERE plan_id=?`, planID).Scan(&max)
		number = int(max.Int64) + 1
	}
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO milestones(id, plan_id, number, title, body, status, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, 'planned', ?, ?)`,
		id, planID, number, title, nullStr(body), now, modelID,
	)
	return id, err
}

// ListMilestones returns milestones for a plan.
func ListMilestones(db *sql.DB, planID string) ([]Milestone, error) {
	rows, err := db.Query(`
		SELECT id, plan_id, number, title, COALESCE(body,''), status, created_at
		FROM milestones WHERE plan_id=? ORDER BY number`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.PlanID, &m.Number, &m.Title, &m.Body, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ShowPlan returns plan header and milestones.
func ShowPlan(db *sql.DB, slugOrID string) (Plan, []Milestone, error) {
	var p Plan
	err := db.QueryRow(`
		SELECT id, slug, title, COALESCE(body,''), status, created_at
		FROM plans WHERE slug=? OR id=? OR id LIKE ?`,
		slugOrID, slugOrID, slugOrID+"%",
	).Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &p.Status, &p.CreatedAt)
	if err != nil {
		return Plan{}, nil, err
	}
	ms, err := ListMilestones(db, p.ID)
	return p, ms, err
}

// TreeNode is one row in a plan tree.
type TreeNode struct {
	Kind     string     `json:"kind"`
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Status   string     `json:"status,omitempty"`
	Children []TreeNode `json:"children,omitempty"`
}

// PlanTree builds a hierarchical view of a plan.
func PlanTree(db *sql.DB, slugOrID string) ([]TreeNode, error) {
	p, ms, err := ShowPlan(db, slugOrID)
	if err != nil {
		return nil, err
	}
	root := TreeNode{Kind: "plan", ID: p.ID, Label: p.Title, Status: p.Status}
	for _, m := range ms {
		mNode := TreeNode{Kind: "milestone", ID: m.ID, Label: fmt.Sprintf("M%d %s", m.Number, m.Title), Status: m.Status}
		items, _ := ListItems(db, ItemFilter{PlanID: p.ID, MilestoneID: m.ID, Limit: 100})
		for _, it := range items {
			mNode.Children = append(mNode.Children, TreeNode{
				Kind: "item", ID: it.ID, Label: it.Title, Status: it.Status,
			})
		}
		root.Children = append(root.Children, mNode)
	}
	legacy, _ := ListItems(db, ItemFilter{LegacyOnly: true, Limit: 50})
	for _, it := range legacy {
		label := it.Title
		if it.Phase != "" {
			label = fmt.Sprintf("[%s.%s] %s", it.Phase, it.Step, it.Title)
		}
		root.Children = append(root.Children, TreeNode{Kind: "legacy_item", ID: it.ID, Label: label, Status: it.Status})
	}
	return []TreeNode{root}, nil
}

// ItemFilter for listing plan items.
type ItemFilter struct {
	PlanID      string
	MilestoneID string
	Status      string
	LegacyOnly  bool
	Limit       int
}

// ListItems returns plan items matching filter.
func ListItems(db *sql.DB, f ItemFilter) ([]PlanItem, error) {
	q := `SELECT id, COALESCE(plan_id,''), COALESCE(milestone_id,''), COALESCE(item_number,0),
	      title, COALESCE(body,''), status, approval_status, created_at,
	      COALESCE(phase,''), COALESCE(step,'')
	      FROM plan_items WHERE 1=1`
	args := []any{}
	if f.LegacyOnly {
		q += ` AND plan_id IS NULL`
	} else if f.PlanID != "" {
		q += ` AND plan_id = ?`
		args = append(args, f.PlanID)
	}
	if f.MilestoneID != "" {
		q += ` AND milestone_id = ?`
		args = append(args, f.MilestoneID)
	}
	if f.Status != "" && f.Status != "all" {
		q += ` AND status = ?`
		args = append(args, f.Status)
	}
	q += ` ORDER BY created_at DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		var p PlanItem
		if err := rows.Scan(&p.ID, &p.PlanID, &p.MilestoneID, &p.ItemNumber, &p.Title, &p.Body,
			&p.Status, &p.Approval, &p.CreatedAt, &p.Phase, &p.Step); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddLegacyItem creates a flat plan item (no plan_id).
func AddLegacyItem(db *sql.DB, phase, step, title, body, memoryRef, modelID string) (string, error) {
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO plan_items(id, phase, step, title, body, memory_ref, status, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, 'planned', ?, ?)`,
		id, nullStr(phase), nullStr(step), title, nullStr(body), nullStr(memoryRef), now, modelID,
	)
	return id, err
}

// AddPlanFile attaches a scoped file to a plan item.
func AddPlanFile(db *sql.DB, planItemID, path, role, modelID string) (string, error) {
	switch role {
	case "create", "modify", "forbidden", "touched":
	default:
		return "", fmt.Errorf("role must be create, modify, forbidden, or touched")
	}
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO plan_item_files(id, plan_item_id, path, role, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, planItemID, path, role, now, modelID,
	)
	return id, err
}

// ListPlanFiles returns files for a plan item.
func ListPlanFiles(db *sql.DB, planItemID string) ([]PlanFile, error) {
	rows, err := db.Query(`
		SELECT id, path, role FROM plan_item_files WHERE plan_item_id=? ORDER BY path`, planItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanFile
	for rows.Next() {
		var f PlanFile
		if err := rows.Scan(&f.ID, &f.Path, &f.Role); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SetItemStatusExplicit sets status with audit log.
func SetItemStatusExplicit(db *sql.DB, idPrefix, status, note, modelID string) (string, error) {
	id, err := resolveItemID(db, idPrefix)
	if err != nil {
		return "", err
	}
	if err := SetItemStatus(db, id, status, note, modelID); err != nil {
		return "", err
	}
	return id, nil
}

func resolveItemID(db *sql.DB, prefix string) (string, error) {
	rows, err := db.Query(`SELECT id FROM plan_items`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	return storage.ResolveID(prefix, ids)
}

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func nullStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
