package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var commands = map[string]func(){
	"vhd2tar":       vhd2tarMain,
	"exportSandbox": exportSandboxMain,
	"netnscfg":      netnsConfigMain,
	"remotefs":      remotefsMain,
}

func main() {
	cmd := filepath.Base(os.Args[0])
	mainFunc := commands[cmd]
	if mainFunc == nil {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		fmt.Fprintf(os.Stderr, "known commands:\n")
		for k := range commands {
			fmt.Fprintf(os.Stderr, "\t%s\n", k)
		}
		os.Exit(127)
	}

	mainFunc()
}
