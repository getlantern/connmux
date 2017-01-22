package connmux

type sendbuffer struct {
	in  chan []byte
	ack chan bool
}

func newSendBuffer(out chan []byte, depth int) *sendbuffer {
	buf := &sendbuffer{
		in:  make(chan []byte, depth),
		ack: make(chan bool, depth),
	}
	// Write initial acks to send up to depth right away
	for i := 0; i < depth; i++ {
		buf.ack <- true
	}
	go buf.sendLoop(out)
	return buf
}

func (buf *sendbuffer) sendLoop(out chan []byte) {
	// Send one frame for every ack
	for range buf.ack {
		out <- <-buf.in
	}
}

func (buf *sendbuffer) close() {
	close(buf.ack)
}
