package tutorial

import (
	"reflect"
	"testing"
)

func TestParseArgumentsWithStrings(t *testing.T) {
	tests := []struct {
		name string
		args string
		want []string
	}{
		{
			name: "one",
			args: "one two three 'four five'",
			want: []string{"one", "two", "three", "four five"},
		},
		{
			name: "two",
			args: "  one    two three  'four   five'  ",
			want: []string{"one", "two", "three", "four   five"},
		},
		{
			name: "two",
			args: "  one    two three  'four   \"five\"'  ",
			want: []string{"one", "two", "three", "four   \"five\""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseArgumentsWithStrings(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseArgumentsWithStrings() = %v, want %v", got, tt.want)
			}
		})
	}
}
