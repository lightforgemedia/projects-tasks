package pt

import "sort"

// SortIssues sorts issues by the given key.
func SortIssues(issues []Issue, key string) {
	switch key {
	case "title":
		sort.Slice(issues, func(i, j int) bool {
			return issues[i].Title < issues[j].Title
		})
	default: // priority
		sort.Slice(issues, func(i, j int) bool {
			if issues[i].Priority == issues[j].Priority {
				return issues[i].Title < issues[j].Title
			}
			return issues[i].Priority > issues[j].Priority
		})
	}
}

// IssueExtra returns a short inline status for verbose ready output.
func IssueExtra(iss Issue) string {
	var parts []string
	if iss.Assignee != "" {
		parts = append(parts, "@"+iss.Assignee)
	}
	for _, l := range iss.Labels {
		if l == "state:blocked" || l == "blocked" {
			parts = append(parts, "blocked")
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + join(parts, ",") + "]"
}

func join(list []string, sep string) string {
	switch len(list) {
	case 0:
		return ""
	case 1:
		return list[0]
	default:
		out := list[0]
		for i := 1; i < len(list); i++ {
			out += sep + list[i]
		}
		return out
	}
}
