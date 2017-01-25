package connmux

import (
	"io"
	"sync"
	"time"
)

type receiveBuffer struct {
	ackFrame []byte
	in       chan []byte
	ack      chan []byte
	pool     BufferPool
	poolable []byte
	current  []byte
	closed   bool
	mx       sync.RWMutex
}

func newReceiveBuffer(streamID []byte, ack chan []byte, pool BufferPool, depth int) *receiveBuffer {
	ackFrame := make([]byte, len(streamID))
	copy(ackFrame, streamID)
	setFrameType(ackFrame, frameTypeACK)
	return &receiveBuffer{
		ackFrame: ackFrame,
		in:       make(chan []byte, depth),
		ack:      ack,
		pool:     pool,
	}
}

func (buf *receiveBuffer) submit(b []byte) {
	buf.mx.RLock()
	closed := buf.closed
	in := buf.in
	buf.mx.RUnlock()
	if closed {
		return
	}
	in <- b
}

func (buf *receiveBuffer) read(b []byte, deadline time.Time) (totalN int, err error) {
	buf.mx.RLock()
	in := buf.in
	buf.mx.RUnlock()

	for {
		n := copy(b, buf.current)
		buf.current = buf.current[n:]
		totalN += n
		if n == len(b) {
			// nothing more to copy
			return
		}
		if err != nil {
			return
		}

		// b can hold more than we had in the current slice, try to read more if
		// immediately available.
		b = b[n:]
		select {
		case frame, open := <-in:
			// Read next frame, continue loop
			if !open && frame == nil {
				// we've hit the end
				err = io.EOF
				return
			}
			buf.onFrame(frame)
			continue
		default:
			// nothing immediately available
			if totalN > 0 {
				// we're read something, return what we have
				return
			}

			// We haven't ready anything, wait up till deadline to read
			now := time.Now()
			if deadline.IsZero() {
				deadline = largeDeadline
			} else if deadline.Before(now) {
				// Deadline already past, don't bother doing anything
				return
			}
			timer := time.NewTimer(deadline.Sub(now))
			select {
			case <-timer.C:
				// Nothing read within deadline
				err = ErrTimeout
				timer.Stop()
				return
			case frame, open := <-in:
				// Read next frame, continue loop
				timer.Stop()
				if !open && frame == nil {
					// we've hit the end
					err = io.EOF
					return
				}
				buf.onFrame(frame)
				continue
			}
		}
	}
}

func (buf *receiveBuffer) onFrame(frame []byte) {
	if buf.poolable != nil {
		// Return previous frame to pool
		buf.pool.Put(buf.poolable[:maxFrameLen])
	}
	buf.poolable = frame
	buf.current = frame[frameHeaderLen:]
	buf.ack <- buf.ackFrame
}

func (buf *receiveBuffer) close() {
	buf.mx.Lock()
	if !buf.closed {
		buf.closed = true
		close(buf.in)
	}
	buf.mx.Unlock()
}
