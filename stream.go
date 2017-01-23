package connmux

import (
	"net"
	"sync"
	"time"
)

type stream struct {
	net.Conn
	id            []byte
	session       *session
	pool          BufferPool
	rb            *receiveBuffer
	sb            *sendBuffer
	readDeadline  time.Time
	writeDeadline time.Time
	closed        bool
	mx            sync.RWMutex
}

func (c *stream) Read(b []byte) (int, error) {
	c.mx.RLock()
	closed := c.closed
	readDeadline := c.readDeadline
	c.mx.RUnlock()
	if closed {
		return 0, ErrConnectionClosed
	}
	return c.rb.read(b, readDeadline)
}

func (c *stream) Write(b []byte) (int, error) {
	// TODO: make sure this splitting works
	if len(b) > MaxDataLen {
		return c.writeChunks(b)
	}

	c.mx.RLock()
	closed := c.closed
	writeDeadline := c.writeDeadline
	c.mx.RUnlock()
	if closed {
		// Make it look like the write worked even though we're not going to send it
		// anywhere (TODO, might be better way to handle this?)
		return len(b), nil
	}
	// copy buffer since we hang on to it past the call to Write but callers
	// expect that they can reuse the buffer after Write returns
	_b := b
	b = c.pool.getForFrame()[:len(b)]
	copy(b, _b)
	if writeDeadline.IsZero() {
		c.sb.in <- b
		return len(b), nil
	}
	now := time.Now()
	if writeDeadline.Before(now) {
		return 0, ErrTimeout
	}
	timer := time.NewTimer(writeDeadline.Sub(now))
	select {
	case c.sb.in <- b:
		timer.Stop()
		return len(b), nil
	case <-timer.C:
		timer.Stop()
		return 0, ErrTimeout
	}
}

// writeChunks breaks the buffer down into units smaller than MaxDataLen in size
func (c *stream) writeChunks(b []byte) (int, error) {
	totalN := 0
	for {
		toWrite := b
		last := true
		if len(b) > MaxDataLen {
			toWrite = b[:MaxDataLen]
			b = b[MaxDataLen:]
			last = false
		}
		n, err := c.Write(toWrite)
		totalN += n
		if last || err != nil {
			return totalN, err
		}
	}
}

func (c *stream) Close() error {
	return c.close(true)
}

func (c *stream) close(sendRST bool) error {
	didClose := false
	c.mx.Lock()
	if !c.closed {
		c.closed = true
		didClose = true
	}
	c.mx.Unlock()
	if didClose {
		c.rb.close()
		c.sb.close(sendRST)
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
	c.mx.Lock()
	c.readDeadline = t
	c.writeDeadline = t
	c.mx.Unlock()
	return nil
}

func (c *stream) SetReadDeadline(t time.Time) error {
	c.mx.Lock()
	c.readDeadline = t
	c.mx.Unlock()
	return nil
}

func (c *stream) SetWriteDeadline(t time.Time) error {
	c.mx.Lock()
	c.writeDeadline = t
	c.mx.Unlock()
	return nil
}
