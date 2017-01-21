package connmux

import (
	"fmt"
	"io"
	"net"
)

type listener struct {
	wrapped             net.Listener
	sessionBufferSource BufferSource
	readBufferSource    BufferSource
	errCh               chan error
	connCh              chan net.Conn
}

func WrapListener(wrapped net.Listener, sessionBufferSource BufferSource, readBufferSource BufferSource) net.Listener {
	l := &listener{
		wrapped:             wrapped,
		sessionBufferSource: sessionBufferSource,
		readBufferSource:    readBufferSource,
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
		go l.newConn(conn)
	}
}

func (l *listener) newConn(conn net.Conn) {
	b := make([]byte, sessionStartLength)
	// Try to read start sequence
	n, err := io.ReadFull(conn, b)
	if err != nil {
		l.errCh <- err
		return
	}
	fmt.Printf("Session start? %v\n", string(b))
	if n == sessionStartLength && string(b) == sessionStart {
		fmt.Println("It's multiplexed")
		// It's a multiplexed connection
		s := &session{
			Conn:                conn,
			sessionBufferSource: l.sessionBufferSource,
			readBufferSource:    l.readBufferSource,
			sessionBuffer:       l.sessionBufferSource.Get(),
			vconns:              make(map[uint64]*vconn),
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

type session struct {
	net.Conn
	sessionBufferSource BufferSource
	readBufferSource    BufferSource
	sessionBuffer       []byte
	vconns              map[uint64]*vconn
}

func (s *session) readLoop() {
	b := s.sessionBuffer
	defer s.sessionBufferSource.Put(b)

	for {
		fmt.Println("Read looping")
		// TODO: read deadline?
		n, err := io.ReadAtLeast(s, b, 8)
		if err != nil {
			if err == io.EOF {
				for _, vc := range s.vconns {
					vc.readBuffer.Close()
				}
			} else {
				// TODO: propagate read error
			}
			s.Conn.Close()
			return
		}
		fmt.Printf("Read %v\n", string(b[:n]))
		id := b[:8]
		isClose := false
		if id[0] == connClose {
			// Closing existing connection
			isClose = true
			id[0] = 0
		}

		_id := binaryEncoding.Uint64(id)
		vc := s.vconns[_id]
		if isClose {
			s.vconns[_id].readBuffer.Close()
			delete(s.vconns, _id)
			continue
		}

		if vc == nil {
			vc = &vconn{
				Conn:         s.Conn,
				id:           id,
				bufferSource: s.readBufferSource,
				readBuffer:   newBoundedBuffer(s.readBufferSource.Get()),
			}
			s.vconns[_id] = vc
		}

		bufferErr := vc.readBuffer.Write(b[2:n])
		if bufferErr != nil {
			vc.markOverflowed()
			delete(s.vconns, _id)
		}
		fmt.Println("Wrote to read buffer")
	}
}
