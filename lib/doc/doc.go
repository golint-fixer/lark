package doc

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/bmatsuo/lark/gluamodule"
	"github.com/bmatsuo/lark/lib/decorator/_intern"
	"github.com/bmatsuo/lark/lib/doc/internal/textutil"
	"github.com/yuin/gopher-lua"
)

// Module is a gluamodule.Module that loads the doc module.
var Module = gluamodule.New("doc", docLoader,
	intern.Module,
)

// GoDocs represents documentation for a Go object
type GoDocs struct {
	Sig    string
	Desc   string
	Params []string
	Vars   []string
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
	if len(doc.Vars) > 0 {
		_var := l.GetField(mod, "var")
		for _, v := range doc.Vars {
			l.Push(_var)
			l.Push(lua.LString(v))
			err := l.PCall(1, 1, nil)
			if err != nil {
				l.RaiseError("%s", err)
			}
			ndec++
		}
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

func docLoader(l *lua.LState) int {
	mod := l.NewTable()

	setmt, ok := l.GetGlobal("setmetatable").(*lua.LFunction)
	if !ok {
		l.RaiseError("unexpected type for setmetatable")
	}
	signatures := weakTable(l, setmt, "kv")
	descriptions := weakTable(l, setmt, "kv")
	parameters := weakTable(l, setmt, "k")
	variables := weakTable(l, setmt, "k")

	l.Push(l.GetGlobal("require"))
	l.Push(lua.LString("decorator.intern"))
	l.Call(1, 1)
	annotator := l.GetField(l.Get(-1), "annotator")
	l.Pop(1)

	newAnnotator := func(t lua.LValue, prepend bool) lua.LValue {
		l.Push(annotator)
		l.Push(t)
		l.Push(lua.LBool(prepend))
		l.Call(2, 1)
		val := l.Get(-1)
		l.Pop(1)
		return val
	}

	sig := newAnnotator(signatures, false)
	desc := newAnnotator(descriptions, false)
	param := newAnnotator(parameters, true)
	_var := newAnnotator(variables, true)

	dodoc := func(obj lua.LValue, s, d string, ps ...string) {
		l.Push(sig)
		l.Push(lua.LString(s))
		l.Call(1, 1)
		l.Push(desc)
		l.Push(lua.LString(d))
		l.Call(1, 1)
		for _, p := range ps {
			l.Push(param)
			l.Push(lua.LString(p))
			l.Call(1, 1)
		}
		l.Push(obj)
		for i := 0; i < 2+len(ps); i++ {
			l.Call(1, 1)
		}
	}

	dodoc(sig,
		"s => fn => fn",
		"A decorator that documents a function's signature.",
		`s  string -- The function signature.`,
	)
	dodoc(desc,
		"s => fn => fn",
		"A decorator that describes an object.",
		`s  string -- The object description.`,
	)
	dodoc(param,
		"s => fn => fn",
		"A decorator that describes a function parameter.",
		`s  string -- The parameter name and description separated by white space.`,
	)
	dodoc(_var,
		"s => fn => fn",
		"A decorator that describes module variable (table field).",
		`s  string -- The variable name and description separated by white space.`,
	)

	get := l.NewClosure(
		luaGet(signatures, descriptions, parameters, variables),
		signatures, descriptions, parameters, variables,
	)
	dodoc(get,
		"obj => table",
		"Retrieve a table containing documentation for obj.",
		`obj   table, function, or userdata -- The object to retrieve documentation for.`,
	)

	help := l.NewClosure(
		luaHelp(mod, get),
		mod, get,
	)
	dodoc(help,
		"obj => ()",
		"Print the documentation for obj.",
		`obj   table, function, or userdata -- The object to retrieve documentation for.`,
	)

	l.SetField(mod, "get", get)
	l.SetField(mod, "sig", sig)
	l.SetField(mod, "desc", desc)
	l.SetField(mod, "var", _var)
	l.SetField(mod, "param", param)
	l.SetField(mod, "help", help)
	l.Push(mod)
	return 1
}

func luaHelp(mod lua.LValue, get lua.LValue) lua.LGFunction {
	return func(l *lua.LState) int {
		print := l.GetGlobal("print")
		if l.GetTop() == 0 {
			def := l.GetField(mod, "default")
			if def == lua.LNil {
				return 0
			}
			lstr, ok := l.ToStringMeta(def).(lua.LString)
			if !ok {
				l.RaiseError("default is not a string")
			}
			str := textutil.Unindent(string(lstr))
			str = textutil.Wrap(str, 72)
			l.Push(print)
			l.Push(lua.LString(str))
			l.Call(1, 0)
			return 0
		}

		val := l.Get(1)
		l.SetTop(0)
		l.Push(get)
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
				str = textutil.Wrap(str, 72)
				str = strings.TrimSpace(str)
				l.Push(print)
				l.Push(lua.LString(str))
				l.Call(1, 0)
			}
			vars := l.GetField(docs, "vars")
			if vars != lua.LNil {

				vtab, ok := vars.(*lua.LTable)
				if !ok {
					l.RaiseError("variables are not a table")
				}
				if vtab.Len() > 0 {
					l.Push(print)
					l.Call(0, 0)

					l.Push(print)
					l.Push(lua.LString("Variables"))
					l.Call(1, 0)
				}
				l.ForEach(vtab, func(i, v lua.LValue) {
					v = l.ToStringMeta(v)
					s, ok := v.(lua.LString)
					if !ok {
						l.RaiseError("variable description is not a string")
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
					desc = textutil.Wrap(desc, 72)
					desc = textutil.Indent(desc, "      ")
					l.Push(print)
					l.Push(lua.LString(desc))
					l.Call(1, 0)
				})
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
				if ptab.Len() > 0 {
					l.Push(print)
					l.Call(0, 0)

					l.Push(print)
					l.Push(lua.LString("Parameters"))
					l.Call(1, 0)
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
					desc = textutil.Wrap(desc, 72)
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
				_, ok = v.(*lua.LFunction)
				if !ok {
					return
				}

				l.Push(get)
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
				l.Push(lua.LString("Functions"))
				l.Call(1, 0)
			}
			for _, t := range topics {
				l.Push(print)
				l.Call(0, 0)

				l.Push(print)
				l.Push(lua.LString(fmt.Sprintf("  %s", t.k)))
				l.Call(1, 0)

				if t.desc != lua.LNil {
					syn := textutil.Synopsis(string(t.desc))
					syn = textutil.Wrap(syn, 66)
					syn = textutil.Indent(syn, "      ")
					l.Push(print)
					l.Push(lua.LString(syn))
					l.Call(1, 0)
				}
			}
		}

		return 0
	}
}

func luaGet(signatures, descriptions, parameters, variables lua.LValue) lua.LGFunction {
	return func(l *lua.LState) int {
		val := l.Get(1)
		l.SetTop(0)
		sig := l.GetTable(signatures, val)
		desc := l.GetTable(descriptions, val)
		params := l.GetTable(parameters, val)
		vars := l.GetTable(variables, val)
		if sig == lua.LNil && desc == lua.LNil && params == lua.LNil && vars == lua.LNil {
			l.Push(lua.LNil)
			return 1
		}
		t := l.NewTable()
		l.SetField(t, "sig", sig)
		l.SetField(t, "desc", desc)
		l.SetField(t, "params", params)
		l.SetField(t, "vars", vars)
		l.Push(t)
		return 1
	}
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

func splitParam(s string) (name, desc string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}