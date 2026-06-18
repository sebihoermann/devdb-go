package hub

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// SyncResult summarizes a hub sync run.
type SyncResult struct {
	RunID           string   `json:"run_id"`
	Status          string   `json:"status"`
	ProjectsSeen    int      `json:"projects_seen"`
	ProjectsUpdated int      `json:"projects_updated"`
	ProjectsFailed  int      `json:"projects_failed"`
	Errors          []string `json:"errors,omitempty"`
}

// ListEntry is one registered project with optional hub snapshot.
type ListEntry struct {
	Alias    string            `json:"alias"`
	Root     string            `json:"root"`
	DBPath   string            `json:"db_path"`
	Exists   bool              `json:"exists"`
	SyncedAt string            `json:"synced_at,omitempty"`
	Status   string            `json:"status,omitempty"`
	Counts   map[string]int    `json:"counts,omitempty"`
	Snapshot *Snapshot         `json:"snapshot,omitempty"`
}

// DashboardRow is a compact hub dashboard line.
type DashboardRow struct {
	Alias           string `json:"alias"`
	Root            string `json:"root_path"`
	Status          string `json:"status"`
	WorkStatus      string `json:"work_status"`
	AttentionScore  int    `json:"attention_score"`
	StatusReason    string `json:"status_reason,omitempty"`
	InProgress      int    `json:"in_progress_plan_items"`
	OpenItems       int    `json:"open_plan_items"`
	OpenFeedback    int    `json:"open_feedback"`
	OpenHighFinding int    `json:"open_high_findings"`
	StaleArch       int    `json:"stale_arch_notes"`
	Verification    string `json:"latest_verification_freshness,omitempty"`
	GitBranch       string `json:"git_branch,omitempty"`
	GitDirty        bool   `json:"git_dirty"`
	SyncedAt        string `json:"synced_at,omitempty"`
}

// ProjectDetail is hub drill-down for one project.
type ProjectDetail struct {
	Alias      string          `json:"alias"`
	Root       string          `json:"root_path"`
	DBPath     string          `json:"db_path"`
	Status     string          `json:"status"`
	SyncedAt   string          `json:"synced_at,omitempty"`
	Snapshot   Snapshot        `json:"snapshot"`
	Attention  []AttentionItem `json:"attention"`
}

// DoctorRow is sync diagnostic for one project.
type DoctorRow struct {
	Alias            string `json:"alias"`
	RootPath         string `json:"root_path"`
	DBPath           string `json:"db_path"`
	SourceExists     bool   `json:"source_exists"`
	HubSyncedAt      string `json:"hub_synced_at,omitempty"`
	HubStatus        string `json:"hub_status"`
	SourceDBMTime    string `json:"source_db_mtime,omitempty"`
	FreshnessStatus  string `json:"freshness_status"`
	LastSyncError    string `json:"last_sync_error,omitempty"`
	RecommendedCmd   string `json:"recommended_command"`
}

