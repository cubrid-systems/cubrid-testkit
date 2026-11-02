//go:build race

package haapply

// raceEnabled is whether the tests run under -race, where every fake csb call
// re-executes an instrumented test binary and a whole catalog takes minutes.
const raceEnabled = true
