package connmux

import (
	"sync"
	"time"
)

const (
	assumedAvgPacketSize = 100
)

var (
	defaultDeadline = time.Now().Add(100000 * time.Hour)
)

type boundedBuffer struct {
	limit int
	size  int
	data  chan []byte
	mx    sync.Mutex
}

func newBoundedBuffer(limit int) Buffer {
	return &boundedBuffer{
		limit: limit,
		data:  make(chan []byte, limit/assumedAvgPacketSize),
	}
}

func (bb *boundedBuffer) Write(b []byte) error {
	n := len(b)
	bb.mx.Lock()
	bb.size += n
	if bb.size > bb.limit {
		bb.mx.Unlock()
		return ErrBufferOverflow
	}
	select {
	case bb.data <- b:
		bb.mx.Unlock()
		return nil
	default:
		bb.mx.Unlock()
		return ErrBufferBlocked
	}
}

func (bb *boundedBuffer) Read(deadline time.Time) ([]byte, error) {
	var b []byte
	if deadline.IsZero() {
		b = <-bb.data
	} else {
		now := time.Now()
		if deadline.Before(now) {
			return nil, ErrTimeout
		}

		timer := time.NewTimer(deadline.Sub(now))
		select {
		case b = <-bb.data:
			timer.Stop()
		case <-timer.C:
			timer.Stop()
			return nil, ErrTimeout
		}
	}

	n := len(b)
	bb.mx.Lock()
	bb.size -= n
	bb.mx.Unlock()
	return b, nil
}
