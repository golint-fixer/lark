package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/bmatsuo/lark/execgroup"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/yuin/gopher-lua"
)

var defaultCore = newCore(os.Stderr)

type core struct {
	logger *log.Logger
	isTTY  bool
	groups map[string]*execgroup.Group
}

func istty(w io.Writer) bool {
	type fd interface {
		Fd() uintptr
	}
	wfd, ok := w.(fd)
	if ok {
		if isatty.IsTerminal(wfd.Fd()) {
			return true
		}
	}
	return false
}

func newCore(logfile io.Writer) *core {
	c := &core{
		isTTY:  istty(logfile),
		groups: make(map[string]*execgroup.Group),
	}
	flags := log.LstdFlags
	if c.isTTY {
		flags &^= log.Ldate | log.Ltime
	}
	c.logger = log.New(logfile, "", flags)

	return c
}

// Loader preloads the lark.core module so it may be required in lua scripts.
func Loader(l *lua.LState) int {
	t := l.NewTable()
	mod := l.SetFuncs(t, Exports)
	l.Push(mod)
	return 1
}

// Exports contains the API for the lark.core lua module.
var Exports = map[string]lua.LGFunction{
	"log":        defaultCore.LuaLog,
	"environ":    defaultCore.LuaEnviron,
	"exec":       defaultCore.LuaExecRaw,
	"start":      defaultCore.LuaStartRaw,
	"make_group": defaultCore.LuaMakeGroup,
	"wait":       defaultCore.LuaWait,
}

// LuaLog logs a message from lua.
func (c *core) LuaLog(state *lua.LState) int {
	opt := &LogOpt{}
	var msg string
	v1 := state.Get(1)
	if v1.Type() == lua.LTTable {
		arr := luaTableArray(state, v1.(*lua.LTable), nil)
		if len(arr) > 0 {
			msg = fmt.Sprint(arr[0])
		}
	} else if v1.Type() == lua.LTString {
		msg = string(string(v1.(lua.LString)))
	}

	lcolor, ok := state.GetField(v1, "color").(lua.LString)
	if ok {
		opt.Color = string(lcolor)
	}

	c.log(msg, opt)

	return 0
}

func (c *core) LuaEnviron(state *lua.LState) int {
	rt := state.NewTable()

	for _, env := range os.Environ() {
		pieces := strings.SplitN(env, "=", 2)
		if len(pieces) == 2 {
			state.SetField(rt, pieces[0], lua.LString(pieces[1]))
		} else {
			state.SetField(rt, pieces[0], lua.LString(""))
		}
	}

	state.Push(rt)

	return 1
}

func luaTableArray(state *lua.LState, t *lua.LTable, vals []lua.LValue) []lua.LValue {
	t.ForEach(func(kv, vv lua.LValue) {
		if kv.Type() == lua.LTNumber {
			vals = append(vals, vv)
		}
	})
	return vals
}

// LuaMakeGroup makes creates a group with dependencies.  LuaMakeGroup expects
// one table argument.
func (c *core) LuaMakeGroup(state *lua.LState) int {
	v1 := state.Get(1)
	if v1.Type() != lua.LTTable {
		state.ArgError(1, "first argument must be a table")
		return 0
	}

	var groupname string
	var lname = state.GetField(v1, "name")
	if lname == lua.LNil {
		state.ArgError(1, "missing named value 'name'")
		return 0
	}
	if lname.Type() != lua.LTString {
		msg := fmt.Sprintf("named value 'name' is not a string: %s", lname.Type())
		state.ArgError(1, msg)
		return 0
	}
	groupname = string(lname.(lua.LString))

	var follows []string
	lfollows := state.GetField(v1, "follows")
	if lfollows != lua.LNil {
		switch val := lfollows.(type) {
		case lua.LString:
			follows = append(follows, string(val))
		case *lua.LTable:
			tvals := flattenTable(state, val)
			for _, tv := range tvals {
				s, ok := tv.(lua.LString)
				if !ok {
					msg := fmt.Sprintf("named value 'follows' may only contain strings: %s", tv.Type())
					state.ArgError(1, msg)
					return 0
				}
				follows = append(follows, string(s))
			}
		default:
			msg := fmt.Sprintf("named value 'follows' is not a table: %s", lfollows.Type())
			state.ArgError(1, msg)
			return 0
		}
	}

	var gfollows []*execgroup.Group
	for _, name := range follows {
		g, ok := c.groups[name]
		if !ok {
			g = execgroup.NewGroup(nil)
			c.groups[name] = g
		}
		gfollows = append(gfollows, g)
	}

	_, ok := c.groups[groupname]
	if ok {
		msg := fmt.Sprintf("group already exists: %q", groupname)
		state.ArgError(1, msg)
		return 0
	}

	c.groups[groupname] = execgroup.NewGroup(gfollows)
	return 0
}

