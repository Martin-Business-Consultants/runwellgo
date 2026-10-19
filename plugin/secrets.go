package plugin

import "errors"

// A plugin granted "secrets" keeps a value it must use again (an API key
// for a service) sealed with the install's key, as Runwell keeps its own
// settings' secrets: Seal before storing it, Unseal to use it, and never
// send it back to the app. Unseal also opens a value the Rails install
// encrypted, when Runwell runs with that install's keys.

// Seal is plain sealed with the install's key (secrets).
func (a *API) Seal(plain string) (string, error) {
	if err := a.need("secrets", a.grants.Secrets); err != nil {
		return "", err
	}
	if plain == "" {
		return "", nil
	}
	return a.h.env.Seal(plain), nil
}

// Unseal is a sealed value's plaintext (secrets); an error when it can't be
// read (sealed by another install's key).
func (a *API) Unseal(sealed string) (string, error) {
	if err := a.need("secrets", a.grants.Secrets); err != nil {
		return "", err
	}
	if sealed == "" {
		return "", nil
	}
	plain, ok := a.h.env.Unseal(sealed)
	if !ok {
		return "", errors.New("the value was sealed by another install's key and can't be read here")
	}
	return plain, nil
}
