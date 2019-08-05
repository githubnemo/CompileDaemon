# Very simple Compile Daemon for Go [![GoDoc](https://godoc.org/github.com/githubnemo/CompileDaemon?status.png)](http://godoc.org/github.com/githubnemo/CompileDaemon)

Watches your .go files in a directory and invokes `go build` if
a file changed. Nothing more.

Usage:

	$ ./CompileDaemon -directory=yourproject/

## Installation

You can use the `go` tool to install `CompileDaemon`:

	go get github.com/githubnemo/CompileDaemon

## Command Line Options

|Option    | Default     | Description|
|--------- | ----------- | -----------|
| | | **actions** |
|`-build=…`   | go build    | Specify the command to run when rebuilding is required.|
|`-command=…` | *none*      | Specify the command to run after a succesful build. The default is to run nothing. This command is issued with the working directory set to -directory.|
| | | **file selection** |
|`-directory=…` | . | Which directory to watch.|
|`-recursive=…` | true      | Recurse down the specified directory|
|`-exclude-dir=…` | none | Do not watch directories matching this glob pattern, e.g. ".git". You may have multiples of this flag.|
|`-exclude=…` | none | Exclude files matching this glob pattern, e.g. ".#*" ignores emacs temporary files. You may have multiples of this flag.|
|`-include=…` | none | Include files whose last path component matches this glob pattern. You may have multiples of this flag.|
|`-pattern=…` | (.+\\.go&#124;.+\\.c)$ | A regular expression which matches the files to watch. The default watches *.go* and *.c* files.|
| | | **misc** |
|`-color=_` | false | Colorize the output of the daemon's status messages. |
|`-log-prefix=_` | true | Prefix all child process output with stdout/stderr labels and log timestamps. |
|`-graceful-kill=_`| false | On supported platforms, send the child process a SIGTERM to allow it to exit gracefully if possible. |

## Examples

In its simplest form, the defaults will do. With the current working directory set
to the source directory you can simply…

    $ CompileDaemon

… and it will recompile your code whenever you save a source file.

If you want it to also run your program each time it builds you might add…

    $ CompileDaemon -command="./MyProgram -my-options"

… and it will also keep a copy of your program running. Killing the old one and
starting a new one each time you build.

You may find that you need to exclude some directories and files from
monitoring, such as a .git repository or emacs temporary files…

    $ CompileDaemon -exclude-dir=.git -exclude=".#*" …

If you want to monitor files other than .go and .c files you might…

    $ CompileDaemon -include=Makefile -include="*.less" -include="*.tmpl"

## Notes

### Too many open files

If you get an error for too many open files, you might wish to exclude your .git, .hg, or similar VCS directories using `-exclude-dir=…`. This is common on OS X and BSD platforms where each watched file consumes a file descriptor.

If you still have too many open files, then you need to raise your process's file limit using the `ulimit` command. Something like `ulimit -n 1024` will probably take care of it. There is also a sysctl based limit which you may reach and need to adjust.


## Project Details

### Credits

CompileDaemon was written by [githubnemo](https://github.com/githubnemo).

Code and documentation was contributed by [jimstudt](https://github.com/jimstudt).

### Repository

CompileDaemon is kept at [https://github.com/githubnemo/CompileDaemon](https://github.com/githubnemo/CompileDaemon)

### License

CompileDaemon is licensed under the [BSD Two Clause License](https://github.com/githubnemo/CompileDaemon/blob/master/LICENSE)