func (c *core) LuaWait(state *lua.LState) int {
	var names []string
	n := state.GetTop()
	if n == 0 {
		for name := range c.groups {
			names = append(names, name)
		}
	} else {
		for i := 1; i <= n; i++ {
			names = append(names, state.CheckString(1))
		}
	}

	rt := state.NewTable()

	var err error
	for _, name := range names {
		group := c.groups[name]
		if group != nil {
			err = group.Wait()
			break
		}
	}

	if err != nil {
		msg := fmt.Sprintf("asynchronous error: %v", err)
		state.SetField(rt, "error", lua.LString(msg))
	}
	state.Push(rt)

	return 1
}

// LuaStartRaw makes executes a program.  LuaStartRaw expects one table
// argument and returns one table.
func (c *core) LuaStartRaw(state *lua.LState) int {
	v1 := state.Get(1)
	if v1.Type() != lua.LTTable {
		state.ArgError(1, "first argument must be a table")
		return 0
	}

	largs := flattenTable(state, v1.(*lua.LTable))
	if len(largs) == 0 {
		state.ArgError(1, "missing positional values")
		return 0
	}
	args := make([]string, len(largs))
	for i, larg := range largs {
		arg, ok := larg.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("positional values are not strings: %s", larg.Type())
			state.ArgError(1, msg)
			return 0
		}
		args[i] = string(arg)
	}

	opt := &ExecRawOpt{}

	ignore := false
	lignore := state.GetField(v1, "ignore")
	if lignore != lua.LNil {
		_ignore, ok := lignore.(lua.LBool)
		if !ok {
			msg := fmt.Sprintf("named value 'ignore' is not boolean: %s", lignore.Type())
			state.ArgError(1, msg)
			return 0
		}
		ignore = bool(_ignore)
	}

	var groupname string
	lgroup := state.GetField(v1, "group")
	if lgroup != lua.LNil {
		group, ok := lgroup.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'group' is not a string: %s", lgroup.Type())
			state.ArgError(1, msg)
			return 0
		}
		groupname = string(group)
	}

	ldir := state.GetField(v1, "dir")
	if ldir != lua.LNil {
		dir, ok := ldir.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'dir' is not a string: %s", ldir.Type())
			state.ArgError(1, msg)
			return 0
		}
		opt.Dir = string(dir)
	}

	lstdin := state.GetField(v1, "stdin")
	if lstdin != lua.LNil {
		stdin, ok := lstdin.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'stdin' is not a string: %s", lstdin.Type())
			state.ArgError(1, msg)
			return 0
		}
		opt.StdinFile = string(stdin)
	}

	linput := state.GetField(v1, "input")
	if linput != lua.LNil {
		input, ok := linput.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'input' is not a string: %s", linput.Type())
			state.ArgError(1, msg)
			return 0
		}
		if opt.StdinFile != "" {
			msg := fmt.Sprintf("conflicting named values 'stdin' and 'input' both provided")
			state.ArgError(1, msg)
			return 0
		}
		opt.Input = []byte(input)
	}

	lstdout := state.GetField(v1, "stdout")
	if lstdout != lua.LNil {
		stdout, ok := lstdout.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'stdout' is not a string: %s", lstdout.Type())
			state.ArgError(1, msg)
			return 0
		}
		mappend := false
		if strings.HasPrefix(string(stdout), "+") {
			mappend = true
			stdout = stdout[1:]
		}
		opt.StdoutFile = string(stdout)
		opt.StdoutAppend = mappend
	}

	lstderr := state.GetField(v1, "stderr")
	if lstderr != lua.LNil {
		stderr, ok := lstderr.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'stderr' is not a string: %s", lstderr.Type())
			state.ArgError(1, msg)
			return 0
		}
		mappend := false
		if strings.HasPrefix(string(stderr), "+") {
			mappend = true
			stderr = stderr[1:]
		}
		opt.StderrFile = string(stderr)
		opt.StderrAppend = mappend
	}

	var env []string
	lenv := state.GetField(v1, "env")
	if lenv != lua.LNil {
		t, ok := lenv.(*lua.LTable)
		if !ok {
			msg := fmt.Sprintf("env is not a table: %s", lenv.Type())
			state.ArgError(1, msg)
			return 0
		}
		var err error
		env, err = tableEnv(t)
		if err != nil {
			state.ArgError(1, err.Error())
			return 0
		}
	}
	opt.Env = env

	group, ok := c.groups[groupname]
	if !ok {
		group = execgroup.NewGroup(nil)
		c.groups[groupname] = group
	}

	err := group.Exec(func() error {
		result := c.execRaw(args[0], args[1:], opt)
		if ignore {
			return nil
		}
		return result.Err
	})

	rt := state.NewTable()

	if err != nil {
		msg := fmt.Sprintf("asynchronous error: %v", err)
		state.SetField(rt, "error", lua.LString(msg))
	}
	state.Push(rt)

	return 1
}