// OpenHub opens (and migrates) the metadata hub database.
func OpenHub(metadataDB string) (*sql.DB, error) {
	path := ResolveMetadataDB(metadataDB)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := storage.Open(path)
	if err != nil {
		return nil, err
	}
	if err := migrate.RunHub(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// Register adds a project to the registry and hub, then syncs when possible.
func Register(root, alias, registry, metadataDB string) (RegisteredProject, error) {
	root, err := filepath.Abs(expandHome(root))
	if err != nil {
		return RegisteredProject{}, err
	}
	if alias == "" {
		alias = defaultAlias(root)
	}
	project := RegisteredProject{
		Alias:  alias,
		Root:   root,
		DBPath: filepath.Join(root, ".devdb", "development.db"),
	}
	if _, err := os.Stat(project.DBPath); err == nil {
		project.Exists = true
	}

	projects, err := ReadRegistry(registry)
	if err != nil {
		return RegisteredProject{}, err
	}
	filtered := projects[:0]
	for _, p := range projects {
		if p.Alias == alias || p.Root == root {
			continue
		}
		filtered = append(filtered, p)
	}
	filtered = append(filtered, project)
	if err := WriteRegistry(registry, filtered); err != nil {
		return RegisteredProject{}, err
	}

	hub, err := OpenHub(metadataDB)
	if err != nil {
		return project, err
	}
	defer hub.Close()

	now := storage.NowUTC()
	_, err = hub.Exec(`
		INSERT INTO projects(alias, root_path, registered_at) VALUES (?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET root_path=excluded.root_path`,
		project.Alias, project.Root, now)
	if err != nil {
		return project, err
	}
	if project.Exists {
		_, _ = SyncOne(hub, project)
	}
	return project, nil
}

// SyncAll refreshes every registered project into the hub.
func SyncAll(registry, metadataDB string, strict bool) (SyncResult, error) {
	projects, err := ReadRegistry(registry)
	if err != nil {
		return SyncResult{}, err
	}
	hub, err := OpenHub(metadataDB)
	if err != nil {
		return SyncResult{}, err
	}
	defer hub.Close()

	runID, _ := storage.NewID()
	res := SyncResult{RunID: runID, Status: "passed"}
	for _, p := range projects {
		res.ProjectsSeen++
		ok, syncErr := SyncOne(hub, p)
		if ok {
			res.ProjectsUpdated++
		} else {
			res.ProjectsFailed++
			if syncErr != "" {
				res.Errors = append(res.Errors, p.Alias+": "+syncErr)
			}
		}
	}
	if res.ProjectsFailed > 0 && strict {
		res.Status = "failed"
	}
	return res, nil
}

// SyncOne collects and stores a project snapshot.
func SyncOne(hub *sql.DB, p RegisteredProject) (bool, string) {
	snap := CollectSnapshot(p.Root, p.DBPath)
	now := storage.NowUTC()
	raw, err := encodeSnapshot(snap)
	if err != nil {
		return false, err.Error()
	}

	_, err = hub.Exec(`
		INSERT INTO projects(alias, root_path, registered_at) VALUES (?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET root_path=excluded.root_path`,
		p.Alias, p.Root, now)
	if err != nil {
		return false, err.Error()
	}

	_, err = hub.Exec(`
		INSERT INTO project_snapshots(alias, root_path, synced_at, sync_status, snapshot_json)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET
			root_path=excluded.root_path,
			synced_at=excluded.synced_at,
			sync_status=excluded.sync_status,
			snapshot_json=excluded.snapshot_json`,
		p.Alias, p.Root, now, snap.SyncStatus, raw)
	if err != nil {
		return false, err.Error()
	}

	markSourceSynced(p.DBPath, now)
	if snap.SyncStatus == "active" {
		return true, ""
	}
	if snap.Error != "" {
		return false, snap.Error
	}
	return false, snap.SyncStatus
}

func markSourceSynced(dbPath, syncedAt string) {
	if dbPath == "" {
		return
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		return
	}
	defer db.Close()
	_, _ = db.Exec(`
		INSERT INTO sync_state(key, value, updated_at) VALUES ('last_hub_sync_at', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		syncedAt, syncedAt)
	_, _ = db.Exec(`
		INSERT INTO sync_state(key, value, updated_at) VALUES ('metadata_dirty', '0', ?)
		ON CONFLICT(key) DO UPDATE SET value='0', updated_at=excluded.updated_at`,
		syncedAt)
}

// List returns registered projects, optionally refreshing stale snapshots.
func List(registry, metadataDB string, refreshDirty bool) ([]ListEntry, error) {
	projects, err := ReadRegistry(registry)
	if err != nil {
		return nil, err
	}
	hub, err := OpenHub(metadataDB)
	if err != nil {
		return nil, err
	}
	defer hub.Close()

	if refreshDirty {
		for _, p := range projects {
			if needsRefresh(hub, p) {
				_, _ = SyncOne(hub, p)
			}
		}
	}

	var out []ListEntry
	for _, p := range projects {
		entry := ListEntry{
			Alias:  p.Alias,
			Root:   p.Root,
			DBPath: p.DBPath,
			Exists: p.Exists,
		}
		var syncedAt, syncStatus, raw string
		err := hub.QueryRow(`
			SELECT synced_at, sync_status, snapshot_json FROM project_snapshots WHERE alias=?`,
			p.Alias).Scan(&syncedAt, &syncStatus, &raw)
		if err == nil {
			entry.SyncedAt = syncedAt
			entry.Status = syncStatus
			snap, _ := decodeSnapshot(raw)
			entry.Snapshot = &snap
			entry.Counts = map[string]int{
				"feedback":           snap.OpenFeedback,
				"architecture_notes": snap.StaleArchNotes,
				"review_findings":    snap.OpenFindings,
			}
		} else if !p.Exists {
			entry.Status = "missing"
		}
		out = append(out, entry)
	}
	return out, nil
}

func needsRefresh(hub *sql.DB, p RegisteredProject) bool {
	if !p.Exists {
		return false
	}
	var syncedAt string
	err := hub.QueryRow(`SELECT synced_at FROM project_snapshots WHERE alias=?`, p.Alias).Scan(&syncedAt)
	if err != nil {
		return true
	}
	if readSourceDirty(p.DBPath) {
		return true
	}
	lastHub := readLastHubSync(p.DBPath)
	if lastHub == "" {
		return sourceDBNewerThan(p.DBPath, syncedAt)
	}
	hubT, err1 := time.Parse(time.RFC3339Nano, syncedAt)
	srcT, err2 := time.Parse(time.RFC3339Nano, lastHub)
	if err1 != nil || err2 != nil {
		return true
	}
	return srcT.After(hubT.UTC())
}

// Dashboard returns sorted dashboard rows for a view.
func Dashboard(registry, metadataDB, view string, attentionOnly bool) ([]DashboardRow, error) {
	entries, err := List(registry, metadataDB, true)
	if err != nil {
		return nil, err
	}
	var rows []DashboardRow
	for _, e := range entries {
		if e.Snapshot == nil {
			rows = append(rows, DashboardRow{
				Alias: e.Alias, Root: e.Root, Status: e.Status,
				WorkStatus: "attention", AttentionScore: 50,
				StatusReason: "never synced",
			})
			continue
		}
		s := *e.Snapshot
		row := DashboardRow{
			Alias:           e.Alias,
			Root:            e.Root,
			Status:          e.Status,
			WorkStatus:      s.WorkStatus,
			AttentionScore:  s.AttentionScore,
			StatusReason:    s.StatusReason,
			InProgress:      s.InProgressItems,
			OpenItems:       s.OpenPlanItems,
			OpenFeedback:    s.OpenFeedback,
			OpenHighFinding: s.OpenHighFindings,
			StaleArch:       s.StaleArchNotes,
			Verification:    s.LatestVerificationFreshness,
			GitBranch:       s.GitBranch,
			GitDirty:        s.GitDirty,
			SyncedAt:        e.SyncedAt,
		}
		rows = append(rows, row)
	}
	if attentionOnly {
		filtered := rows[:0]
		for _, r := range rows {
			if r.Status == "missing" || r.Status == "error" ||
				r.WorkStatus == "blocked" || r.WorkStatus == "attention" ||
				r.AttentionScore >= 10 || verificationNeedsAttention(r.Verification) {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AttentionScore != rows[j].AttentionScore {
			return rows[i].AttentionScore > rows[j].AttentionScore
		}
		return rows[i].Alias < rows[j].Alias
	})
	_ = view // views share columns in compact output; JSON carries full snapshot via project detail
	return rows, nil
}

func verificationNeedsAttention(freshness string) bool {
	if freshness == "" {
		return false
	}
	return freshness != "fresh"
}

// Project returns detail for alias or absolute path.
func Project(registry, metadataDB, key string) (*ProjectDetail, error) {
	projects, err := ReadRegistry(registry)
	if err != nil {
		return nil, err
	}
	key = strings.TrimSpace(key)
	resolved := key
	if st, err := os.Stat(expandHome(key)); err == nil && st.IsDir() {
		resolved, _ = filepath.Abs(expandHome(key))
	}
	var match *RegisteredProject
	for i := range projects {
		if projects[i].Alias == key || projects[i].Root == resolved {
			match = &projects[i]
			break
		}
	}
	if match == nil {
		return nil, fmt.Errorf("project not found: %s", key)
	}

	hub, err := OpenHub(metadataDB)
	if err != nil {
		return nil, err
	}
	defer hub.Close()

	if needsRefresh(hub, *match) {
		_, _ = SyncOne(hub, *match)
	}

	var syncedAt, syncStatus, raw string
	err = hub.QueryRow(`
		SELECT synced_at, sync_status, snapshot_json FROM project_snapshots WHERE alias=?`,
		match.Alias).Scan(&syncedAt, &syncStatus, &raw)
	snap, _ := decodeSnapshot(raw)
	if err != nil {
		snap = CollectSnapshot(match.Root, match.DBPath)
		syncStatus = snap.SyncStatus
	}

	return &ProjectDetail{
		Alias:     match.Alias,
		Root:      match.Root,
		DBPath:    match.DBPath,
		Status:    syncStatus,
		SyncedAt:  syncedAt,
		Snapshot:  snap,
		Attention: snap.AttentionItems,
	}, nil
}

// Doctor returns per-project sync diagnostics.
func Doctor(registry, metadataDB, projectFilter string) ([]DoctorRow, error) {
	projects, err := ReadRegistry(registry)
	if err != nil {
		return nil, err
	}
	hub, err := OpenHub(metadataDB)
	if err != nil {
		return nil, err
	}
	defer hub.Close()

	filterKey := projectFilter
	if filterKey != "" {
		if st, err := os.Stat(expandHome(filterKey)); err == nil && st.IsDir() {
			filterKey, _ = filepath.Abs(expandHome(filterKey))
		}
	}

	var rows []DoctorRow
	for _, p := range projects {
		if filterKey != "" && filterKey != p.Alias && filterKey != p.Root {
			continue
		}
		row := DoctorRow{
			Alias:        p.Alias,
			RootPath:     p.Root,
			DBPath:       p.DBPath,
			SourceExists: p.Exists,
		}
		if info, err := os.Stat(p.DBPath); err == nil {
			row.SourceDBMTime = info.ModTime().UTC().Format(time.RFC3339Nano)
		}
		var syncedAt, syncStatus, raw string
		err := hub.QueryRow(`
			SELECT synced_at, sync_status, snapshot_json FROM project_snapshots WHERE alias=?`,
			p.Alias).Scan(&syncedAt, &syncStatus, &raw)
		if err == nil {
			row.HubSyncedAt = syncedAt
			row.HubStatus = syncStatus
			snap, _ := decodeSnapshot(raw)
			if snap.Error != "" {
				row.LastSyncError = snap.Error
			}
		} else if !p.Exists {
			row.HubStatus = "missing"
		} else {
			row.HubStatus = "unsynced"
		}

		row.FreshnessStatus, row.RecommendedCmd = diagnoseFreshness(p, row)
		if row.LastSyncError == "" && row.FreshnessStatus == "error" {
			row.LastSyncError = readSourceSyncError(p.DBPath)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Alias < rows[j].Alias })
	return rows, nil
}

func diagnoseFreshness(p RegisteredProject, row DoctorRow) (status, cmd string) {
	if !p.Exists {
		return "missing", "devdb init"
	}
	if row.HubSyncedAt == "" {
		return "stale", "devdb hub sync"
	}
	if readSourceDirty(p.DBPath) {
		return "dirty", "devdb hub sync"
	}
	if lastHub := readLastHubSync(p.DBPath); lastHub != "" {
		hubT, err1 := time.Parse(time.RFC3339Nano, row.HubSyncedAt)
		srcT, err2 := time.Parse(time.RFC3339Nano, lastHub)
		if err1 == nil && err2 == nil {
			if srcT.After(hubT.UTC()) {
				return "dirty", "devdb hub sync"
			}
			if row.LastSyncError != "" {
				return "error", "devdb hub sync"
			}
			return "fresh", ""
		}
	}
	if row.SourceDBMTime != "" && readLastHubSync(p.DBPath) == "" {
		if sourceDBNewerThan(p.DBPath, row.HubSyncedAt) {
			return "dirty", "devdb hub sync"
		}
	}
	if row.LastSyncError != "" {
		return "error", "devdb hub sync"
	}
	if row.HubSyncedAt != "" {
		return "fresh", ""
	}
	return "unknown", "devdb hub sync"
}

func sourceDBNewerThan(dbPath, syncedAt string) bool {
	info, err := os.Stat(dbPath)
	if err != nil {
		return false
	}
	synced, err := time.Parse(time.RFC3339Nano, syncedAt)
	if err != nil {
		return true
	}
	return info.ModTime().UTC().After(synced.UTC())
}

func readSourceDirty(dbPath string) bool {
	db, err := storage.Open(dbPath)
	if err != nil {
		return false
	}
	defer db.Close()
	var val string
	err = db.QueryRow(`SELECT value FROM sync_state WHERE key='metadata_dirty'`).Scan(&val)
	return err == nil && val == "1"
}

func readLastHubSync(dbPath string) string {
	db, err := storage.Open(dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()
	var val string
	if err := db.QueryRow(`SELECT value FROM sync_state WHERE key='last_hub_sync_at'`).Scan(&val); err != nil {
		return ""
	}
	return val
}

func readSourceSyncError(dbPath string) string {
	db, err := storage.Open(dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()
	var val string
	if err := db.QueryRow(`SELECT value FROM sync_state WHERE key='last_metadata_push_error'`).Scan(&val); err == nil {
		return val
	}
	return ""
}

// MarkDirty flags a project database as needing hub refresh after a local write.
func MarkDirty(dbPath string) {
	if dbPath == "" {
		return
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		return
	}
	defer db.Close()
	now := storage.NowUTC()
	_, _ = db.Exec(`
		INSERT INTO sync_state(key, value, updated_at) VALUES ('metadata_dirty', '1', ?)
		ON CONFLICT(key) DO UPDATE SET value='1', updated_at=excluded.updated_at`,
		now)
}
