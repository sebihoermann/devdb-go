package cli

// effectiveListLimit returns 0 (unlimited) when the global --all flag is set.
func effectiveListLimit(defaultLimit int) int {
	if flagAll {
		return 0
	}
	return defaultLimit
}

// applyAllLimit overrides an explicit --limit value when --all is set.
func applyAllLimit(limit int) int {
	if flagAll {
		return 0
	}
	return limit
}
