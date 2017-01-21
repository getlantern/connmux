package connmux

import (
	"io"
	"net"
	"sync"
	"testing"

	"github.com/oxtoacart/bpool"
	"github.com/stretchr/testify/assert"
)

const (
	testdata = "Hello Dear World"
)

var (
	sessionBufferSource = bpool.NewBytePool(100, 70000)
	streamBufferSource  = bpool.NewBytePool(100, 1024768)
)

func TestConnNoMultiplex(t *testing.T) {
	doTestConnBasicFlow(t, func(network, addr string) func() (net.Conn, error) {
		return func() (net.Conn, error) {
			return net.Dial(network, addr)
		}
	})
}

func TestConnMultiplex(t *testing.T) {
	doTestConnBasicFlow(t, func(network, addr string) func() (net.Conn, error) {
		return Dialer(sessionBufferSource, streamBufferSource, func() (net.Conn, error) {
			return net.Dial(network, addr)
		})
	})
}

func doTestConnBasicFlow(t *testing.T, dialer func(network, addr string) func() (net.Conn, error)) {
	wrapped, err := net.Listen("tcp", "localhost:0")
	if !assert.NoError(t, err) {
		return
	}

	l := WrapListener(wrapped, sessionBufferSource, streamBufferSource)
	defer l.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, acceptErr := l.Accept()
		if !assert.NoError(t, acceptErr) {
			return
		}
		defer conn.Close()

		b := make([]byte, 4)
		for {
			n, readErr := conn.Read(b)
			if readErr != io.EOF && !assert.NoError(t, readErr) {
				return
			}
			n2, writeErr := conn.Write(b[:n])
			if !assert.NoError(t, writeErr) {
				return
			}
			if !assert.Equal(t, n, n2) {
				return
			}
			if readErr == io.EOF {
				return
			}
		}
	}()

	dial := dialer("tcp", l.Addr().String())
	conn, err := dial()
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
	wg.Wait()
}
