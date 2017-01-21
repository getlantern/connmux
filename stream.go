package connmux

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/getlantern/framed"
)

type stream struct {
	net.Conn
	framed        *framed.Writer
	id            []byte
	bufferSource  BufferSource
	readBuffer    Buffer
	readDeadline  time.Time
	writeDeadline time.Time
	overflowed    int64
	closed        int64
}

func (c *stream) Read(b []byte) (int, error) {
	if atomic.LoadInt64(&c.overflowed) == 1 {
		return 0, ErrBufferOverflowed
	}
	if atomic.LoadInt64(&c.closed) == 1 {
		return 0, ErrConnectionClosed
	}
	return c.readBuffer.Read(b, c.readDeadline)
}

func (c *stream) Write(b []byte) (int, error) {
	err := c.Conn.SetWriteDeadline(c.writeDeadline)
	if err != nil {
		return 0, fmt.Errorf("Unable to set write deadline: %v", err)
	}
	n, err := c.framed.WritePieces(c.id, b)
	return n - idlen, err
}

func (c *stream) Close() error {
	if atomic.CompareAndSwapInt64(&c.closed, 0, 1) {
		c.bufferSource.Put(c.readBuffer.Raw())
		c.id[0] = connClose
		_, err := c.framed.Write(c.id)
		return err
	}
	return nil
}

func (c *stream) LocalAddr() net.Addr {
	return c.Conn.LocalAddr()
}

func (c *stream) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

func (c *stream) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	c.SetWriteDeadline(t)
	return nil
}

func (c *stream) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

func (c *stream) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

func (c *stream) markOverflowed() {
	atomic.StoreInt64(&c.overflowed, 1)
}
