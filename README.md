# Very simple Compile Daemon for Go

Watches your .go files in a directory and invokes `go build` if
a file changed. Nothing more.

Usage:

	$ ./CompileDaemon -directory=yourproject/

## Command Line Options

Option    | Default     | Description
--------- | ----------- | -----------
`-build=…`   | go build    | Specify the command to run when rebuilding is required.
`-command=…` | *none*      | Specify the command to run after a succesful build. The default is to run nothing. This command is issued with the working directory set to -directory.
`-directory=…` | . | Which directory to watch.
`-pattern=…` | (.+\\.go&#124;.+\\.c)$ | A regular expression which matches the files to watch. The default watches *.go* and *.c* files.
`-recursive=…` | true      | Recurse down the specified directory
