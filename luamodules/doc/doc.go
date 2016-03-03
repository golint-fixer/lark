package doc

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bmatsuo/lark/internal/module"
	"github.com/bmatsuo/lark/luamodules/doc/internal/textutil"
	"github.com/yuin/gopher-lua"
)

// GoDocs represents documentation for a Go object
type GoDocs struct {
	Sig    string
	Desc   string
	Params []string
}

// Go sets the description for obj to desc.
func Go(l *lua.LState, obj lua.LValue, doc *GoDocs) {
	require := l.GetGlobal("require")
	l.Push(require)
	l.Push(lua.LString("doc"))
	l.Call(1, 1)
	mod := l.CheckTable(-1)
	l.Pop(1)

	ndec := 0
	if doc.Sig != "" {
		sig := l.GetField(mod, "sig")
		l.Push(sig)
		l.Push(lua.LString(doc.Sig))
		err := l.PCall(1, 1, nil)
		if err != nil {
			l.RaiseError("%s", err)
		}
		ndec++
	}

	if doc.Desc != "" {
		sig := l.GetField(mod, "desc")
		l.Push(sig)
		l.Push(lua.LString(doc.Desc))
		err := l.PCall(1, 1, nil)
		if err != nil {
			l.RaiseError("%s", err)
		}
		ndec++
	}
	if len(doc.Params) > 0 {
		param := l.GetField(mod, "param")
		for _, p := range doc.Params {
			l.Push(param)
			l.Push(lua.LString(p))
			err := l.PCall(1, 1, nil)
			if err != nil {
				l.RaiseError("%s", err)
			}
			ndec++
		}
	}
	l.Push(obj)
	for i := 0; i < ndec; i++ {
		err := l.PCall(1, 1, nil)
		if err != nil {
			l.RaiseError("%s", err)
		}
	}
}

// Module returns an instance of a Lua module.
func Module() module.Module {
	return defaultDocs.Module()
}

var defaultDocs = &Doc{}

// Doc creates lua modules that provides the doc API.
type Doc struct {
}

// Module returns a lua module.
func (d *Doc) Module() module.Module {
	return &doc{}
}

type doc struct {
	desc   *lua.LTable
	params *lua.LTable
}

