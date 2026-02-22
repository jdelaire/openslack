package keychain

import "github.com/zalando/go-keyring"

const serviceName = "openslack"

// Get retrieves a secret from the system keychain.
func Get(account string) (string, error) {
	return keyring.Get(serviceName, account)
}

// Set stores a secret in the system keychain.
func Set(account, value string) error {
	return keyring.Set(serviceName, account, value)
}
