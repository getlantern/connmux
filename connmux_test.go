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
	doTestConnBasicFlow(t, func(network, addr string) func() (net.Conn, error) {
		return func() (net.Conn, error) {
			return net.Dial(network, addr)
		}
	})
}

func TestConnMultiplex(t *testing.T) {
	doTestConnBasicFlow(t, func(network, addr string) func() (net.Conn, error) {
		return Dialer(10, 100, func() (net.Conn, error) {
			return net.Dial(network, addr)
		})
	})
}

func doTestConnBasicFlow(t *testing.T, dialer func(network, addr string) func() (net.Conn, error)) {
	wrapped, err := net.Listen("tcp", "localhost:0")
	if !assert.NoError(t, err) {
		return
	}

	l := WrapListener(wrapped, 10, 100)
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

		for {
			b := make([]byte, 4, maxFrameLen)
			n, readErr := conn.Read(b)
			if readErr != io.EOF && !assert.NoError(t, readErr, "Error reading for echo") {
				return
			}
			n2, writeErr := conn.Write(b[:n])
			if !assert.NoError(t, writeErr, "Error writing echo") {
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

func BenchmarkConnMux(b *testing.B) {
	_lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}

	lst := WrapListener(_lst, 10, 100)

	conn, err := Dialer(10, 100, func() (net.Conn, error) {
		return net.Dial("tcp", lst.Addr().String())
	})()
	if err != nil {
		b.Fatal(err)
	}

	doBench(b, lst, conn)
}

func BenchmarkTCP(b *testing.B) {
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}

	conn, err := net.Dial("tcp", lst.Addr().String())
	if err != nil {
		b.Fatal(err)
	}

	doBench(b, lst, conn)
}

func doBench(b *testing.B, l net.Listener, wr io.Writer) {
	size := maxDataLen
	buf := make([]byte, size)
	buf2 := make([]byte, size)
	b.SetBytes(int64(size))
	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := l.Accept()
		if err != nil {
			b.Fatal(err)
		}
		count := 0
		for {
			n, err := conn.Read(buf2)
			if err != nil {
				b.Fatal(err)
			}
			count += n
			if count == size*b.N {
				return
			}
		}
	}()
	for i := 0; i < b.N; i++ {
		_, err := wr.Write(buf)
		if err != nil {
			b.Fatal(err)
		}
	}
	wg.Wait()
}
