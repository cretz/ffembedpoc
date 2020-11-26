// +build !qtcgoless

package main

import "github.com/therecipe/qt/core"

type mainHelper struct {
	core.QObject
	_ func(f func()) `slot:"runOnMainHelper,auto"`
}

func (*mainHelper) runOnMainHelper(f func()) { f() }

func initRunOnMain() {
	runOnMain = NewMainHelper(nil).RunOnMainHelper
}
