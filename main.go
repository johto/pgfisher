package main

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"log"
	"os"
	"regexp"
	"strings"
)

type LogStreamPosition struct {
	Filename string
	Offset int64
}

type PGFisherPlugin struct {
	init func() error
	process func(record []string) error
}

type PGFisher struct {
	dbh *sql.DB

	pluginPath string
	plugin *PGFisherPlugin

	// Channel used by directoryWatcherLoop to communicate the next file the
	// main loop should use.
	newFilenameChan chan string

	bytesReadSinceLastPersist int64
}

var (
	logPath string
	// These should be read via flags
	logGlob         = "postgresql-%Y-%m-%d.csv"

	// Extremely naive log_filename parser
	re             = regexp.MustCompile(`%[mdHMS]`)
	LogGlobPattern = re.ReplaceAllLiteralString(strings.Replace(logGlob, "%Y", "%H%H", -1), "[0-9][0-9]")
	fakeFileDest   = strings.NewReplacer("%Y", "2006", "%m", "01", "%d", "02", "%H", "15", "%M", "04", "%S", "05").Replace(logGlob)
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: %s LOGPATH PLUGIN\n", os.Args[0])
		os.Exit(1)
	}
	logPath = os.Args[1]
	pluginPath := os.Args[2]

	conninfo := "fallback_application_name=pgfisher"
	dbh, err := sql.Open("postgres", conninfo)
	if err != nil {
		log.Fatalf("sql.Open() failed: %s", err)
	}
	if err := dbh.Ping(); err != nil {
		log.Fatalf("could not open connection to postgres server: %s", err)
	}

	pgf := &PGFisher{
		dbh: dbh,

		pluginPath: pluginPath,
	}
	pgf.Tail()
}
