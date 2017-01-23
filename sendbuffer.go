package connmux

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
	defer func() {
		// drain in
		for {
			select {
			case <-buf.in:
				// found something
			default:
				// done draining
				return
			}
		}
	}()

	// Send one frame for every ack
	for {
		select {
		case <-buf.ack:
			// Grab next frame
			select {
			case frame := <-buf.in:
				out <- append(frame, buf.streamID...)
			case sendRST := <-buf.closeRequested:
				// done
				if sendRST {
					buf.sendRST(out)
				}
				return
			}
		case sendRST := <-buf.closeRequested:
			// done
			if sendRST {
				buf.sendRST(out)
			}
			return
		}
	}
}

func (buf *sendBuffer) sendRST(out chan []byte) {
	// send just the streamID to indicate we've closed the connection
	rst := make([]byte, len(buf.streamID))
	copy(rst, buf.streamID)
	setFrameType(rst, frameTypeRST)
	out <- rst
}

func (buf *sendBuffer) close(sendRST bool) {
	select {
	case buf.closeRequested <- sendRST:
		// okay
	default:
		// close already requested, ignore
	}
}
