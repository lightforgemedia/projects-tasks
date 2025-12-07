package pt

import (
	"context"
	"strings"
)

// SearchResult represents a matched issue with a simple score.
type SearchResult struct {
	Issue Issue  `json:"issue"`
	Score int    `json:"score"`
	Match string `json:"match"`
}

// SearchOptions controls search behavior.
type SearchOptions struct {
	Query string
	Role  string
	Limit int
}

// Search performs a simple case-insensitive substring match over title/description/labels.
func (c *StoreClient) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(opts.Query))
	role := strings.TrimSpace(opts.Role)
	var out []SearchResult
	for _, iss := range c.data.Issues {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if role != "" && !c.hasLabelLocked(iss.ID, "role:"+role) {
			continue
		}
		score := 0
		match := ""
		if q == "" {
			score = 1
			match = "any"
		} else {
			if strings.Contains(strings.ToLower(iss.Title), q) {
				score += 3
				match = "title"
			}
			if strings.Contains(strings.ToLower(iss.Description), q) {
				score += 2
				if match == "" {
					match = "description"
				}
			}
			for _, l := range iss.Labels {
				if strings.Contains(strings.ToLower(l), q) {
					score++
					if match == "" {
						match = "label"
					}
				}
			}
		}
		if score == 0 {
			continue
		}
		out = append(out, SearchResult{Issue: iss, Score: score, Match: match})
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}
	return out, nil
}
