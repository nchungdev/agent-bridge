package core

import "testing"

func TestCheapestModel(t *testing.T) {
	cases := []struct {
		in   []Model
		want string
	}{
		{[]Model{{ID: "gemini-3.1-pro-high"}, {ID: "gemini-3.8-flash-high"}, {ID: "gemini-3.8-flash-low"}}, "gemini-3.8-flash-low"},
		{[]Model{{ID: "gpt-6-astra"}, {ID: "gpt-5.5-mini"}, {ID: "o3-pro"}}, "gpt-5.5-mini"},
		{[]Model{{ID: "big"}, {ID: "bigger"}}, "big"},
		{[]Model{{ID: "x-nano"}, {ID: "x-mini"}}, "x-nano"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := CheapestModel(c.in); got != c.want {
			t.Errorf("CheapestModel(%v)=%q want %q", c.in, got, c.want)
		}
	}
}
