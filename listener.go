package connmux

import (
	"io"
	"net"
)

type listener struct {
	wrapped net.Listener
	errCh   chan error
	connCh  chan net.Conn
}

func WrapListener(wrapped net.Listener) net.Listener {
	l := &listener{
		wrapped: wrapped,
		connCh:  make(chan net.Conn),
		errCh:   make(chan error),
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
		go l.newConn(conn)
	}
}

func (l *listener) newConn(conn net.Conn) {
	b := make([]byte, controlLength)
	// Try to read start sequence
	n, err := io.ReadFull(conn, b)
	if err != nil {
		l.errCh <- err
		return
	}
	if n == controlLength && string(b) == start {
		// It's a multiplexed connection
		s := &session{
			Conn:       conn,
			readBuffer: make([]byte, 32768), // todo, get this from pool
			vconns:     make(map[int]*vconn),
		}
		go s.read()
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

type session struct {
	net.Conn
	readBuffer []byte
	vconns     map[int]*vconn
}

func (s *session) read() {
	// b := s.readBuffer
	// for {
	//   // TODO: read deadline?
	//   n, err := s.Read(b)
	//
	// }
}
