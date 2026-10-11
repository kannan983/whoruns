package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// set by the release build; `go install` builds fall back to the module version
var version = "dev"

const usage = `whoruns - find what's running, and where it lives

  whoruns              listening ports (same as: whoruns ports)
  whoruns ports 3000   only port 3000
  whoruns top          apps using the most memory
  whoruns top cpu      apps using the most CPU
  whoruns top gpu      apps using the most GPU
  whoruns --tui        live view with search and copy
  whoruns --version    print the version

Run with sudo to see processes owned by other users.

Made by Kannan <kannan848@proton.me>
https://github.com/kannan983/whoruns`

func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "", "ports":
		err = runPorts(args)
	case "top":
		err = runTop(args)
	case "--tui", "tui":
		err = runTUI()
	case "-h", "--help", "help":
		fmt.Println(usage)
	case "-v", "--version", "version":
		if bi, ok := debug.ReadBuildInfo(); ok && version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = bi.Main.Version
		}
		fmt.Println("whoruns", version)
	case "the":
		if len(args) == 1 && args[0] == "world" {
			theWorld()
		} else {
			err = fmt.Errorf("unknown command %q", "the "+strings.Join(args, " "))
		}
	default:
		if _, perr := strconv.Atoi(cmd); perr == nil { // `whoruns 3000`
			err = runPorts([]string{cmd})
		} else {
			err = fmt.Errorf("unknown command %q", cmd)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "whoruns:", err)
		if strings.HasPrefix(err.Error(), "unknown") || strings.HasPrefix(err.Error(), "usage") {
			fmt.Fprintln(os.Stderr, "\n"+usage)
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func runPorts(args []string) error {
	var filter uint32
	if len(args) > 0 {
		p, err := strconv.ParseUint(args[0], 10, 16)
		if err != nil || len(args) > 1 {
			return fmt.Errorf("usage: whoruns ports [PORT]")
		}
		filter = uint32(p)
	}
	rows, err := listPorts(filter)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		if filter != 0 {
			fmt.Printf("nothing is listening on port %d\n", filter)
		} else {
			fmt.Println("nothing is listening")
		}
		return nil
	}
	printPorts(rows)
	return nil
}

func runTop(args []string) error {
	by := "mem"
	if len(args) == 1 {
		by = args[0]
	}
	if len(args) > 1 || (by != "mem" && by != "cpu" && by != "gpu") {
		return fmt.Errorf("usage: whoruns top [mem|cpu|gpu]")
	}
	prev := takeSnapshot()
	sys := sysStats(true) // blocks ~1s, which is the CPU and GPU sampling window
	apps, now := listApps(prev)
	sys.gpu, sys.hasGPU = now.gpuTotal, gpuAvailable()
	sortApps(apps, by)
	printApps(apps, sys, by)
	return nil
}
