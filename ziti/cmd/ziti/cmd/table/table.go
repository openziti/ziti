/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package table

import (
	"fmt"
	"io"
)

type Table struct {
	Out          io.Writer
	Rows         [][]string
	ColumnWidths []int
	ColumnAlign  []int
}

func CreateTable(out io.Writer) Table {
	return Table{
		Out: out,
	}
}

// Clear removes all rows while preserving the layout
func (t *Table) Clear() {
	t.Rows = [][]string{}
}

// AddRow adds a new row to the table
func (t *Table) AddRow(col ...string) {
	t.Rows = append(t.Rows, col)
}

func (t *Table) Render() {
	// lets figure out the max widths of each column
	for _, row := range t.Rows {
		for ci, col := range row {
			l := len(col)
			t.ColumnWidths = ensureArrayCanContain(t.ColumnWidths, ci)
			if l > t.ColumnWidths[ci] {
				t.ColumnWidths[ci] = l
			}
		}
	}

	out := t.Out
	for _, row := range t.Rows {
		lastColumn := len(row) - 1
		for ci, col := range row {
			if ci > 0 {
				fmt.Fprint(out, " ")
			}
			l := t.ColumnWidths[ci]
			align := t.GetColumnAlign(ci)
			if ci >= lastColumn && align != ALIGN_CENTER && align != ALIGN_RIGHT {
				fmt.Fprint(out, col)
			} else {
				fmt.Fprint(out, Pad(col, " ", l, align))
			}
		}
		fmt.Fprint(out, "\n")
	}
}

// SetColumnsAligns sets the alignment of the columns
func (t *Table) SetColumnsAligns(colAligns []int) {
	t.ColumnAlign = colAligns
}

// GetColumnAlign return the column alignment
func (t *Table) GetColumnAlign(i int) int {
	t.ColumnAlign = ensureArrayCanContain(t.ColumnAlign, i)
	return t.ColumnAlign[i]
}

// SetColumnAlign sets the column alignment for the given column index
func (t *Table) SetColumnAlign(i int, align int) {
	t.ColumnAlign = ensureArrayCanContain(t.ColumnAlign, i)
	t.ColumnAlign[i] = align
}

func ensureArrayCanContain(array []int, idx int) []int {
	diff := idx + 1 - len(array)
	for i := 0; i < diff; i++ {
		array = append(array, 0)
	}
	return array
}
