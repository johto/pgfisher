package main

import (
	bolt "go.etcd.io/bbolt"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	logPath string
	// These should be read via flags
	logGlob         = "postgresql-%Y-%m-%d.csv"

	// Extremely naive log_filename parser
	re             = regexp.MustCompile(`%[mdHMS]`)
	LogGlobPattern = re.ReplaceAllLiteralString(strings.Replace(logGlob, "%Y", "%H%H", -1), "[0-9][0-9]")
	fakeFileDest   = strings.NewReplacer("%Y", "2006", "%m", "01", "%d", "02", "%H", "15", "%M", "04", "%S", "05").Replace(logGlob)
)

func printUsage(w io.Writer) {
	programName := filepath.Base(os.Args[0])
    fmt.Fprintf(w, `Usage:
  %[1]s [--help] COMMAND [ARG]...

Commands:

  tail                  tails the log stream
  initdb                initializes a database file

Options:
  --help                display this help and exit

See "%[1]s COMMAND --help" for usage of a specific command.
`, programName)

}

func printTailUsage(w io.Writer) {
	programName := filepath.Base(os.Args[0])
	fmt.Fprintf(w, `Usage:
	%[1]s tail DB_PATH LOG_PATH
`, programName)
}

func commandTail(args []string) {
	if len(args) != 2 {
		printTailUsage(os.Stderr)
		os.Exit(1)
	}
	dbPath := args[0]
	logPath = args[1]

	dbh, err := bolt.Open(dbPath, 0644, &bolt.Options{Timeout: time.Second})
	if err != nil {
		log.Fatalf("could not open database: %s", err)
	}
	pgf := NewPGFisher(dbh, ":9488")
	pgf.MainLoop()
}

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}
	command := os.Args[1]
	if command == "-h" || command == "--help" {
		printUsage(os.Stdout)
		os.Exit(0)
	}
	commandArgs := []string{}
	if len(os.Args) > 2 {
		commandArgs = os.Args[2:]
	}

	var printCommandUsage func (w io.Writer)
	var executeCommand func (args []string)
	switch command {
		case "tail":
			printCommandUsage = printTailUsage
			executeCommand = commandTail
		default:
			fmt.Fprintf(os.Stderr, "unknown command %s\n", command)
			os.Exit(1)
	}

	for _, arg := range commandArgs {
		if arg == "-h" || arg == "--help" {
			printCommandUsage(os.Stdout)
			os.Exit(0)
		}
	}
	executeCommand(commandArgs)
}
