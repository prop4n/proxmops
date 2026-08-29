package auth

import "testing"

func TestHashVerifyRoundtrip(t *testing.T) {
	hash, err := HashPassword("s3cr3t-pass")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword(hash, "s3cr3t-pass")
	if err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	hash, _ := HashPassword("right")
	ok, err := VerifyPassword(hash, "wrong")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("wrong password verified as correct")
	}
}

func TestVerifyInvalidHash(t *testing.T) {
	if _, err := VerifyPassword("not-a-hash", "x"); err == nil {
		t.Fatal("want error for malformed hash")
	}
}

func TestHashIsSalted(t *testing.T) {
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	if h1 == h2 {
		t.Fatal("hashes of the same password should differ (random salt)")
	}
}
