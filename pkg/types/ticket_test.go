package types

import (
	"gotest.tools/assert"
	"testing"
)

func TestTicket_Compare(t *testing.T) {
	t1 := &Ticket{VRFProof: []byte("ticket AAAAAAAAAAAAAA")}
	t2 := &Ticket{VRFProof: []byte("ticket BBBBBBBBBBBBBB")}
	assert.Equal(t, t1.Compare(t2), 1)
	assert.Equal(t, t1.Less(t2), false)
}

func TestTicket_Quality(t1 *testing.T) {
	tests := []struct {
		vrf  string
		want float64
	}{
		{
			"ticket AAAAAAAAAAAAAA",
			0.5401823082699237,
		},
		{
			"ticket 1",
			0.2031681850495035,
		},
		{
			"ticket 5",
			0.8156229025272539,
		},
		{
			"ticket zzzzzzzzz",
			0.6878070592967909,
		},
	}
	for _, tt := range tests {
		t1.Run("", func(t1 *testing.T) {
			t := &Ticket{
				VRFProof: []byte(tt.vrf),
			}
			if got := t.Quality(); got != tt.want {
				t1.Errorf("ticket %s Quality() = %v, want %v", tt.vrf, got, tt.want)
			}
		})
	}
}
