package connmux

import (
	"time"
)

var (
	closeTimeout = 30 * time.Second
)

type sendBuffer struct {
	streamID       []byte
	in             chan []byte
	ack            chan bool
	closeRequested chan bool
}

func newSendBuffer(streamID []byte, out chan []byte, depth int) *sendBuffer {
	buf := &sendBuffer{
		streamID:       streamID,
		in:             make(chan []byte, depth),
		ack:            make(chan bool, depth),
		closeRequested: make(chan bool, 1),
	}
	// Write initial acks to send up to depth right away
	for i := 0; i < depth; i++ {
		buf.ack <- true
	}
	go buf.sendLoop(out)
	return buf
}

func (buf *sendBuffer) sendLoop(out chan []byte) {
	sendRST := false

	defer func() {
		if sendRST {
			buf.sendRST(out)
		}

		// drain remaining writes
		for {
			select {
			case <-buf.in:
				// draining
			default:
				// done draining
				return
			}
		}
	}()

	closeTimer := time.NewTimer(largeTimeout)

	// Send one frame for every ack
	for {
		select {
		case <-buf.ack:
			// Grab next frame
			select {
			case frame, open := <-buf.in:
				if open || frame != nil {
					out <- append(frame, buf.streamID...)
				}
				if !open {
					// we've closed
					return
				}
			case sendRST = <-buf.closeRequested:
				// Signal that we're closing
				close(buf.in)
				closeTimer.Reset(closeTimeout)
			}
		case sendRST = <-buf.closeRequested:
			// Signal that we're closing
			close(buf.in)
			closeTimer.Reset(closeTimeout)
		case <-closeTimer.C:
			// We had queued writes, but we haven't gotten any acks within
			// closeTimeout of closing, don't wait any longer
			return
		}
	}
}

func (buf *sendBuffer) close(sendRST bool) {
	select {
	case buf.closeRequested <- sendRST:
		// okay
	default:
		// close already requested, ignore
	}
}

func (buf *sendBuffer) sendRST(out chan []byte) {
	// send just the streamID to indicate we've closed the connection
	rst := make([]byte, len(buf.streamID))
	copy(rst, buf.streamID)
	setFrameType(rst, frameTypeRST)
	out <- rst
}
