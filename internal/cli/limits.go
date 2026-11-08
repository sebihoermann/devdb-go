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

// listLimitOr resolves a verb's own --limit flag against the global --all
// switch. A caller that did not pass --limit gets defaultLimit; --all wins
// over both and means unlimited. Note the domain layer reads 0 as "no LIMIT
// clause", which is why the default has to be substituted here rather than
// passed through.
func listLimitOr(limit, defaultLimit int) int {
	if flagAll {
		return 0
	}
	if limit <= 0 {
		return defaultLimit
	}
	return limit
}
