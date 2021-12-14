package main

import (
	bolt "go.etcd.io/bbolt"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

type LogStreamPosition struct {
	Filename string
	Offset int64
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
		fmt.Fprintf(os.Stderr, "usage: %s DB_PATH LOG_PATH\n", os.Args[0])
		os.Exit(1)
	}
	dbPath := os.Args[1]
	logPath = os.Args[2]

	dbh, err := bolt.Open(dbPath, 0644, &bolt.Options{Timeout: time.Second})
	if err != nil {
		log.Fatalf("could not open database: %s", err)
	}
	err = dbh.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("pgfisher"))
		return err
	})
	if err != nil {
		log.Fatalf("could not update database: %s", err)
	}

	pgf := NewPGFisher(dbh, ":9488")
	pgf.MainLoop()
}
