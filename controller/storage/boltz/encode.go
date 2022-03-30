package boltz

import (
	"encoding/binary"
	"github.com/pkg/errors"
)

func EncodeStringSlice(values []string) ([]byte, error) {
	var compoundKey []byte
	for _, value := range values {
		encoded, err := EncodeByteSlice([]byte(value))
		if err != nil {
			return nil, err
		}
		compoundKey = append(compoundKey, encoded...)
	}
	return compoundKey, nil
}

func EncodeByteSlice(value []byte) ([]byte, error) {
	if len(value) > MaxLinkedSetKeySize {
		return nil, errors.Errorf("On encode, linked key component %v exceeds max size of %v", value, MaxLinkedSetKeySize)
	}
	buf := make([]byte, binary.MaxVarintLen64+len(value))
	written := binary.PutUvarint(buf, uint64(len(value)))
	buf = append(buf[0:written], value...)
	return buf, nil
}

func DecodeStringSlice(compoundKey []byte) ([]string, error) {
	var result []string
	var next []byte
	var err error

	for len(compoundKey) > 0 {
		next, compoundKey, err = DecodeNext(compoundKey)
		if err != nil {
			return nil, err
		}
		result = append(result, string(next))
	}
	return result, nil
}

func DecodeNext(val []byte) ([]byte, []byte, error) {
	keyLen, read := binary.Uvarint(val)
	if read < 1 {
		return nil, nil, errors.Errorf("incorrectly encoded compound key? %+v", val)
	}
	if keyLen > MaxLinkedSetKeySize {
		return nil, nil, errors.Errorf("On decoded, linked key component exceeds max size of %v", MaxLinkedSetKeySize)
	}
	val = val[read:]
	if len(val) < int(keyLen) {
		return nil, nil, errors.Errorf("incorrectly encoded compound key? Not enough bytes left to decode. Should be %v bytes", keyLen)
	}
	next := val[:keyLen]
	val = val[keyLen:]

	return next, val, nil
}
