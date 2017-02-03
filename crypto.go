package connmux

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
	"math/big"
)

const (
	secretLen = 16
	ivLen     = 16
)

func (s *session) newAESSecret() ([]byte, error) {
	secret := make([]byte, secretLen)
	_, err := rand.Read(secret)
	if err != nil {
		return nil, fmt.Errorf("Unable to generate random AES secret: %v", err)
	}

	return secret, nil
}

func (s *session) newIV() ([]byte, error) {
	iv := make([]byte, ivLen)
	_, err := rand.Read(iv)
	if err != nil {
		return nil, fmt.Errorf("Unable to generate random initialization vector: %v", err)
	}

	return iv, nil
}

func (s *session) initAESCipher(secret []byte, iv []byte) error {
	block, err := aes.NewCipher(secret)
	if err != nil {
		return fmt.Errorf("Unable to generate client AES cipher: %v", err)
	}
	s.cipher = cipher.NewCTR(block, iv)

	return nil
}

func (s *session) buildInitCryptoMsg(secret []byte, iv []byte) ([]byte, error) {
	_p, err := rand.Int(rand.Reader, big.NewInt(32))
	if err != nil {
		return nil, fmt.Errorf("Unable to generate random padding length: %v", err)
	}
	p := int(_p.Int64())
	msg := make([]byte, 1+p)
	msg[0] = byte(p)
	_, err = rand.Read(msg[1 : 1+p])
	if err != nil {
		return nil, fmt.Errorf("Unable to generate random padding: %v", err)
	}

	_m, err := rand.Int(rand.Reader, big.NewInt(16))
	if err != nil {
		return nil, fmt.Errorf("Unable to generate random mac base length: %v", err)
	}
	m := byte(_m.Int64())
	data := make([]byte, 0, winLen+secretLen+ivLen+1)
	data = append(data, byte(s.windowSize))
	data = append(data, secret...)
	data = append(data, iv...)
	data = append(data, m)
	initMsg, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, s.serverPublicKey, data, nil)
	if err != nil {
		return nil, fmt.Errorf("Unable to encrypt init msg: %v", err)
	}

	msg = append(msg, initMsg...)
	return msg, nil
}
