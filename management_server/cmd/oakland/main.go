// Parts of this code uses code from 'https://github.com/caddyserver/caddy/blob/master/cmd/main.go'
//<Copyright 2015 Matthew Holt and The Caddy Authors>

package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/nokusukun/stemp"
	"os"
)

var commands map[string]*Command

func init() {
	commands = map[string]*Command{}
}

func main() {
	InitializeCommands()

	switch len(os.Args) {
	case 0:
		fmt.Printf("[FATAL] no arguments provided by OS; args[0] must be command\n")
	case 1:
		printCommands()
		os.Exit(0)
	}

	cmdname := os.Args[1]
	cmd, ok := commands[cmdname]
	if !ok {
		fmt.Println("Unknown command", cmdname)
		printCommands()
		os.Exit(1)
	}

	flagset := cmd.Flags()
	err := flagset.Parse(os.Args[2:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	exitCode, err := cmd.Function(Flags{flagset})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cmd.Name, err)
	}

	os.Exit(exitCode)
}

func printCommands() {
	fmt.Println("oakland cli\n\n" +
		"commands:")
	for name, command := range commands {
		fmt.Println(stemp.Compile("  {name:w=10} {description:w=20}\n             usage: {usage}", gin.H{
			"name":        name,
			"description": command.Description,
			"usage":       command.Usage,
		}))
	}
}
