package connmux

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/codahale/blake2"
	"github.com/getlantern/keyman"
	"github.com/stretchr/testify/assert"
)

func TestInitCrypto(t *testing.T) {
	pk, err := keyman.GeneratePK(2048)
	if !assert.NoError(t, err) {
		return
	}

	s := &session{
		windowSize:      5,
		serverPublicKey: &pk.RSA().PublicKey,
	}

	secret, err := s.newAESSecret()
	if !assert.NoError(t, err) {
		return
	}

	iv, err := s.newIV()
	if !assert.NoError(t, err) {
		return
	}

	err = s.initAESCipher(secret, iv)
	if !assert.NoError(t, err) {
		return
	}

	msg, err := s.buildInitCryptoMsg(secret, iv)
	if !assert.NoError(t, err) {
		return
	}

	log.Debug(len(msg))
}

func BenchmarkHMACMD5(b *testing.B) {
	secret := make([]byte, 16)
	rand.Read(secret)
	data := make([]byte, 8192)
	rand.Read(data)
	mac := hmac.New(md5.New, secret)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac.Write(data)
		mac.Sum(nil)
		mac.Reset()
	}
}

func BenchmarkHMACSHA256(b *testing.B) {
	secret := make([]byte, 32)
	rand.Read(secret)
	data := make([]byte, 8192)
	rand.Read(data)
	mac := hmac.New(sha256.New, secret)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac.Write(data)
		mac.Sum(nil)
		mac.Reset()
	}
}

func BenchmarkHMACBlake2b512(b *testing.B) {
	secret := make([]byte, 64)
	rand.Read(secret)
	data := make([]byte, 8192)
	rand.Read(data)
	mac := hmac.New(blake2.NewBlake2B, secret)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac.Write(data)
		mac.Sum(nil)
		mac.Reset()
	}
}

func BenchmarkHMACBlake2b256(b *testing.B) {
	secret := make([]byte, 32)
	rand.Read(secret)
	data := make([]byte, 8192)
	rand.Read(data)
	mac := blake2.New(&blake2.Config{
		Size: 32,
		Key:  secret,
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac.Write(data)
		mac.Sum(nil)
		mac.Reset()
	}
}
