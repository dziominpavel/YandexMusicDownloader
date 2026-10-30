package downloader

import (
	"encoding/json"
	"testing"
)

func TestIDString(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "6735580", "6735580"},
		{"float small", float64(39707), "39707"},
		{"float large", float64(6735580), "6735580"},
		{"float album", float64(30462666), "30462666"},
		{"json number", json.Number("456"), "456"},
		{"int", 789, "789"},
		{"int64", int64(1011), "1011"},
		{"fraction", float64(1.5), "1.5"},
	}
	for _, c := range cases {
		if got := idString(c.in); got != c.want {
			t.Errorf("%s: idString(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
