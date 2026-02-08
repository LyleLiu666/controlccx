package main

import "testing"

func TestTokenRequiredForListenAddr(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{addr: "127.0.0.1:5174", want: false},
		{addr: "localhost:5174", want: false},
		{addr: "[::1]:5174", want: false},
		{addr: ":5174", want: true},
		{addr: "0.0.0.0:5174", want: true},
		{addr: "[::]:5174", want: true},
		{addr: "192.168.1.10:5174", want: true},
		{addr: "example.com:5174", want: true},
		{addr: "badaddr", want: true},
	}
	for _, tc := range cases {
		if got := tokenRequiredForListenAddr(tc.addr); got != tc.want {
			t.Fatalf("addr=%q: got %v want %v", tc.addr, got, tc.want)
		}
	}
}
