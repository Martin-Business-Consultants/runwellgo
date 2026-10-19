package core

import "context"

// seed makes what a new install needs before anyone signs in: the settings
// row. People come from sign-up (the first owner) and invitations.
func (a *App) seed(ctx context.Context) error {
	_, err := a.settings(ctx)
	return err
}
