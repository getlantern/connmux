package connmux

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
)

func (s *session) initAESCipher() error {
	s.clientKeyBytes = make([]byte, 16)
	_, err := rand.Read(s.clientKeyBytes)
	if err != nil {
		return fmt.Errorf("Unable to generate random client key bytes: %v", err)
	}

	s.clientCipher, err = aes.NewCipher(s.clientKeyBytes)
	if err != nil {
		return fmt.Errorf("Unable to generate client AES cipher: %v", err)
	}

	return nil
}

func (s *session) encryptedKey() ([]byte, error) {
	return rsa.EncryptOAEP(sha256.New(), rand.Reader, s.serverPublicKey, s.clientKeyBytes, nil)
}
