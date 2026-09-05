package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const encoded_size = sha256.Size * 2

func Create(secret []byte, timestamp string, body []byte) string {
	digest := hmac.New(sha256.New, secret)
	_, _ = digest.Write([]byte(timestamp))
	_, _ = digest.Write([]byte("."))
	_, _ = digest.Write(body)
	return "v1=" + hex.EncodeToString(digest.Sum(nil))
}

func Valid(secret []byte, timestamp string, body []byte, supplied string) bool {
	const prefix = "v1="
	if len(supplied) != len(prefix)+encoded_size {
		return false
	}
	if supplied[:len(prefix)] != prefix {
		return false
	}
	supplied_digest, error_value := hex.DecodeString(supplied[len(prefix):])
	if error_value != nil {
		return false
	}

	expected := Create(secret, timestamp, body)
	expected_digest, error_value := hex.DecodeString(expected[len(prefix):])
	if error_value != nil {
		panic("signature encoding invariant failed")
	}
	return hmac.Equal(expected_digest, supplied_digest)
}
