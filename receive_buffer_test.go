package connmux

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReceiveBuffer(t *testing.T) {
	depth := 5

	pool := &testpool{}
	ack := make(chan bool, 1000)
	buf := newReceiveBuffer(ack, pool, depth)
	for i := 0; i < 2; i++ {
		b := pool.Get()
		b[frameHeaderLen] = fmt.Sprint(i)[0]
		buf.in <- b[:frameHeaderLen+1]
	}

	b := make([]byte, 2)
	n, err := buf.read(b, time.Time{})
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 2, n)
	assert.Equal(t, "01", string(b[:n]))
	assert.Equal(t, maxFrameLen, pool.getTotalReturned(), "Failed to return first buffer to pool")

	totalAcks := 0
ackloop:
	for {
		select {
		case <-ack:
			totalAcks += 1
		default:
			break ackloop
		}
	}
	assert.Equal(t, 2, totalAcks)
}

type testpool struct {
	totalReturned int64
}

func (tp *testpool) Get() []byte {
	return make([]byte, maxFrameLen)
}

func (tp *testpool) Put(b []byte) {
	atomic.AddInt64(&tp.totalReturned, int64(len(b)))
}

func (tp *testpool) getTotalReturned() int {
	return int(atomic.LoadInt64(&tp.totalReturned))
}
