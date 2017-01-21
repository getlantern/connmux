package connmux

import (
	"io"
	"net"
	"sync"

	"github.com/getlantern/framed"
)

type session struct {
	net.Conn
	framed              *framed.Reader
	sessionBufferSource BufferSource
	streamBufferSource  BufferSource
	sessionBuffer       []byte
	connCh              chan net.Conn
	streams             map[uint32]*stream
	mx                  sync.Mutex
}

func (s *session) readLoop() {
	b := s.sessionBuffer
	defer s.sessionBufferSource.Put(b)

	for {
		n, err := s.framed.Read(b)
		if err != nil {
			if err == io.EOF {
				for _, c := range s.streams {
					c.readBuffer.Close()
				}
			} else {
				// TODO: propagate read error
			}
			s.Conn.Close()
			return
		}
		id := b[:idlen]
		isClose := false
		if id[0] == connClose {
			// Closing existing connection
			isClose = true
			id[0] = 0
		}

		_id := binaryEncoding.Uint32(id)
		if isClose {
			s.mx.Lock()
			c := s.streams[_id]
			delete(s.streams, _id)
			s.mx.Unlock()
			c.readBuffer.Close()
			continue
		}

		c := s.getOrCreateStream(_id)

		if n > idlen {
			bufferErr := c.readBuffer.Write(b[idlen:n])
			if bufferErr != nil {
				c.markOverflowed()
				delete(s.streams, _id)
			}
		}
	}
}

func (s *session) getOrCreateStream(id uint32) *stream {
	s.mx.Lock()
	c := s.streams[id]
	if c != nil {
		s.mx.Unlock()
		return c
	}
	_id := make([]byte, idlen)
	binaryEncoding.PutUint32(_id, id)
	c = &stream{
		Conn:         s.Conn,
		framed:       framed.NewWriter(s.Conn),
		id:           _id,
		bufferSource: s.streamBufferSource,
		readBuffer:   newBoundedBuffer(s.streamBufferSource.Get()),
	}
	s.streams[id] = c
	s.mx.Unlock()
	if s.connCh != nil {
		s.connCh <- c
	}
	return c
}
