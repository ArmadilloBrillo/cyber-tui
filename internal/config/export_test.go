package config

// SetRename replaces the rename used by Migrate until the test ends.
func SetRename(t interface{ Cleanup(func()) }, f func(from, to string) error) {
	old := rename
	rename = f
	t.Cleanup(func() { rename = old })
}
