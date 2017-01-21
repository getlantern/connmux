package connmux

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func Dialer(bufferSource BufferSource, dial func() (net.Conn, error)) func() (net.Conn, error) {
	d := &dialer{doDial: dial, bufferSource: bufferSource}
	return d.dial
}

type dialer struct {
	doDial       func() (net.Conn, error)
	bufferSource BufferSource
	current      net.Conn
	id           uint64
	mx           sync.Mutex
}

func (d *dialer) dial() (net.Conn, error) {
	d.mx.Lock()
	current := d.current
	if current == nil {
		conn, err := d.doDial()
		if err != nil {
			d.mx.Unlock()
			return nil, err
		}
		_, writeErr := conn.Write(sessionStartBytes)
		if writeErr != nil {
			conn.Close()
			return nil, writeErr
		}
		current = conn
		d.current = conn
	}
	_id := d.id
	d.id += 1
	d.mx.Unlock()

	id := make([]byte, 8)
	binaryEncoding.PutUint64(id, _id)
	return &vconn{
		Conn:         current,
		id:           id,
		bufferSource: d.bufferSource,
		readBuffer:   newBoundedBuffer(d.bufferSource.Get()),
	}, nil
}

type vconn struct {
	net.Conn
	id            []byte
	bufferSource  BufferSource
	readBuffer    Buffer
	readDeadline  time.Time
	writeDeadline time.Time
	overflowed    int64
	closed        int64
}

func (c *vconn) Read(b []byte) (int, error) {
	if atomic.LoadInt64(&c.overflowed) == 1 {
		return 0, ErrBufferOverflowed
	}
	if atomic.LoadInt64(&c.closed) == 1 {
		return 0, ErrConnectionClosed
	}
	fmt.Println("Reading from read buffer")
	return c.readBuffer.Read(b, c.readDeadline)
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
	if atomic.CompareAndSwapInt64(&c.closed, 0, 1) {
		c.bufferSource.Put(c.readBuffer.Raw())
	}
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
