package connmux

import (
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testdata = "Hello Dear World"
)

func TestConnNoMultiplex(t *testing.T) {
	wrapped, err := net.Listen("tcp", "localhost:0")
	if !assert.NoError(t, err) {
		return
	}

	l := WrapListener(wrapped)
	defer l.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		conn, acceptErr := l.Accept()
		if !assert.NoError(t, acceptErr) {
			return
		}
		defer conn.Close()
		n, copyErr := io.Copy(conn, conn)
		if assert.NoError(t, copyErr) {
			assert.Equal(t, len(testdata), n)
		}
		wg.Done()
	}()

	conn, err := net.Dial("tcp", l.Addr().String())
	if !assert.NoError(t, err) {
		return
	}
	defer conn.Close()

	n, err := conn.Write([]byte(testdata))
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, len(testdata), n) {
		return
	}

	b := make([]byte, len(testdata))
	n, err = io.ReadFull(conn, b)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, len(testdata), n) {
		return
	}

	assert.Equal(t, testdata, string(b))
	conn.Close()
}
