package openclaw

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/spf13/cobra"
)

// MemoryLink joins one plan item to its opaque OpenClaw memory reference.
type MemoryLink struct {
	ItemID    string `json:"item_id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	MemoryRef string `json:"memory_ref"`
	Path      string `json:"path,omitempty"`
	Fragment  string `json:"fragment,omitempty"`
	Exists    bool   `json:"exists"`
}

// ListMemoryLinks returns referenced items for a plan.
func ListMemoryLinks(runtime *runtime, planRef string) ([]MemoryLink, error) {
	planID, err := planning.ResolvePlanID(runtime.app.DB, planRef)
	if err != nil {
		return nil, err
	}
	items, err := planning.ListItems(runtime.app.DB, planning.ItemFilter{PlanID: planID})
	if err != nil {
		return nil, err
	}
	links := make([]MemoryLink, 0)
	for _, item := range items {
		if strings.TrimSpace(item.MemoryRef) == "" {
			continue
		}
		path, fragment := splitMemoryRef(item.MemoryRef)
		link := MemoryLink{
			ItemID: item.ID, Title: item.Title, Status: item.Status,
			MemoryRef: item.MemoryRef, Path: path, Fragment: fragment,
		}
		if safeMemoryPath(path) {
			link.Exists = safeRegularFile(filepath.Join(runtime.config.Workspace, filepath.FromSlash(path)))
		}
		links = append(links, link)
	}
	return links, nil
}

func splitMemoryRef(ref string) (string, string) {
	value := strings.TrimPrefix(strings.TrimSpace(ref), "openclaw:")
	path, fragment, found := strings.Cut(value, "#")
	if !found {
		return path, ""
	}
	return path, fragment
}

func safeMemoryPath(value string) bool {
	if value == "" || filepath.IsAbs(value) {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func newLinksCommand(opts *commandOptions) *cobra.Command {
	var planRef string
	cmd := &cobra.Command{
		Use:   "links",
		Short: "Join plan items to OpenClaw memory references",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := openRuntime(opts, true)
			if err != nil {
				return err
			}
			defer runtime.close()
			links, err := ListMemoryLinks(runtime, planRef)
			if err != nil {
				return err
			}
			if runtime.config.JSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(links)
			}
			for _, link := range links {
				state := "missing"
				if link.Exists {
					state = "present"
				}
				shortID := link.ItemID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %-8s  %s  ->  %s\n", shortID, state, link.Title, link.MemoryRef)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&planRef, "plan", "", "plan slug or id")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}
