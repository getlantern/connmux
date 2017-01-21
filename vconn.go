package connmux

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type vconn struct {
	net.Conn
	id            []byte
	readBuf       Buffer
	readDeadline  time.Time
	writeDeadline time.Time
	overflowed    int64
}

func (c *vconn) Read(b []byte) (int, error) {
	if atomic.LoadInt64(&c.overflowed) == 1 {
		return 0, ErrBufferOverflowed
	}
	return c.readBuf.Read(b, c.readDeadline)
}

func (c *vconn) Write(b []byte) (int, error) {
	err := c.Conn.SetWriteDeadline(c.writeDeadline)
	if err != nil {
		return 0, fmt.Errorf("Unable to set write deadline: %v", err)
	}
	_, err = c.Conn.Write(c.id)
	if err != nil {
		return 0, err
	}
	return c.Conn.Write(b)
}

func (c *vconn) Close() error {
	return nil
}

func (c *vconn) LocalAddr() net.Addr {
	return c.Conn.LocalAddr()
}

func (c *vconn) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

func (c *vconn) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	c.SetWriteDeadline(t)
	return nil
}

func (c *vconn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

func (c *vconn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

func (c *vconn) markOverflowed() {
	atomic.StoreInt64(&c.overflowed, 1)
}
