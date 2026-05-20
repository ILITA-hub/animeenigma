package httputil

import (
	"reflect"
	"testing"
)

func TestParseCommaList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty input", in: "", want: nil},
		{name: "single value", in: "a", want: []string{"a"}},
		{name: "multiple values", in: "a,b,c", want: []string{"a", "b", "c"}},
		{name: "leading and trailing whitespace", in: " a , b ", want: []string{"a", "b"}},
		{name: "trailing comma", in: "a,", want: []string{"a"}},
		{name: "leading comma", in: ",a", want: []string{"a"}},
		{name: "lone comma", in: ",", want: nil},
		{name: "internal empty", in: "a,,b", want: []string{"a", "b"}},
		{name: "whitespace only", in: " ", want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCommaList(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseCommaList(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
