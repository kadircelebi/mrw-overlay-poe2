package session

// ProtectSecret seals a small secret for this Windows user (DPAPI), the same
// way the pathofexile.com session is kept.
func ProtectSecret(plain []byte) ([]byte, error) { return protect(plain) }

// UnprotectSecret opens what ProtectSecret sealed.
func UnprotectSecret(sealed []byte) ([]byte, error) { return unprotect(sealed) }
