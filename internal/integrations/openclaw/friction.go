package openclaw

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/spf13/cobra"
)

var defaultFrictionMarkers = []string{"froke", "friction", "lesson learned", "lesson:", "broken", "broken:"}

// FrictionResult summarizes one memory scan.
type FrictionResult struct {
	FilesScanned int `json:"files_scanned"`
	Hits         int `json:"hits"`
	Created      int `json:"created"`
	Skipped      int `json:"skipped"`
}

// ScanFriction turns marked daily-memory lines into deduplicated feedback.
func ScanFriction(db *sql.DB, workspace string, markers []string, modelID string) (FrictionResult, error) {
	if len(markers) == 0 {
		markers = defaultFrictionMarkers
	}
	pattern, err := markerPattern(markers)
	if err != nil {
		return FrictionResult{}, err
	}
	discovery, err := Discover(workspace)
	if err != nil {
		return FrictionResult{}, err
	}
	existing, err := feedback.List(db, "", 0)
	if err != nil {
		return FrictionResult{}, err
	}
	identities := map[string]bool{}
	for _, row := range existing {
		if identity := frictionIdentityFromContext(row.Context); identity != "" {
			identities[identity] = true
		}
	}

	result := FrictionResult{}
	for _, file := range discovery.Files {
		if file.Kind != "daily" || !file.Exists {
			continue
		}
		result.FilesScanned++
		abs := filepath.Join(workspace, filepath.FromSlash(file.Path))
		f, err := os.Open(abs)
		if err != nil {
			return result, fmt.Errorf("open %s: %w", file.Path, err)
		}
		scanner := bufio.NewScanner(f)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !pattern.MatchString(line) {
				continue
			}
			result.Hits++
			identity := frictionIdentity(file.Path, line)
			if identities[identity] {
				result.Skipped++
				continue
			}
			note := fmt.Sprintf("%s  [src: %s:%d]", line, file.Path, lineNumber)
			context := fmt.Sprintf("openclaw-friction:%s source=%s", identity, MemoryRef(file.Path, fmt.Sprintf("L%d", lineNumber)))
			if _, err := feedback.Add(db, feedback.AddInput{
				Role: "codebase", Category: "dogfood", Severity: "med",
				Note: note, Context: context, ModelID: modelID,
			}); err != nil {
				_ = f.Close()
				return result, err
			}
			identities[identity] = true
			result.Created++
		}
		if err := scanner.Err(); err != nil {
			_ = f.Close()
			return result, fmt.Errorf("scan %s: %w", file.Path, err)
		}
		if err := f.Close(); err != nil {
			return result, err
		}
	}
	return result, nil
}

func markerPattern(markers []string) (*regexp.Regexp, error) {
	parts := make([]string, 0, len(markers))
	for _, marker := range markers {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			return nil, fmt.Errorf("friction marker cannot be empty")
		}
		parts = append(parts, regexp.QuoteMeta(marker))
	}
	return regexp.Compile(`(?i)(` + strings.Join(parts, "|") + `)`)
}

func frictionIdentity(relPath, line string) string {
	hash := sha256.Sum256([]byte(filepath.ToSlash(relPath) + "\x00" + strings.TrimSpace(line)))
	return hex.EncodeToString(hash[:])
}

func frictionIdentityFromContext(context string) string {
	const prefix = "openclaw-friction:"
	index := strings.Index(context, prefix)
	if index < 0 {
		return ""
	}
	value := context[index+len(prefix):]
	if end := strings.IndexAny(value, " \n\t"); end >= 0 {
		value = value[:end]
	}
	return value
}

func newFrictionCommand(opts *commandOptions) *cobra.Command {
	var markers []string
	parent := &cobra.Command{Use: "friction", Short: "Scan memory for friction markers"}
	scan := &cobra.Command{
		Use:   "scan",
		Short: "Create deduplicated feedback from friction markers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := openRuntime(opts, true)
			if err != nil {
				return err
			}
			defer runtime.close()
			result, err := ScanFriction(runtime.app.DB, runtime.config.Workspace, markers, runtime.app.ModelID)
			if err != nil {
				return err
			}
			if runtime.config.JSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "files=%d hits=%d created=%d skipped=%d\n", result.FilesScanned, result.Hits, result.Created, result.Skipped)
			return nil
		},
	}
	scan.Flags().StringSliceVar(&markers, "marker", nil, "case-insensitive literal marker (repeatable or comma-separated)")
	parent.AddCommand(scan)
	return parent
}
