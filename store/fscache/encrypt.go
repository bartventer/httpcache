package fscache

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"io"
)

var errCiphertextTooShort = errors.New("ciphertext too short")

type encryptor interface {
	Encrypt(data []byte) ([]byte, error)
	Decrypt(data []byte) ([]byte, error)
}

// aesgcmEncryptor implements the encryptor interface using AES-GCM.
type aesgcmEncryptor struct {
	gcm cipher.AEAD
	r   io.Reader
}

// newAESGCMEncryptor creates a new aesgcmEncryptor instance.
//
// The keyB64 parameter should be a base64-encoded string (RFC 4648 §5, url-safe variant)
// representing the AES key to be used for encryption and decryption.
func newAESGCMEncryptor(r io.Reader, keyB64 string) (*aesgcmEncryptor, error) {
	key, err := base64.URLEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &aesgcmEncryptor{gcm: gcm, r: r}, nil
}

func (e *aesgcmEncryptor) Encrypt(data []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(e.r, nonce); err != nil {
		return nil, err
	}
	ciphertext := e.gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

func (e *aesgcmEncryptor) Decrypt(data []byte) ([]byte, error) {
	if len(data) < e.gcm.NonceSize() {
		return nil, errCiphertextTooShort
	}
	nonce, ciphertext := data[:e.gcm.NonceSize()], data[e.gcm.NonceSize():]
	return e.gcm.Open(ciphertext[:0], nonce, ciphertext, nil)
}
