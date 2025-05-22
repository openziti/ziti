package xgress_validation

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"testing"
	"time"
)

func TestBufferConn(t *testing.T) {
	conn := NewBufferConn("test")
	blocks := make(chan []byte, 1000)
	errs := make(chan error, 5)
	doneC := make(chan struct{})

	reportErr := func(err error) {
		select {
		case errs <- err:
		default:
		}
	}

	go func() {
		seed := [32]byte{}
		for i := 0; i < 32; i++ {
			seed[i] = byte(time.Now().UnixMilli() >> (i % 8))
		}
		randSrc := rand.NewChaCha8(seed)
		for i := 0; i < 1000; i++ {
			block := make([]byte, 64+rand.IntN(10000))
			_, _ = randSrc.Read(block)
			blocks <- block
			_, err := conn.Write(block)
			if err != nil {
				reportErr(err)
				return
			}
		}
	}()

	timeout := time.After(time.Second * 10)

	go func() {
		for i := 0; i < 1000; i++ {
			select {
			case block := <-blocks:
				blockCopy := make([]byte, len(block))
				n, err := io.ReadFull(conn, blockCopy)
				if err != nil {
					reportErr(fmt.Errorf("error reading block (%w)", err))
					return
				}
				if n != len(block) {
					reportErr(fmt.Errorf("block sizes did not match, expected %d, got %d", len(block), n))
					return
				}
				if !bytes.Equal(block, blockCopy) {
					reportErr(fmt.Errorf("block contents did not match for index %d", i))
					return
				}
			case <-timeout:
				select {
				case errs <- errors.New("timeout getting next block"):
				default:
				}
				return
			}
		}
		close(doneC)
	}()

	select {
	case err := <-errs:
		t.Fatal(err)
	case <-doneC:
	case <-timeout:
		t.Fatal("test timed out")
	}
}
