package xgress

var payloadIngester *PayloadIngester

func InitPayloadIngester(closeNotify <-chan struct{}) {
	payloadIngester = NewPayloadIngester(closeNotify)
}

type payloadEntry struct {
	payload *Payload
	x       *Xgress
}

type PayloadIngester struct {
	payloadIngest  chan *payloadEntry
	payloadSendReq chan *Xgress
	closeNotify    <-chan struct{}
}

func NewPayloadIngester(closeNotify <-chan struct{}) *PayloadIngester {
	pi := &PayloadIngester{
		payloadIngest:  make(chan *payloadEntry, 16),
		payloadSendReq: make(chan *Xgress, 16),
		closeNotify:    closeNotify,
	}

	go pi.run()

	return pi
}

func (payloadIngester *PayloadIngester) ingest(payload *Payload, x *Xgress) {
	payloadIngester.payloadIngest <- &payloadEntry{
		payload: payload,
		x:       x,
	}
}

func (payloadIngester *PayloadIngester) run() {
	for {
		select {
		case payloadEntry := <-payloadIngester.payloadIngest:
			payloadEntry.x.payloadIngester(payloadEntry.payload)
		case x := <-payloadIngester.payloadSendReq:
			x.queueSends()
		case <-payloadIngester.closeNotify:
			return
		}
	}
}
