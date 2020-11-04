package xgress

var payloadIngester *PayloadIngester

func InitPayloadIngester() {
	payloadIngester = NewPayloadIngester()
}

type payloadEntry struct {
	payload *Payload
	x       *Xgress
}

type PayloadIngester struct {
	payloadIngest  chan *payloadEntry
	payloadSendReq chan *Xgress
}

func NewPayloadIngester() *PayloadIngester {
	pi := &PayloadIngester{
		payloadIngest:  make(chan *payloadEntry, 16),
		payloadSendReq: make(chan *Xgress, 16),
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
		}
	}
}
