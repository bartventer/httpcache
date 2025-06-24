package fscache

import (
	"bytes"
	"crypto/aes"
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func mustBase64Key(t *testing.T, size int) string {
	t.Helper()
	key := make([]byte, size)
	_, err := rand.Read(key)
	testutil.RequireNoError(t, err, "failed to generate random key")
	return base64.URLEncoding.EncodeToString(key)
}

func Test_newAESGCMEncryptor(t *testing.T) {
	type args struct {
		r   io.Reader
		key string
	}
	tests := []struct {
		name      string
		args      args
		assertion func(*testing.T, *aesgcmEncryptor, error)
	}{
		{
			name: "ValidKey",
			args: args{
				r:   rand.Reader,
				key: mustBase64Key(t, 32),
			},
			assertion: func(t *testing.T, enc *aesgcmEncryptor, err error) {
				testutil.RequireNoError(t, err)
				testutil.AssertNotNil(t, enc)
				testutil.AssertNotNil(t, enc.gcm)
			},
		},
		{
			name: "InvalidBase64",
			args: args{
				r:   rand.Reader,
				key: "not-base64!",
			},
			assertion: func(t *testing.T, enc *aesgcmEncryptor, err error) {
				testutil.RequireErrorIs(t, err, base64.CorruptInputError(10))
				testutil.AssertNil(t, enc)
			},
		},
		{
			name: "InvalidKeySize",
			args: args{
				r:   rand.Reader,
				key: mustBase64Key(t, 10),
			},
			assertion: func(t *testing.T, enc *aesgcmEncryptor, err error) {
				testutil.RequireErrorIs(t, err, aes.KeySizeError(10))
				testutil.AssertNil(t, enc)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := newAESGCMEncryptor(tt.args.r, tt.args.key)
			tt.assertion(t, enc, err)
		})
	}
}

func TestAESGCMEncryptor_EncryptDecrypt(t *testing.T) {
	key := mustBase64Key(t, 32)
	enc, err := newAESGCMEncryptor(rand.Reader, key)
	testutil.RequireNoError(t, err)

	plaintext := []byte("hello world")
	ciphertext, err := enc.Encrypt(plaintext)
	testutil.RequireNoError(t, err)
	testutil.AssertTrue(t, !bytes.Contains(ciphertext, plaintext))

	decrypted, err := enc.Decrypt(ciphertext)
	testutil.RequireNoError(t, err)
	testutil.AssertTrue(t, bytes.Equal(decrypted, plaintext))
}

func TestAESGCMEncryptor_DecryptShortCiphertext(t *testing.T) {
	key := mustBase64Key(t, 16)
	enc, err := newAESGCMEncryptor(rand.Reader, key)
	testutil.RequireNoError(t, err)
	// Too short: less than nonce size
	short := []byte("short")
	_, err = enc.Decrypt(short)
	testutil.RequireErrorIs(t, err, errCiphertextTooShort)
}

func TestAESGCMEncryptor_DecryptTamperedCiphertext(t *testing.T) {
	key := mustBase64Key(t, 24)
	enc, err := newAESGCMEncryptor(rand.Reader, key)
	testutil.RequireNoError(t, err)
	plaintext := []byte("secret")
	ciphertext, err := enc.Encrypt(plaintext)
	testutil.RequireNoError(t, err)
	ciphertext[len(ciphertext)-1] ^= 0xFF
	_, err = enc.Decrypt(ciphertext)
	testutil.RequireError(t, err)
	testutil.AssertTrue(t, err.Error() == "cipher: message authentication failed")
}

func TestAESGCMEncryptor_Encrypt_ReaderError(t *testing.T) {
	// Patch rand.Reader to always fail (after key generation)
	key := mustBase64Key(t, 16)
	enc, err := newAESGCMEncryptor(errorReader{}, key)
	testutil.RequireNoError(t, err)
	_, err = enc.Encrypt([]byte("data"))
	testutil.RequireErrorIs(t, err, testutil.ErrSample)
}

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, testutil.ErrSample
}
