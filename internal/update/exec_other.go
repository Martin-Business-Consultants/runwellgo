//go:build !unix

package update

// Exec can't replace a process here: `runwell update` and a restart
// are the way.
func Exec(string) error { return ErrNoExec }