// LuaExecRaw makes executes a program.  LuaExecRaw expects one table argument
// and returns one table.
func (c *core) LuaExecRaw(state *lua.LState) int {
	v1 := state.Get(1)
	if v1.Type() != lua.LTTable {
		state.ArgError(1, "first argument must be a table")
		return 0
	}

	largs := flattenTable(state, v1.(*lua.LTable))
	if len(largs) == 0 {
		state.ArgError(1, "missing positional values")
		return 0
	}
	args := make([]string, len(largs))
	for i, larg := range largs {
		arg, ok := larg.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("positional values are not strings: %s", larg.Type())
			state.ArgError(1, msg)
			return 0
		}
		args[i] = string(arg)
	}

	opt := &ExecRawOpt{}

	ldir := state.GetField(v1, "dir")
	if ldir != lua.LNil {
		dir, ok := ldir.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'dir' is not a string: %s", ldir.Type())
			state.ArgError(1, msg)
			return 0
		}
		opt.Dir = string(dir)
	}

	lstdin := state.GetField(v1, "stdin")
	if lstdin != lua.LNil {
		stdin, ok := lstdin.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'stdin' is not a string: %s", lstdin.Type())
			state.ArgError(1, msg)
			return 0
		}
		opt.StdinFile = string(stdin)
	}

	linput := state.GetField(v1, "input")
	if linput != lua.LNil {
		input, ok := linput.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'input' is not a string: %s", linput.Type())
			state.ArgError(1, msg)
			return 0
		}
		if opt.StdinFile != "" {
			msg := fmt.Sprintf("conflicting named values 'stdin' and 'input' both provided")
			state.ArgError(1, msg)
			return 0
		}
		opt.Input = []byte(input)
	}

	lstdout := state.GetField(v1, "stdout")
	if lstdout != lua.LNil {
		stdout, ok := lstdout.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'stdout' is not a string: %s", lstdout.Type())
			state.ArgError(1, msg)
			return 0
		}
		mappend := false
		if strings.HasPrefix(string(stdout), "+") {
			mappend = true
			stdout = stdout[1:]
		}
		opt.StdoutFile = string(stdout)
		opt.StdoutAppend = mappend
	}

	lstderr := state.GetField(v1, "stderr")
	if lstderr != lua.LNil {
		stderr, ok := lstderr.(lua.LString)
		if !ok {
			msg := fmt.Sprintf("named value 'stderr' is not a string: %s", lstderr.Type())
			state.ArgError(1, msg)
			return 0
		}
		mappend := false
		if strings.HasPrefix(string(stderr), "+") {
			mappend = true
			stderr = stderr[1:]
		}
		opt.StderrFile = string(stderr)
		opt.StderrAppend = mappend
	}

	var env []string
	lenv := state.GetField(v1, "env")
	if lenv != lua.LNil {
		t, ok := lenv.(*lua.LTable)
		if !ok {
			msg := fmt.Sprintf("env is not a table: %s", lenv.Type())
			state.ArgError(1, msg)
			return 0
		}
		var err error
		env, err = tableEnv(t)
		if err != nil {
			state.ArgError(1, err.Error())
			return 0
		}
	}
	opt.Env = env

	result := c.execRaw(args[0], args[1:], opt)
	rt := state.NewTable()
	if result.Err != nil {
		state.SetField(rt, "error", lua.LString(result.Err.Error()))
	}
	state.Push(rt)

	return 1
}

