package result

import "unicode/utf16"

// jtable is java.util.Hashtable as JDK 8 lays it out, which is where CQT's
// summary files get their order. The children of a directory are listed in
// summary_info, and the cases of the JUnit report are walked, in the order
// Hashtable iterates their keys -- and the keys are result-directory paths
// with the run's timestamp in them, so CTP's own order differs from one run
// to the next. It is not an order anyone chose, but it is the bytes CTP
// writes, and a comparison that sorts them is a comparison that could be
// sorting away something else.
//
// What is kept is what decides the order: String.hashCode over UTF-16 code
// units, 11 buckets and a load factor of 0.75, a new entry at the head of its
// bucket, a rehash to 2n+1 that walks the old buckets from the top, and
// iteration from the top bucket down.
type jtable[V any] struct {
	buckets   [][]jentry[V]
	count     int
	threshold int
}

type jentry[V any] struct {
	key   string
	value V
}

func newJTable[V any]() *jtable[V] {
	return &jtable[V]{buckets: make([][]jentry[V], 11), threshold: 8} // (int)(11 * 0.75f)
}

// jhash is String.hashCode.
func jhash(s string) int32 {
	var h int32
	for _, u := range utf16.Encode([]rune(s)) {
		h = 31*h + int32(u)
	}
	return h
}

func (t *jtable[V]) index(key string, n int) int {
	return int(uint32(jhash(key))&0x7FFFFFFF) % n
}

func (t *jtable[V]) get(key string) (V, bool) {
	for _, e := range t.buckets[t.index(key, len(t.buckets))] {
		if e.key == key {
			return e.value, true
		}
	}
	var zero V
	return zero, false
}

func (t *jtable[V]) put(key string, value V) {
	b := t.buckets[t.index(key, len(t.buckets))]
	for i := range b {
		if b[i].key == key {
			b[i].value = value
			return
		}
	}
	if t.count >= t.threshold {
		t.rehash()
	}
	i := t.index(key, len(t.buckets))
	t.buckets[i] = append([]jentry[V]{{key, value}}, t.buckets[i]...)
	t.count++
}

func (t *jtable[V]) rehash() {
	old := t.buckets
	n := len(old)*2 + 1
	t.buckets = make([][]jentry[V], n)
	t.threshold = int(float32(n) * 0.75)
	for i := len(old) - 1; i >= 0; i-- {
		for _, e := range old[i] {
			j := t.index(e.key, n)
			t.buckets[j] = append([]jentry[V]{e}, t.buckets[j]...)
		}
	}
}

// keys is the iteration order: buckets from the top down, each from its head.
func (t *jtable[V]) keys() []string {
	out := make([]string, 0, t.count)
	for i := len(t.buckets) - 1; i >= 0; i-- {
		for _, e := range t.buckets[i] {
			out = append(out, e.key)
		}
	}
	return out
}
