package main

import (
	"fmt"
	"runtime"
)

func cmdVersion(args []string) {
	if wantsHelp(args) {
		showHelp(helpSpec{
			name:    "version",
			summary: "print the flock binary version + build info",
			usage:   "flock version",
		})
	}
	fmt.Printf("flock %s (%s/%s, %s)\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
}
