package connmux

import (
	"fmt"
	"io"
	"net"
	"sync"
)

type session struct {
	net.Conn
	windowSize int
	pool       BufferPool
	out        chan []byte
	streams    map[uint32]*stream
	connCh     chan net.Conn
	mx         sync.RWMutex
}

func (s *session) readLoop() {
	for {
		b := s.pool.getForFrame()
		// First read id
		id := b[:idLen]
		_, err := io.ReadFull(s, id)
		if err != nil {
			if err == io.EOF {
				s.mx.RLock()
				streams := make([]*stream, 0, len(s.streams))
				for _, c := range s.streams {
					streams = append(streams, c)
				}
				s.mx.RUnlock()
				for _, c := range streams {
					c.Close()
				}
			} else {
				// TODO: propagate read error
			}
			s.Conn.Close()
			return
		}

		isACK := false
		isRST := false
		switch frameType(id) {
		case frameTypeACK:
			isACK = true
		case frameTypeRST:
			// Closing existing connection
			isRST = true
		}
		setFrameType(id, frameTypeData)

		_id := binaryEncoding.Uint32(id)
		if isACK {
			c := s.getOrCreateStream(_id)
			c.sb.ack <- true
			continue
		} else if isRST {
			s.mx.Lock()
			c := s.streams[_id]
			delete(s.streams, _id)
			s.mx.Unlock()
			// Close, but don't send an RST back the other way since the other end is
			// already closed.
			c.close(false)
			continue
		}

		// Read frame length
		dataLength := b[idLen:frameHeaderLen]
		_, err = io.ReadFull(s, dataLength)
		if err != nil {
			// TODO: DRY
			if err == io.EOF {
				for _, c := range s.streams {
					c.Close()
				}
			} else {
				// TODO: propagate read error
			}
			s.Conn.Close()
			return
		}

		_dataLength := int(binaryEncoding.Uint16(dataLength))

		// Read frame
		b = b[:frameHeaderLen+_dataLength]
		_, err = io.ReadFull(s, b[frameHeaderLen:])
		if err != nil {
			// TODO: DRY
			if err == io.EOF {
				for _, c := range s.streams {
					c.Close()
				}
			} else {
				// TODO: propagate read error
			}
			s.Conn.Close()
			return
		}

		c := s.getOrCreateStream(_id)
		c.rb.submit(b)
	}
}

func (s *session) writeLoop() {
	for frame := range s.out {
		dataLen := len(frame) - idLen
		if dataLen > MaxDataLen {
			panic(fmt.Sprintf("Data length of %d exceeds maximum allowed of %d", dataLen, MaxDataLen))
		}
		id := frame[dataLen:]
		_, err := s.Write(id)
		if err != nil {
			// TODO: handle error gracefully
			log.Error(err)
			return
		}
		if frameType(id) != frameTypeData {
			// This is a special control message, no data included
			continue
		}
		length := make([]byte, lenLen)
		binaryEncoding.PutUint16(length, uint16(dataLen))
		_, err = s.Write(length)
		if err != nil {
			// TODO: handle error gracefully
			log.Error(err)
			return
		}
		_, err = s.Write(frame[:dataLen])
		// Put frame back in pool
		s.pool.Put(frame[:maxFrameLen])
		if err != nil {
			// TODO: handle error gracefully
			log.Error(err)
			return
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
	_id := make([]byte, idLen)
	binaryEncoding.PutUint32(_id, id)
	c = &stream{
		Conn:    s,
		id:      _id,
		session: s,
		pool:    s.pool,
		rb:      newReceiveBuffer(_id, s.out, s.pool, s.windowSize),
		sb:      newSendBuffer(_id, s.out, s.windowSize),
	}
	s.streams[id] = c
	s.mx.Unlock()
	if s.connCh != nil {
		s.connCh <- c
	}
	return c
}
