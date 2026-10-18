package main

import (
	"testing"

	"trustissue/trustctl/internal/protocol"
)

func TestResolve(t *testing.T) {
	keys := []protocol.Key{{ID: 0, Name: "GitHub"}, {ID: 3, Name: "GitLab work"}, {ID: 7, Name: "AWS"}, {ID: 12, Name: "7"}}
	cases := []struct {
		arg  string
		want int // -1: error
	}{
		{"GitHub", 0}, {"github", 0}, {"aws", 7}, {"gitl", 3}, {"3", 3}, {"7", 12} /* a name beats a slot */, {"12", 12},
		{"git", -1} /* ambiguous */, {"nope", -1}, {"99", -1},
	}
	for _, c := range cases {
		k, err := resolve(keys, c.arg)
		if c.want < 0 {
			if err == nil {
				t.Errorf("%q: want an error, got %v", c.arg, k)
			}
		} else if err != nil || k.ID != c.want {
			t.Errorf("%q: got %v %v, want slot %d", c.arg, k, err, c.want)
		}
	}
}