func tableEnv(t *lua.LTable) ([]string, error) {
	var env []string
	msg := ""
	t.ForEach(func(kv, vv lua.LValue) {
		if kv.Type() == lua.LTString && vv.Type() == lua.LTString {
			defn := fmt.Sprintf("%s=%s", kv.(lua.LString), vv.(lua.LString))
			env = append(env, defn)
		} else if msg == "" {
			msg = fmt.Sprintf("invalid %s-%s environment variable pair", kv.Type(), vv.Type())
		}
	})
	if msg != "" {
		return nil, errors.New(msg)
	}
	return env, nil
}

func flattenTable(state *lua.LState, val *lua.LTable) []lua.LValue {
	var flat []lua.LValue
	largs := luaTableArray(state, val, nil)
	for _, arg := range largs {
		switch t := arg.(type) {
		case *lua.LTable:
			flat = append(flat, flattenTable(state, t)...)
		default:
			flat = append(flat, arg)
		}
	}
	return flat
}

// ExecRawResult is returned from ExecRaw
type ExecRawResult struct {
	Err error
	// Output string
}

// ExecRawOpt contains options for ExecRaw.
type ExecRawOpt struct {
	Env []string
	Dir string

	Input        []byte
	StdinFile    string
	StdoutFile   string
	StdoutAppend bool
	StderrFile   string
	// StderrAppend is ignored if StderrFile equals StdoutFile.
	StderrAppend bool

	// TODO: Output capture. This will interact interestingly with command
	// result caching and file redirection.  It should be thought out more.
	//CaptureOutput  bool
}

// ExecRaw executes the named command with the given arguments.
func ExecRaw(name string, args []string, opt *ExecRawOpt) *ExecRawResult {
	return defaultCore.execRaw(name, args, opt)
}

func (c *core) execRaw(name string, args []string, opt *ExecRawOpt) *ExecRawResult {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if opt == nil {
		err := cmd.Run()
		return &ExecRawResult{Err: err}
	}

	cmd.Env = opt.Env
	cmd.Dir = opt.Dir

	if len(opt.Input) != 0 {
		cmd.Stdin = bytes.NewReader(opt.Input)
	} else if opt.StdinFile != "" {
		f, err := os.Open(opt.StdinFile)
		if err != nil {
			return &ExecRawResult{Err: err}
		}
		defer f.Close()
		cmd.Stdin = f
	}

	if opt.StdoutFile != "" {
		f, err := getOutFile(opt.StdoutFile, opt.StdoutAppend)
		if err != nil {
			return &ExecRawResult{Err: err}
		}
		defer f.Close()
		cmd.Stdout = f
	}

	if opt.StderrFile != "" && opt.StderrFile == opt.StdoutFile {
		cmd.Stderr = cmd.Stdout
	} else if opt.StderrFile != "" {
		f, err := getOutFile(opt.StderrFile, opt.StderrAppend)
		if err != nil {
			return &ExecRawResult{Err: err}
		}
		defer f.Close()
		cmd.Stderr = f
	}

	result := &ExecRawResult{}
	result.Err = cmd.Run()
	return result
}

func getOutFile(name string, a bool) (*os.File, error) {
	if !strings.HasPrefix(name, "&") {
		flag := os.O_WRONLY | os.O_TRUNC | os.O_CREATE
		if a {
			flag |= os.O_APPEND
		}
		f, err := os.OpenFile(name, flag, 0644)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	if name == "&1" {
		return os.Stdout, nil
	}
	if name == "&2" {
		return os.Stderr, nil
	}
	return nil, fmt.Errorf("invalid file descriptor")
}

// LogOpt contains options for the Log function
type LogOpt struct {
	Color string
}

// Log logs a message to standard error.
func Log(msg string, opt *LogOpt) {
	defaultCore.log(msg, opt)
}

func (c *core) log(msg string, opt *LogOpt) {
	if opt == nil {
		opt = &LogOpt{}
	}

	var esc func(format string, v ...interface{}) string
	if opt.Color != "" && c.isTTY {
		esc = colorMap[opt.Color]
	}

	if esc != nil {
		msg = esc("%s", msg)
	}
	c.logger.Print(msg)
}

var colorMap = map[string]func(format string, v ...interface{}) string{
	"black":   color.BlackString,
	"blue":    color.BlueString,
	"cyan":    color.CyanString,
	"green":   color.GreenString,
	"magenta": color.MagentaString,
	"red":     color.RedString,
	"white":   color.WhiteString,
	"yellow":  color.YellowString,
}
