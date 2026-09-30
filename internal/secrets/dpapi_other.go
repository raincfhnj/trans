//go:build !windows

package secrets

// Encrypt is where DPAPI does not exist. The stub exists so the tree builds
// and tests on every system the CI reaches; only Windows ever calls it.
func Encrypt(plaintext []byte, variable string, scope Scope) ([]byte, error) {
	return nil, ErrUnsupported
}

// Decrypt is the same refusal the other way round.
func Decrypt(ciphertext []byte, variable string) ([]byte, error) {
	return nil, ErrUnsupported
}
