package mcp

import "time"

// sinceString renders an elapsed duration the way the old status endpoints did.
func sinceString(t time.Time) string {
	return time.Since(t).Round(time.Second).String()
}
