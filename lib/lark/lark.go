//go:generate ./build_lib.sh

package lark

import (
	"github.com/bmatsuo/lark/gluamodule"
	"github.com/bmatsuo/lark/lib/doc"
	"github.com/bmatsuo/lark/lib/fun"
	"github.com/bmatsuo/lark/lib/lark/core"
	"github.com/bmatsuo/lark/lib/lark/task"
	"github.com/yuin/gopher-lua"
)

// Module is a gluamodule.Module that loads the lark module.
var Module = gluamodule.New("lark", Loader,
	core.Module,
	doc.Module,
	fun.Module,
	task.Module,
)

// Loader loads the default lark module.
func Loader(l *lua.LState) int {
	fn, err := l.LoadString(LarkLib)
	if err != nil {
		l.RaiseError("%s", err)
	}
	l.Push(fn)
	l.Call(0, 1)
	return 1
}
