package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/bmatsuo/lark/luamodules/lark/core"
	"github.com/codegangsta/cli"
	"github.com/yuin/gopher-lua"
)

// CommandRun implements the "run" action (the default action)
var CommandRun = Command(func(lark *Context, cmd *cli.Command) {
	cmd.Name = "run"
	cmd.Aliases = []string{"make"}
	cmd.Usage = "Run lark project task(s)"
	cmd.ArgsUsage = `task ...

    The arguments are the names of tasks from lark.lua.`
	cmd.Flags = []cli.Flag{
		cli.IntFlag{
			Name:   "j",
			Usage:  "Number of parallel processes.",
			EnvVar: "LARK_RUN_PARALLEL",
		},
		cli.BoolFlag{
			Name:        "v",
			Usage:       "Enable verbose reporting of errors.",
			EnvVar:      "LARK_VERBOSE",
			Destination: Verbose,
		},
	}
	cmd.Action = lark.Action(Run)
})

// Run loads a lua vm and runs tasks specified in the command line.
func Run(c *Context) {
	tasks, err := normTasks(c.Args())
	if err != nil {
		log.Fatal(err)
	}

	luaFiles, err := FindTaskFiles("")
	if err != nil {
		log.Fatal(err)
	}

	luaConfig := &LuaConfig{}
	c.Lua, err = LoadVM(luaConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Lua.Close()

	err = InitLark(c, luaFiles)
	if err != nil {
		log.Fatal(err)
	}

	for _, task := range tasks {
		err := RunTask(c, task)
		if err != nil {
			os.Exit(1)
		}
	}
}

func normTasks(args []string) ([]string, error) {
	if len(args) == 0 {
		return []string{""}, nil
	}
	for _, task := range args {
		if task == "" {
			return nil, fmt.Errorf("invalid task name")
		}
	}
	return args, nil
}

// RunTask calls lark.run in state to execute task.
func RunTask(c *Context, task string) error {
	taskLit := "nil"
	if task != "" {
		taskLit = fmt.Sprintf("%q", task)
	}
	script := fmt.Sprintf("lark.run(%s)", taskLit)
	err := c.Lua.DoString(script)
	if err != nil {
		handleErr(c, err)
	}
	for {
		errwait := c.Lua.DoString(`lark.wait()`)
		if errwait == nil {
			break
		} else {
			if err == nil {
				handleErr(c, errwait)
				err = errwait
			}
		}
	}
	return err
}

func handleErr(c *Context, err error) {
	var x interface{}
	if c.Verbose() {
		x = err
	} else if e, ok := err.(*lua.ApiError); ok {
		if e.Type == lua.ApiErrorRun {
			x = e.Object
			lstr, _ := e.Object.(lua.LString)
			str := string(lstr)
			if strings.HasPrefix(str, "lark.lua:") {
				x = trimLoc(str)
			}
		} else {
			x = err
		}
	}
	core.Log(fmt.Sprint(x), &core.LogOpt{
		Color: "red",
	})
}

var reLoc = regexp.MustCompile(`^[^:]+:\d+:\s*`)

func trimLoc(msg string) string {
	return reLoc.ReplaceAllString(msg, "")
}
