package dagutil

// AllDepsResolved returns true when every ID in deps exists in doneSet.
// An empty deps slice is trivially resolved.
func AllDepsResolved[ID comparable](deps []ID, doneSet map[ID]bool) bool {
	for _, d := range deps {
		if !doneSet[d] {
			return false
		}
	}
	return true
}

// AllMatch returns true when pred holds for every element.
// An empty slice returns true (vacuous truth).
func AllMatch[T any](items []T, pred func(T) bool) bool {
	for i := range items {
		if !pred(items[i]) {
			return false
		}
	}
	return true
}

// AnyMatch returns true when pred holds for at least one element.
// An empty slice returns false.
func AnyMatch[T any](items []T, pred func(T) bool) bool {
	for i := range items {
		if pred(items[i]) {
			return true
		}
	}
	return false
}
