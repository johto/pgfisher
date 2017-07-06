package main

import (
	"database/sql"
	csv "github.com/johto/go-csvt"
	"github.com/johto/pgfisher/pgfplugin"
	"fmt"
	"fsnotify"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"plugin"
	"sort"
	"time"
)

func (pgf *PGFisher) Tail() {
	pgf.newFilenameChan = make(chan string, 1)

	err := pgf.loadPlugin()
	if err != nil {
		log.Fatalf("could not initialize plugin %s: %s", pgf.pluginPath, err)
	}

	streamPos, err := pgf.fetchInitialLogStreamPosition()
	if err != nil {
		log.Fatalf("could not fetch initial log stream position: %s", err)
	}
	if streamPos == nil {
		streamPos = &LogStreamPosition{
			Filename: "",
			Offset: 0,
		}
	}
	epollFilenameChan, files := pgf.doInitialRead(streamPos.Filename)
	if streamPos.Filename == "" {
		if len(files) == 0 {
			log.Fatalf("could not find any suitable log files in directory %s", logPath)
		}
		streamPos.Filename = files[0]
		files = files[1:]
	}
	go pgf.directoryWatcherLoop(epollFilenameChan, files)

	log.Printf("starting to tail from file %q, position %d", streamPos.Filename, streamPos.Offset)

	for {
		filepath := path.Join(logPath, streamPos.Filename)
		fh, err := os.Open(filepath)
		if err != nil {
			log.Fatalf("could not open file %q: %s", filepath, err)
		}

		err = pgf.readFromFileUntilEOF(fh, streamPos)
		if err != nil {
			log.Fatal(err.Error())
		}

		err = fh.Close()
		if err != nil {
			log.Fatalf("could not close file %q: %s", streamPos.Filename, err)
		}

		pgf.persistLogStreamPosition(streamPos)
	}
}

func (pgf *PGFisher) loadPlugin() error {
	so, err := plugin.Open(pgf.pluginPath)
	if err != nil {
		return err
	}
	initSym, err := so.Lookup("PGFisherPluginInit")
	if err != nil {
		return fmt.Errorf("could not find symbol for Init method")
	}
	init := initSym.(func(args string) (plugin interface{}, err error))
	pl, err := init("")
	if err != nil {
		return err
	}
	pgf.plugin = pl.(pgfplugin.Plugin)
	return nil
}

func (pgf *PGFisher) readFromFileUntilEOF(fh *os.File, streamPos *LogStreamPosition) error {
	tailfTimer := time.NewTimer(time.Hour)
	nextFilename := ""

	for {
		fh.Seek(streamPos.Offset, os.SEEK_SET)
		reader := csv.NewReader(fh)
		reader.RequireTrailingNewline = true
		err := pgf.readFromFileUntilError(reader, streamPos)
		if err != nil {
			if err == io.EOF && nextFilename != "" {
				log.Printf("read loop: switching over to file %s", nextFilename)
				streamPos.Filename = nextFilename
				streamPos.Offset = 0
				return nil
			}

			// If we hit EOF, consider changing to the next file
			var optNewFilenameChan <-chan string
			if nextFilename == "" && err == io.EOF {
				optNewFilenameChan = pgf.newFilenameChan
			}

			// TODO: make configurable
			tailfTimer.Reset(time.Second)
			select {
				case nextFilename = <-optNewFilenameChan:
					log.Printf("read loop: will switch over to file %s when possible", nextFilename)
					// Don't switch over until we get the next io.EOF, or we
					// might miss lines at the very end of the file.

				case <-tailfTimer.C:
			}
			// Try again
			continue
		}
	}
}

func (pgf *PGFisher) readFromFileUntilError(reader *csv.Reader, streamPos *LogStreamPosition) error {
	for {
		reader.ByteOffset = 0
		record, err := reader.Read()
		if err != nil {
			return err
		}
		// TODO: allow this to be configured
		if len(record) != 23 {
			log.Fatalf("length of record %d is not 23 (%v)", len(record), record)
		}

		err = pgf.plugin.Process(record)
		if err != nil {
			log.Fatalf("the plugin's Process function failed: %s", err)
		}

		streamPos.Offset += reader.ByteOffset
		pgf.bytesReadSinceLastPersist += reader.ByteOffset
		if pgf.bytesReadSinceLastPersist >= 1024 * 1024 * 32 {
			pgf.persistLogStreamPosition(streamPos)
		}
	}
}

