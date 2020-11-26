// +build qtcgoless

package main

func initRunOnMain() {
	// Don't need to run back on main here
	runOnMain = func(f func()) { f() }
}
