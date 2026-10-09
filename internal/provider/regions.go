package provider

import "sort"

// regionAliasGroups lists names that mean the same monitoring region across
// every generation of keys: location names (us-east, adopted October 2026),
// the airport codes before them (ash) and the first generation (nyc). The
// API accepts any of them and returns its current key, so a configuration
// written with an older name must not show a diff when the API answers with
// a newer one. Keep in step with config('monitoring.region_aliases').
var regionAliasGroups = [][]string{
	{"us-east", "ash", "nyc"},
	{"us-west", "pdx", "sfo"},
	{"eu-central", "nbg", "fra"},
	{"ap-southeast", "sin"},
}

// canonicalRegion maps any name of a region to the first name of its group;
// unknown names pass through unchanged.
func canonicalRegion(name string) string {
	for _, group := range regionAliasGroups {
		for _, member := range group {
			if member == name {
				return group[0]
			}
		}
	}

	return name
}

// sameRegions reports whether two region lists name the same set of places,
// whatever generation of names each uses.
func sameRegions(a, b []string) bool {
	normalize := func(list []string) []string {
		seen := map[string]bool{}
		out := []string{}
		for _, name := range list {
			c := canonicalRegion(name)
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
		sort.Strings(out)
		return out
	}

	x, y := normalize(a), normalize(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}

	return true
}
