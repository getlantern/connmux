package connmux

import (
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"testing"
	"time"
)

func TestBoundedBuffer(t *testing.T) {
	text := "abcdefghij"

	// Allocate a buffer that's large enough to hold two texts but not three
	buf := newBoundedBuffer(make([]byte, 0, 21))

	// Prepare for reading
	b := make([]byte, 10)

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Room to write twice
		for i := 0; i < 2; i++ {
			err := buf.Write([]byte(text))
			if !assert.NoError(t, err) {
				return
			}
		}
		// Third write should fail because there's no room
		err := buf.Write([]byte(text))
		if assert.Error(t, err) {
			assert.Equal(t, ErrBufferFull, err)
		}
	}()

	// Read with past timeout
	n, err := buf.Read(b, time.Now().Add(-10*time.Hour))
	if !assert.Error(t, err) {
		return
	}
	assert.True(t, err.(net.Error).Timeout())
	assert.Equal(t, "i/o timeout", err.Error())
	assert.Equal(t, 0, n)

	// Read with short timeout
	n, err = buf.Read(b, time.Now().Add(10*time.Millisecond))
	if !assert.Error(t, err) {
		return
	}
	assert.True(t, err.(net.Error).Timeout())
	assert.Equal(t, "i/o timeout", err.Error())
	assert.Equal(t, 0, n)

	time.Sleep(50 * time.Millisecond)

	// Reading again should work, twice
	for i := 0; i < 2; i++ {
		n, err = buf.Read(b, time.Time{})
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, text, string(b[:n]))
	}

	// Read with no more data
	n, err = buf.Read(b, time.Now().Add(100*time.Millisecond))
	if !assert.Error(t, err) {
		return
	}
	assert.True(t, err.(net.Error).Timeout())
	assert.Equal(t, "i/o timeout", err.Error())
	assert.Equal(t, 0, n)

	// Write some more, this time wrapping past end of buffer
	// This will work because we've cleared the buffer by reading from it.
	err = buf.Write([]byte(text))
	if !assert.NoError(t, err) {
		return
	}

	// Close to make sure we get an EOF
	buf.Close()

	// Read one last time
	n, err = buf.Read(b, time.Time{})
	if !assert.Equal(t, err, io.EOF) {
		return
	}
	assert.Equal(t, text, string(b[:n]))
}
