package connmux

import (
	"io"
	"net"

	"github.com/getlantern/framed"
)

type listener struct {
	wrapped             net.Listener
	sessionBufferSource BufferSource
	streamBufferSource  BufferSource
	errCh               chan error
	connCh              chan net.Conn
}

func WrapListener(wrapped net.Listener, sessionBufferSource BufferSource, streamBufferSource BufferSource) net.Listener {
	l := &listener{
		wrapped:             wrapped,
		sessionBufferSource: sessionBufferSource,
		streamBufferSource:  streamBufferSource,
		connCh:              make(chan net.Conn),
		errCh:               make(chan error),
	}
	go l.process()
	return l
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case err := <-l.errCh:
		return nil, err
	}
}

func (l *listener) Addr() net.Addr {
	return l.wrapped.Addr()
}

func (l *listener) Close() error {
	go func() {
		l.errCh <- ErrListenerClosed
	}()
	return l.wrapped.Close()
}

func (l *listener) process() {
	for {
		conn, err := l.wrapped.Accept()
		if err != nil {
			l.errCh <- err
			return
		}
		go l.onConn(conn)
	}
}

func (l *listener) onConn(conn net.Conn) {
	b := make([]byte, sessionStartLength)
	// Try to read start sequence
	n, err := io.ReadFull(conn, b)
	if err != nil {
		l.errCh <- err
		return
	}
	if n == sessionStartLength && string(b) == sessionStart {
		// It's a multiplexed connection
		s := &session{
			Conn:                conn,
			framed:              framed.NewReader(conn),
			sessionBufferSource: l.sessionBufferSource,
			streamBufferSource:  l.streamBufferSource,
			sessionBuffer:       l.sessionBufferSource.Get(),
			connCh:              l.connCh,
			streams:             make(map[uint32]*stream),
		}
		go s.readLoop()
		return
	}

	// It's a normal connection
	l.connCh <- &preReadConn{conn, b}
}

// preReadConn is a conn that takes care of the fact that we've already read a
// little from it
type preReadConn struct {
	net.Conn
	buf []byte
}

func (prc *preReadConn) Read(b []byte) (int, error) {
	buffered := len(prc.buf)
	if buffered == 0 {
		return prc.Conn.Read(b)
	}
	needed := len(b)
	n := copy(b, prc.buf)
	var err error
	prc.buf = prc.buf[n:]
	remain := needed - n
	if remain > 0 {
		var n2 int
		n2, err = prc.Conn.Read(b[n:])
		n += n2
	}
	return n, err
}