func (pgf *PGFisher) persistLogStreamPosition(pos *LogStreamPosition) {
	res, err := pgf.dbh.Exec("UPDATE pgfisher SET filename = $1, filepos = $2", pos.Filename, pos.Offset)
	if err != nil {
		log.Fatalf("could not write log position to database: %s", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		log.Fatalf("RowsAffected() failed: %s", err)
	}
	if rowsAffected != 1 {
		log.Fatalf("could not write log position to database: unexpected number of rows affected %d", rowsAffected)
	}
	pgf.bytesReadSinceLastPersist = 0
}

func (pgf *PGFisher) fetchInitialLogStreamPosition() (*LogStreamPosition, error) {
	var initialFilename sql.NullString
	var initialFilepos sql.NullInt64

	err := pgf.dbh.QueryRow("SELECT filename, filepos FROM pgfisher").Scan(&initialFilename, &initialFilepos)
	if err == sql.ErrNoRows {
		_, err = pgf.dbh.Exec("INSERT INTO pgfisher VALUES (NULL, NULL)")
		if err != nil {
			return nil, fmt.Errorf(`could not INSERT into "pgfisher": %s`, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("could not fetch initial position: %s", err)
	}
	if initialFilename.Valid {
		return &LogStreamPosition{
			Filename: initialFilename.String,
			Offset: initialFilepos.Int64,
		}, nil
	}
	return nil, nil
}

// runs in its own goroutine
func (pgf *PGFisher) fsnotifyWatcherLoop(fsw *fsnotify.Watcher, pathGlob string, newFilenameChan chan<- string) {
	for {
		select {
			case event := <-fsw.Events:
				if event.Op & fsnotify.Create == fsnotify.Create {
					path := event.Name
					match, err := filepath.Match(pathGlob, path)
					if err != nil {
						log.Panic(err)
					}
					if match {
						log.Printf("fsnotify: newly created file %q matches the glob", path)
						newFilenameChan <- filepath.Base(path)
					}
				}

			case err := <-fsw.Errors:
				log.Fatalf("fsnotify error: %s", err)
		}
	}
}

func (pgf *PGFisher) doInitialRead(initialFilename string) (<-chan string, []string) {
	// Set up a watcher before reading all files in the directory.  This way we
	// ensure that we never miss any files created while we're starting up.
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("could not create a new file system watcher: %s", err)
	}
	// TODO: how to size this channel?
	epollFilenameChan := make(chan string, 32)
	pathGlob := filepath.Join(logPath, LogGlobPattern)
	go pgf.fsnotifyWatcherLoop(fsw, pathGlob, epollFilenameChan)
	err = fsw.Add(logPath)
	if err != nil {
		log.Fatalf("could not start listening for file system notifications on %q: %s", logPath, err)
	}

	files, _ := filepath.Glob(pathGlob)
	if files == nil {
		log.Println("unable to find any log files, did you specify your glob correctly?")
		log.Println(filepath.Join(logPath, LogGlobPattern))
		os.Exit(1)
	}

	// Now drain all the events, incorporating any files created into the
	// "files" slice.  This might result in duplicates, but that doesn't
	// matter; they're eliminated in the loop below.
eventDrainLoop:
	for {
		select {
			case filename := <-epollFilenameChan:
				files = append(files, filename)
			default:
				break eventDrainLoop
		}
	}

	var initialFiles []string

	sort.Strings(files)
	previousFile := ""
	for _, file := range files {
		// Skip files we've either already read or don't care about.  Note that
		// we don't want initialFilename to appear in the list, since the main
		// loop will start reading from it automatically.
		if initialFilename != "" && path.Base(file) <= initialFilename {
			continue
		}
		// eliminate duplicates as we go
		if previousFile == file {
			continue
		}
		previousFile = file
		initialFiles = append(initialFiles, file)
	}
	return epollFilenameChan, initialFiles
}

// This loop feeds new filenames to the ReadCSVLoop.  Runs in its own
// goroutine.
func (pgf *PGFisher) directoryWatcherLoop(epollFilenameChan <-chan string, files []string) {
	currentFilename := ""
	for {
		var nextFilename string
		var nextFileChan chan string
		if len(files) > 0 {
			nextFilename = files[0]
			nextFileChan = pgf.newFilenameChan
		} else {
			nextFileChan = nil
		}

		select {
			case nextFileChan <- nextFilename:
				currentFilename = path.Base(nextFilename)
				files = files[1:]

			case evfname := <-epollFilenameChan:
				// Must sort after the file being currently read
				if currentFilename != "" && path.Base(evfname) <= currentFilename {
					log.Fatalf("newly created file %q does not sort after the current filename %q", evfname, currentFilename)
				}
				// Must sort after any of the files we've already queued
				for _, f := range files {
					if path.Base(evfname) <= path.Base(f) {
						log.Fatalf("newly created file %q does not sort after queued filename %q", evfname, f)
					}
				}
				files = append(files, evfname)
		}
	}
}
