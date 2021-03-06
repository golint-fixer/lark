#Lark [![Build Status](https://travis-ci.org/bmatsuo/lark.svg?branch=master)](https://travis-ci.org/bmatsuo/lark)

**NOTE:  Until version 1 is released the Lark Lua library should be considered
unstable.  As features are developed and practical experience is gained the Lua
module APIs will change to better suit the needs of developers.  Any
incompatible change will be preceded by a deprecation warning and migration
plan (if necessary).  The list of open
[issues](https://github.com/bmatsuo/lark/issues) contains all planned upcoming
incompatabilities (breaking changes).**

**BREAKING CHANGE: In lark v0.5.0 the task API was altered.  The new API is
described in the documentation for [Getting Started](docs/getting_started.md).
Users of v0.4.0 or earlier can foind information about the migration strategy
in discussions of issue [#24](https://github.com/bmatsuo/lark/issues/24)**

Lark is a Lua scripting environment that targets build systems and source tree
management tasks.  Lark is inspired by `make`, `bash` and several build systems
written in Python.  The goal of lark is retain the best parts of these systems
while avoiding features about them which make certain build or management tasks
difficult to integrate, less portable, less reliable, or more error prone.

```lua
local task = require('lark.task')

-- global variables that can be used during project tasks
name = 'foobaz'
objects = {
    'main.o',
    'util.o',
}

-- define the "build" project task.
-- build can be executed on the command line with `lark run build`.
build = lark.task .. function()
    -- compile each object.
    for _, o in pairs(objects) do lark.run(o) end

    -- compile the application.
    lark.exec('gcc', '-o', name, objects)
end

-- regular expressions can match sets of task names.
build_object = lark.pattern[[%.o$]] .. function(ctx)
    -- get the object name, construct the source path, and compile the object.
    local o = task.get_name(ctx)
    local c = string.gsub(o, '%.o$', '.c')
    lark.exec('gcc', '-c', c)
end
```

##Core Features

- A simple to install, self-contained system.
- Pattern matching tasks (a la make).
- Parameterized tasks.
- Agnostic to target programing languages.  It doesn't matter if you are
  writing Rust, Python, or Lua.
- Builtin modules to simplify shell-style scripting.
- Extensible module system allows sharing sophisticated scripting logic
  targeted at a specific use case.
- The `LUA_PATH` environment variable is ignored. The module search directory
  is rooted under the project root for repeatable builds.
- Optional command dependency checking and memoization through [external
  tools](docs/memoize.md).
- Explicit parallel processing with execution groups for synchronization.

##Roadmap features

- More idiomatic Lua API.
- System for vendored third-party modules.  Users opt out of repeatable builds
  by explicitly ignoring the module directory in their VCS. 
- Integrated dependency checking in the same spirit of the fabricate and
  memoize.py projects.

##Documentation

New users should read the guide to [Getting Started](docs/getting_started.md).
After getting comfortable with the basics, users should consult the command
reference documentation on
[godoc.org](https://godoc.org/github.com/bmatsuo/lark/cmd/lark) to familiarize
themselves more deeply with lark command and the included Lua library.

To fully leverage Lua in Lark tasks it is recommended users unfamiliar with the
language read relevant sections of the free book [Programming in
Lua](http://www.lua.org/pil/contents.html).


##Development

For developers interested in contributing to the project see the
[docs](docs/development.md) for information about setting up a development
environment.  Also make sure to review the project contribution
[guidelines](CONTRIBUTING.md) before opening any pull requests.
