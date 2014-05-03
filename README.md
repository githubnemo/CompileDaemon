# Very simple Compile Daemon for Go

Watches your .go files in a directory and invokes `go build` if
a file changed. Nothing more.

Usage:

	$ ./CompileDaemon -directory=yourproject/

## Command Line Options

Option    | Default     | Description
--------- | ----------- | -----------
`‑build=…`   | go build    | Specify the command to run when rebuilding is required.
`‑command=…` | *none*      | Specify the command to run after a succesful build. The default is to run nothing. This command is issued with the working directory set to -directory.
`‑directory=…` | . | Which directory to watch.
`‑exclude‑dir=…` | none | Do not watch directories matching this glob pattern, e.g. ".git". You may have multiples of this flag.
`‑exclude=…` | none | Exclude files matching this glob pattern, e.g. ".#*" ignores emacs temporary files. You may have multiples of this flag.
`‑include=…` | none | Include files whose last path component matches this glob pattern. You may have multiples of this flag.
`‑pattern=…` | (.+\\.go&#124;.+\\.c)$ | A regular expression which matches the files to watch. The default watches *.go* and *.c* files.
`‑recursive=…` | true      | Recurse down the specified directory
