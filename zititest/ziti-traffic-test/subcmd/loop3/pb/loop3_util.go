package loop3_pb

const (
	BlockTypeRandomHashed = "random-hashed"
	BlockTypeSequential   = "sequential"
)

func (test *Test) IsRxRandomHashed() bool {
	return test.RxBlockType == "" || test.RxBlockType == BlockTypeRandomHashed
}

func (test *Test) IsRxSequential() bool {
	return test.RxBlockType == BlockTypeSequential
}

func (test *Test) IsTxRandomHashed() bool {
	return test.TxBlockType == "" || test.TxBlockType == BlockTypeRandomHashed
}

func (test *Test) IsTxSequential() bool {
	return test.TxBlockType == BlockTypeSequential
}
