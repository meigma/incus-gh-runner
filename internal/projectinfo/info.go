// Package projectinfo provides stable project metadata for user-facing adapters.
package projectinfo

const summary = "Incus-backed GitHub Actions runner controller"

// Summary returns the short project description.
func Summary() string {
	return summary
}
