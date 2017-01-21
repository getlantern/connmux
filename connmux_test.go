package connmux

import (
	"fmt"
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
	sessionBufferSource = bpool.NewBytePool(100, 32768)
	readBufferSource    = bpool.NewBytePool(100, 1024768)
)

func TestConnNoMultiplex(t *testing.T) {
	doTestConn(t, func(network, addr string) func() (net.Conn, error) {
		return func() (net.Conn, error) {
			return net.Dial(network, addr)
		}
	})
}

func TestConnMultiplex(t *testing.T) {
	doTestConn(t, func(network, addr string) func() (net.Conn, error) {
		return Dialer(readBufferSource, func() (net.Conn, error) {
			return net.Dial(network, addr)
		})
	})
}

func doTestConn(t *testing.T, dialer func(network, addr string) func() (net.Conn, error)) {
	wrapped, err := net.Listen("tcp", "localhost:0")
	if !assert.NoError(t, err) {
		return
	}

	l := WrapListener(wrapped, sessionBufferSource, readBufferSource)
	defer l.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		conn, acceptErr := l.Accept()
		if !assert.NoError(t, acceptErr) {
			return
		}
		fmt.Println("Accepted conn")
		defer conn.Close()
		_, copyErr := io.Copy(conn, conn)
		if copyErr != nil {
			fmt.Println(copyErr)
			return
		}
		fmt.Println("Done copying")
		wg.Done()
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

	fmt.Println("Got here")
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
