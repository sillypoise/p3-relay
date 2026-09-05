package signature

import "testing"

func TestValidAcceptsCreatedSignature(t *testing.T) {
	secret := []byte("test-secret-with-enough-entropy")
	body := []byte(`{"event":"created"}`)
	timestamp := "1757023200"

	supplied := Create(secret, timestamp, body)

	if !Valid(secret, timestamp, body, supplied) {
		t.Fatal("created signature was rejected")
	}
}

func TestValidRejectsInvalidInputs(t *testing.T) {
	secret := []byte("test-secret-with-enough-entropy")
	body := []byte(`{"event":"created"}`)
	timestamp := "1757023200"
	valid := Create(secret, timestamp, body)

	test_cases := map[string]string{
		"empty":        "",
		"wrong tag":    "v2=" + valid[3:],
		"short":        "v1=00",
		"invalid hex":  "v1=zz" + valid[5:],
		"wrong digest": "v1=" + valid[3:len(valid)-2] + "00",
	}
	for name, supplied := range test_cases {
		t.Run(name, func(t *testing.T) {
			if Valid(secret, timestamp, body, supplied) {
				t.Fatal("invalid signature was accepted")
			}
		})
	}
}