func (d *doc) Loader(l *lua.LState) int {
	setmt, ok := l.GetGlobal("setmetatable").(*lua.LFunction)
	if !ok {
		l.RaiseError("unexpected type for setmetatable")
	}
	signatures := weakTable(l, setmt, "kv")
	descriptions := weakTable(l, setmt, "kv")
	parameters := weakTable(l, setmt, "k")
	decorator := l.NewFunction(luaDecorator)

	mod := l.NewTable()

	sig := l.NewClosure(func(l *lua.LState) int {
		s := l.CheckString(1)
		l.SetTop(0)
		fn := l.NewClosure(func(l *lua.LState) int {
			val := l.Get(1)
			l.SetTable(signatures, val, lua.LString(s))
			return 1
		}, signatures) // variable ``s''?
		l.Push(decorator)
		l.Push(fn)
		l.Call(1, 1)
		return 1
	}, signatures, decorator)

	desc := l.NewClosure(func(l *lua.LState) int {
		s := l.CheckString(1)
		l.SetTop(0)
		fn := l.NewClosure(func(l *lua.LState) int {
			val := l.Get(1)
			l.SetTable(descriptions, val, lua.LString(s))
			return 1
		}, descriptions)
		l.Push(decorator)
		l.Push(fn)
		l.Call(1, 1)
		return 1
	}, descriptions, decorator)

	param := l.NewClosure(func(l *lua.LState) int {
		s := l.CheckString(1)
		l.SetTop(0)
		fn := l.NewClosure(func(l *lua.LState) int {
			val := l.Get(1)
			t := l.GetTable(parameters, val)
			if t == lua.LNil {
				t = l.NewTable()
			}
			insert := l.GetField(l.GetGlobal("table"), "insert")
			l.Push(insert)
			l.Push(t)
			l.Push(lua.LNumber(1))
			l.Push(lua.LString(s))
			l.Call(3, 0)
			l.SetTable(parameters, val, t)
			return 1
		}, parameters)
		l.Push(decorator)
		l.Push(fn)
		l.Call(1, 1)
		return 1
	}, parameters, decorator)

	l.Push(sig)
	l.Push(lua.LString("s => fn => fn"))
	l.Call(1, 1)
	l.Push(desc)
	l.Push(lua.LString("A decorator that documents a function's signature."))
	l.Call(1, 1)
	l.Push(param)
	l.Push(lua.LString("s   String containing the function signature"))
	l.Call(1, 1)
	l.Push(sig)
	l.Call(1, 1)
	l.Call(1, 1)
	l.Call(1, 1)

	l.Push(sig)
	l.Push(lua.LString("s => fn => fn"))
	l.Call(1, 1)
	l.Push(desc)
	l.Push(lua.LString("A decorator that describes an object."))
	l.Call(1, 1)
	l.Push(param)
	l.Push(lua.LString("s   String containing the object description"))
	l.Call(1, 1)
	l.Push(desc)
	l.Call(1, 1)
	l.Call(1, 1)
	l.Call(1, 1)

	l.Push(sig)
	l.Push(lua.LString("s => fn => fn"))
	l.Call(1, 1)
	l.Push(desc)
	l.Push(lua.LString("A decorator that describes an function parameter."))
	l.Call(1, 1)
	l.Push(param)
	l.Push(lua.LString("s   String containing the parameter and its description separated by white space"))
	l.Call(1, 1)
	l.Push(param)
	l.Call(1, 1)
	l.Call(1, 1)
	l.Call(1, 1)

	loadDocs := l.NewClosure(func(l *lua.LState) int {
		val := l.Get(1)
		l.SetTop(0)
		sig := l.GetTable(signatures, val)
		desc := l.GetTable(descriptions, val)
		params := l.GetTable(parameters, val)
		if sig == lua.LNil && desc == lua.LNil && params == lua.LNil {
			l.Push(lua.LNil)
			return 1
		}
		t := l.NewTable()
		l.SetField(t, "sig", sig)
		l.SetField(t, "desc", desc)
		l.SetField(t, "params", params)
		l.Push(t)
		return 1
	}, signatures, descriptions, parameters)

	help := l.NewClosure(func(l *lua.LState) int {
		print := l.GetGlobal("print")
		if l.GetTop() == 0 {
			def := l.GetField(mod, "default")
			if def == lua.LNil {
				return 0
			}
			deffn, ok := def.(*lua.LFunction)
			if ok {
				l.Push(deffn)
				l.Call(0, lua.MultRet)
				n := l.GetTop()
				if n > 0 {
					ret := make([]lua.LValue, n)
					for i := 1; i <= n; i++ {
						ret[i-1] = l.Get(i)
					}
					for _, val := range ret {
						l.SetTop(0)
						l.Push(print)
						l.Push(val)
						l.Call(1, 0)
					}
				}
				return 0
			}
			l.Push(print)
			l.Push(def)
			l.Call(1, 0)
			return 0
		}

		val := l.Get(1)
		l.SetTop(0)
		l.Push(loadDocs)
		l.Push(val)
		l.Call(1, 1)
		docs := l.Get(1)
		if docs != lua.LNil {
			desc := l.GetField(docs, "desc")
			if desc != lua.LNil {
				l.Push(print)
				l.Push(lua.LString(""))
				l.Call(1, 0)

				lstr, ok := l.ToStringMeta(desc).(lua.LString)
				if !ok {
					l.RaiseError("description is not a string")
				}
				str := textutil.Unindent(string(lstr))
				str = strings.TrimSpace(str)
				l.Push(print)
				l.Push(lua.LString(str))
				l.Call(1, 0)
			}
			sig := l.GetField(docs, "sig")
			if sig != lua.LNil {
				l.Push(print)
				l.Call(0, 0)

				l.Push(print)
				l.Push(sig)
				l.Call(1, 0)
			}
			params := l.GetField(docs, "params")
			if params != lua.LNil {

				ptab, ok := params.(*lua.LTable)
				if !ok {
					l.RaiseError("parameters are not a table")
				}
				l.ForEach(ptab, func(i, v lua.LValue) {
					v = l.ToStringMeta(v)
					s, ok := v.(lua.LString)
					if !ok {
						l.RaiseError("parameter description is not a string")
					}
					name, desc := splitParam(string(s))
					if name == "" {
						return
					}

					l.Push(print)
					l.Call(0, 0)

					ln := fmt.Sprintf("  %s", name)
					l.Push(print)
					l.Push(lua.LString(ln))
					l.Call(1, 0)

					desc = textutil.Unindent(desc)
					desc = strings.TrimSpace(desc)
					desc = textutil.Wrap(desc, 60)
					desc = textutil.Indent(desc, "      ")
					l.Push(print)
					l.Push(lua.LString(desc))
					l.Call(1, 0)
				})
			}
		}

		tab, ok := val.(*lua.LTable)
		if ok {
			type Topic struct{ k, desc lua.LString }
			var topics []*Topic
			l.ForEach(tab, func(k, v lua.LValue) {
				_k, ok := k.(lua.LString)
				if !ok {
					return
				}

				l.Push(loadDocs)
				l.Push(v)
				l.Call(1, 1)
				subDocs := l.Get(-1)
				l.Pop(1)

				t := &Topic{k: _k, desc: ""}
				if subDocs != lua.LNil {
					desc := l.GetField(subDocs, "desc")
					t.desc, ok = desc.(lua.LString)
					if !ok {
						t.desc, ok = l.ToStringMeta(desc).(lua.LString)
						if !ok {
							l.RaiseError("cannot convert description to string")
						}
					}
				}

				topics = append(topics, t)
			})

			if len(topics) > 0 {
				l.Push(print)
				l.Call(0, 0)
				l.Push(print)
				l.Push(lua.LString("Subtopics"))
				l.Call(1, 0)
				l.Push(print)
				l.Call(0, 0)
			}
			maxlen := 0
			for _, t := range topics {
				if len(t.k) > maxlen {
					maxlen = len(t.k)
				}
			}
			for _, t := range topics {
				l.Push(print)
				if t.desc == lua.LNil {
					l.Push(lua.LString(fmt.Sprintf("  %s", t.k)))
				} else {
					format := "  %-" + fmt.Sprint(maxlen) + "s  %s"
					l.Push(lua.LString(fmt.Sprintf(format, t.k, t.desc)))
				}
				l.Call(1, 0)
			}
		}

		return 0
	}, mod, loadDocs)

	l.SetField(mod, "get", loadDocs)
	l.SetField(mod, "sig", sig)
	l.SetField(mod, "desc", desc)
	l.SetField(mod, "param", param)
	l.SetField(mod, "help", help)
	l.Push(mod)
	return 1
}

func weakTable(l *lua.LState, setmt *lua.LFunction, mode string) lua.LValue {
	mt := l.NewTable()
	l.SetField(mt, "__mode", lua.LString(mode))

	l.Push(setmt)
	l.Push(l.NewTable())
	l.Push(mt)
	l.Call(2, 1)
	val := l.Get(l.GetTop())
	l.Pop(1)
	return val
}

var paramRegexp = regexp.MustCompile(`^\s*(\S+)(\s+.*)$`)

func splitParam(s string) (name, desc string) {
	results := paramRegexp.FindAllStringSubmatch(s, 1)
	if len(results) == 0 {
		return "", ""
	}
	return results[0][1], results[0][2]
}
