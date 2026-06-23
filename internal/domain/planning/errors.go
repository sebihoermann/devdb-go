package planning

import "errors"

// Sentinel errors callers (CLI, tests) match with errors.Is. Wrapping keeps
// the user-facing message stable; only the matcher becomes structural.
var (
	// ErrSlugExists indicates a CreatePlan call collided with an existing
	// slug — the unique constraint on plans.slug fired.
	ErrSlugExists = errors.New("plan slug already exists")

	// ErrInvalidMode indicates ScaffoldPlan received a Mode value other
	// than "implement" or "design".
	ErrInvalidMode = errors.New("mode must be implement or design")

	// ErrSpecFileNotFound indicates the Markdown spec path passed to
	// BackfillAcceptanceFromSpec does not exist on disk.
	ErrSpecFileNotFound = errors.New("spec file not found")

	// ErrNoteRequired indicates a status-change command (pause, close)
	// was called without the required --note argument.
	ErrNoteRequired = errors.New("note is required for this status change")

	// ErrPlanNotFound indicates a plan lookup by slug/id/prefix returned
	// no matching row.
	ErrPlanNotFound = errors.New("plan not found")

	// ErrMilestoneNotFound indicates a milestone lookup by id prefix or
	// numeric reference returned no matching row.
	ErrMilestoneNotFound = errors.New("milestone not found")
)