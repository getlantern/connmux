package connmux

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

var (
	ErrBufferFull = errors.New("buffer full")
	ErrTimeout    = &timeoutError{}

	binaryEncoding = binary.BigEndian
)

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

type Buffer interface {
	Write(b []byte) error

	Read(b []byte, deadline time.Time) (int, error)
}

type vconn struct {
	net.Conn
	id            []byte
	readBuf       Buffer
	readDeadline  time.Time
	writeDeadline time.Time
}

func (c *vconn) Read(b []byte) (int, error) {
	// return c.readBuf.Read(b, c.readDeadline)
	return 0, nil
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
