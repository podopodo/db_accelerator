package mysql

import (
	"crypto/sha1" // #nosec G505 -- required by the MySQL native password wire protocol.
	"crypto/subtle"
)

const NativePasswordPlugin = "mysql_native_password"

// NativePasswordToken implements the MySQL 4.1 native password challenge.
// SHA-1 is mandated by this legacy protocol and is not used for stored hashes.
func NativePasswordToken(password string, seed []byte) []byte {
	if password == "" {
		return nil
	}
	stage1 := sha1.Sum([]byte(password)) // #nosec G401 -- protocol compatibility.
	stage2 := sha1.Sum(stage1[:])        // #nosec G401 -- protocol compatibility.
	challenge := make([]byte, 0, len(seed)+len(stage2))
	challenge = append(challenge, seed...)
	challenge = append(challenge, stage2[:]...)
	scramble := sha1.Sum(challenge) // #nosec G401 -- protocol compatibility.
	token := make([]byte, len(stage1))
	for index := range stage1 {
		token[index] = stage1[index] ^ scramble[index]
	}
	return token
}

func NativePasswordMatches(response []byte, password string, seed []byte) bool {
	return NativePasswordVerifierMatches(response, NativePasswordVerifier(password), seed)
}

// NativePasswordVerifier returns the double-SHA-1 verifier required by the
// legacy mysql_native_password protocol. It avoids retaining plaintext in the
// protocol service but is not a general-purpose password hash.
func NativePasswordVerifier(password string) []byte {
	if password == "" {
		return nil
	}
	stage1 := sha1.Sum([]byte(password)) // #nosec G401 -- protocol compatibility.
	stage2 := sha1.Sum(stage1[:])        // #nosec G401 -- protocol compatibility.
	return append([]byte(nil), stage2[:]...)
}

func NativePasswordVerifierMatches(response, verifier, seed []byte) bool {
	if len(verifier) == 0 {
		return len(response) == 0
	}
	if len(response) != sha1.Size || len(verifier) != sha1.Size {
		return false
	}
	challenge := make([]byte, 0, len(seed)+len(verifier))
	challenge = append(challenge, seed...)
	challenge = append(challenge, verifier...)
	scramble := sha1.Sum(challenge) // #nosec G401 -- protocol compatibility.
	candidateStage1 := make([]byte, sha1.Size)
	for index := range candidateStage1 {
		candidateStage1[index] = response[index] ^ scramble[index]
	}
	candidateVerifier := sha1.Sum(candidateStage1) // #nosec G401 -- protocol compatibility.
	return subtle.ConstantTimeCompare(candidateVerifier[:], verifier) == 1
}
