package connmux

import (
	"sync"
	"time"
)

var (
	defaultDeadline = time.Now().Add(100000 * time.Hour)
)

type boundedBuffer struct {
	limit       int
	readOffset  int
	writeOffset int
	unread      int
	data        []byte
	waitForData chan bool
	mx          sync.Mutex
}

func newBoundedBuffer(data []byte) Buffer {
	limit := cap(data)
	data = data[:limit]
	return &boundedBuffer{
		limit: limit,
		data:  data,
	}
}

func (bb *boundedBuffer) Write(b []byte) error {
	n := len(b)
	bb.mx.Lock()
	unread := bb.unread + n
	if unread > bb.limit {
		bb.mx.Unlock()
		return ErrBufferFull
	}
	bb.unread = unread
	available := bb.limit - bb.writeOffset
	if available == 0 {
		copy(bb.data, b)
		bb.writeOffset = n
	} else if available < n {
		copy(bb.data[bb.writeOffset:], b[:available])
		copy(bb.data, b[available:])
		bb.writeOffset = n - available
	} else {
		copy(bb.data[bb.writeOffset:], b)
		bb.writeOffset += n
	}
	waitForData := bb.waitForData
	bb.mx.Unlock()
	if waitForData != nil {
		select {
		case waitForData <- true:
			// ok
		default:
			// already notified
		}
	}
	return nil
}

func (bb *boundedBuffer) Read(b []byte, deadline time.Time) (int, error) {
	bb.mx.Lock()
	if bb.unread > 0 {
		n, err := bb.doRead(b)
		bb.mx.Unlock()
		return n, err
	}

	now := time.Now()
	if deadline.Before(now) {
		// Don't bother waiting
		bb.mx.Unlock()
		return 0, ErrTimeout
	}

	// wait for data
	waitForData := make(chan bool)
	bb.waitForData = waitForData
	bb.mx.Unlock()

	if deadline.IsZero() {
		// Wait indefinitely
		<-waitForData
		bb.mx.Lock()
		n, err := bb.doRead(b)
		bb.mx.Unlock()
		return n, err
	}

	// Wait with timeout
	timer := time.NewTimer(deadline.Sub(now))
	select {
	case <-waitForData:
		timer.Stop()
		bb.mx.Lock()
		n, err := bb.doRead(b)
		bb.waitForData = nil
		bb.mx.Unlock()
		return n, err
	case <-timer.C:
		timer.Stop()
		bb.mx.Lock()
		bb.waitForData = nil
		bb.mx.Unlock()
		return 0, ErrTimeout
	}
}

func (bb *boundedBuffer) doRead(b []byte) (int, error) {
	n := len(b)
	if bb.unread < n {
		n = bb.unread
		b = b[:n]
	}
	wrapAfter := bb.limit - bb.readOffset
	if wrapAfter >= n {
		copy(b, bb.data[bb.readOffset:])
		bb.readOffset += n
	} else {
		copy(b[:wrapAfter], bb.data[bb.readOffset:])
		copy(b[wrapAfter:], bb.data[0:])
		bb.readOffset += n - wrapAfter
	}
	bb.unread -= n
	return n, nil
}
