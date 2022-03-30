package ast

import (
	"bytes"
	"fmt"
	"github.com/biogo/store/llrb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sort"
	"testing"
)

func Test_TreeCursor(t *testing.T) {
	req := require.New(t)
	var list [][]byte
	tree := &llrb.Tree{}
	for i := 0; i < 100; i++ {
		val := uuid.New()
		baVal := val[:]
		list = append(list, baVal)
		tree.Insert(byteArrayComparable(baVal))
	}

	sort.Slice(list, func(i, j int) bool {
		return bytes.Compare(list[i], list[j]) < 0
	})

	for idx, val := range list {
		id, _ := uuid.FromBytes(val)
		fmt.Printf("%v: %v\n", idx, id.String())
	}

	fmt.Printf("\nTree\n")

	counter := 0
	tree.Do(func(comparable llrb.Comparable) (done bool) {
		id, _ := uuid.FromBytes(comparable.(byteArrayComparable))
		fmt.Printf("%v: %v\n", counter, id.String())
		counter++
		return false
	})

	cursor := NewTreeCursor(tree)
	counter = 0
	for cursor.IsValid() {
		id1, _ := uuid.FromBytes(list[counter])
		id2, _ := uuid.FromBytes(cursor.Current())
		fmt.Printf("%v: comparing %+v to %+v\n", counter, id1, id2)
		req.Equal(list[counter], cursor.Current())
		counter++
		cursor.Next()
	}
	req.Equal(len(list), counter)
}

func Test_UnionCursor(t *testing.T) {
	req := require.New(t)

	var unionList [][]byte
	var list1 [][]byte
	for i := 0; i < 5; i++ {
		val := uuid.New()
		baVal := val[:]
		list1 = append(list1, baVal)
	}

	var list2 [][]byte
	for i := 0; i < 5; i++ {
		val := uuid.New()
		baVal := val[:]
		list2 = append(list2, baVal)
	}

	unionList = append(unionList, list1...)
	unionList = append(unionList, list2...)

	sort.Slice(list1, func(i, j int) bool {
		return bytes.Compare(list1[i], list1[j]) < 0
	})

	sort.Slice(list2, func(i, j int) bool {
		return bytes.Compare(list2[i], list2[j]) < 0
	})

	sort.Slice(unionList, func(i, j int) bool {
		return bytes.Compare(unionList[i], unionList[j]) < 0
	})

	for idx, val := range unionList {
		id, _ := uuid.FromBytes(val)
		fmt.Printf("%v: %v\n", idx, id.String())
	}

	cursor1 := &sliceSetCursor{values: list1}
	cursor2 := &sliceSetCursor{values: list2}
	cursor := NewUnionSetCursor(cursor1, cursor2, true)

	counter := 0
	for cursor.IsValid() {
		id1, _ := uuid.FromBytes(unionList[counter])
		id2, _ := uuid.FromBytes(cursor.Current())
		fmt.Printf("%v: comparing %+v to %+v\n", counter, id1, id2)
		req.Equal(unionList[counter], cursor.Current())
		counter++
		cursor.Next()
	}
	req.Equal(len(unionList), counter)

	sort.Slice(list1, func(i, j int) bool {
		return bytes.Compare(list1[i], list1[j]) > 0
	})

	sort.Slice(list2, func(i, j int) bool {
		return bytes.Compare(list2[i], list2[j]) > 0
	})

	sort.Slice(unionList, func(i, j int) bool {
		return bytes.Compare(unionList[i], unionList[j]) > 0
	})

	fmt.Printf("\nlist1\n")
	for idx, val := range list1 {
		id, _ := uuid.FromBytes(val)
		fmt.Printf("%v: %v\n", idx, id.String())
	}

	fmt.Printf("\nlist2\n")
	for idx, val := range list2 {
		id, _ := uuid.FromBytes(val)
		fmt.Printf("%v: %v\n", idx, id.String())
	}

	fmt.Printf("\nunion list\n")
	for idx, val := range unionList {
		id, _ := uuid.FromBytes(val)
		fmt.Printf("%v: %v\n", idx, id.String())
	}

	cursor = NewUnionSetCursor(&sliceSetCursor{values: list1}, &sliceSetCursor{values: list2}, false)

	counter = 0
	for cursor.IsValid() {
		id1, _ := uuid.FromBytes(unionList[counter])
		id2, _ := uuid.FromBytes(cursor.Current())
		fmt.Printf("%v: comparing %+v to %+v\n", counter, id1, id2)
		req.Equal(unionList[counter], cursor.Current())
		counter++
		cursor.Next()
	}
	req.Equal(len(unionList), counter)
}
