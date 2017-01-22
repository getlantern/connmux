package connmux

import (
	"time"
)

var (
	defaultDeadline = time.Now().Add(100000 * time.Hour)
)

type receivebuffer struct {
	in       chan []byte
	ack      chan bool
	pool     BufferPool
	poolable []byte
	current  []byte
}

func newReceiveBuffer(ack chan bool, pool BufferPool, depth int) *receivebuffer {
	return &receivebuffer{
		in:   make(chan []byte, depth),
		ack:  ack,
		pool: pool,
	}
}

func (buf *receivebuffer) read(b []byte, deadline time.Time) (totalN int, err error) {
	for {
		n := copy(b, buf.current)
		buf.current = buf.current[n:]
		totalN += n
		if n == len(b) {
			// nothing more to copy
			return
		}
		b = b[n:]
		// b can hold more than we had in the current slice, try to read more if
		// immediately available.
		select {
		case frame := <-buf.in:
			// Read next frame, continue loop
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
				deadline = defaultDeadline
			} else if deadline.Before(now) {
				// Deadline already past, don't bother doing anything
				return
			}
			timer := time.NewTimer(now.Sub(deadline))
			select {
			case <-timer.C:
				// Nothing read within deadline
				err = ErrTimeout
				timer.Stop()
				return
			case frame := <-buf.in:
				// Read next frame, continue loop
				buf.onFrame(frame)
				timer.Stop()
				continue
			}
		}
	}
}

func (buf *receivebuffer) onFrame(frame []byte) {
	if buf.poolable != nil {
		// Return previous frame to pool
		buf.pool.Put(buf.poolable[:cap(buf.poolable)])
	}
	buf.poolable = frame
	buf.current = frame[frameHeaderLen:]
	buf.ack <- true
}
