package hareplsuite

import (
	"fmt"
	"path/filepath"
	"testing"
)

// The corpus this runs on is lopsided: `_01_object`'s largest directory holds
// 867 of its 3,327 cases. Dealing whole directories therefore finishes no
// sooner than that one directory, whatever the shard count -- which is the
// defect this pair of tests pins from both sides.
func corpus(dirs map[string]int) []string {
	var out []string
	for name, n := range dirs {
		for i := 0; i < n; i++ {
			out = append(out, filepath.Join("/corpus", name, "cases", fmt.Sprintf("%04d.sql", i)))
		}
	}
	return out
}

func shards(n int) []*runShard {
	out := make([]*runShard, n)
	for i := range out {
		out[i] = &runShard{cluster: fmt.Sprintf("c%d", i)}
	}
	return out
}

func biggest(sh []*runShard) int {
	max := 0
	for _, s := range sh {
		if len(s.cases) > max {
			max = len(s.cases)
		}
	}
	return max
}

// Under reset=dir a directory may not be split, so one huge directory is the
// floor on the run and adding shards past that buys nothing. That is correct
// behaviour, not a bug -- the cases in it may rely on each other -- and it is
// recorded here so nobody "fixes" it without changing the reset.
func TestDealKeepsADirectoryWholeWhenTheResetIsPerDirectory(t *testing.T) {
	cases := corpus(map[string]int{"huge": 867, "a": 200, "b": 200, "c": 200})
	for _, n := range []int{4, 8, 16} {
		sh := shards(n)
		deal(sh, "dir", cases)
		if got := biggest(sh); got != 867 {
			t.Errorf("%d shards: biggest shard %d, want the whole 867-case directory", n, got)
		}
		total := 0
		for _, s := range sh {
			total += len(s.cases)
		}
		if total != len(cases) {
			t.Errorf("%d shards: dealt %d of %d cases", n, total, len(cases))
		}
	}
}

// Under reset=case the database is cleared between every case, so the
// dependence a whole directory was protecting has already been broken by the
// runner itself. Dealing by case is then free, and it is what turns a lopsided
// corpus into an even run.
func TestDealSplitsADirectoryWhenEveryCaseResets(t *testing.T) {
	cases := corpus(map[string]int{"huge": 867, "a": 200, "b": 200, "c": 200})
	sh := shards(8)
	deal(sh, "case", cases)

	even := (len(cases) + 7) / 8
	if got := biggest(sh); got > even {
		t.Errorf("biggest shard %d, want no more than %d", got, even)
	}
	seen := map[string]bool{}
	for _, s := range sh {
		for _, c := range s.cases {
			if seen[c] {
				t.Fatalf("%s was dealt twice", c)
			}
			seen[c] = true
		}
	}
	if len(seen) != len(cases) {
		t.Errorf("dealt %d of %d cases", len(seen), len(cases))
	}
}

// One shard is the run as it always was, in either mode: the corpus is handed
// over whole and in its original order.
func TestDealWithOneShardChangesNothing(t *testing.T) {
	cases := corpus(map[string]int{"a": 5})
	for _, mode := range []string{"case", "dir"} {
		sh := shards(1)
		deal(sh, mode, cases)
		if len(sh[0].cases) != len(cases) {
			t.Fatalf("reset=%s: got %d cases, want %d", mode, len(sh[0].cases), len(cases))
		}
		for i := range cases {
			if sh[0].cases[i] != cases[i] {
				t.Fatalf("reset=%s: order changed at %d", mode, i)
			}
		}
	}
}
