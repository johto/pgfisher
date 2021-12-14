package main

import (
	bolt "go.etcd.io/bbolt"
	csv "github.com/johto/go-csvt"
	"github.com/fsnotify/fsnotify"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

type PGFisher struct {
	dbh *bolt.DB
	prometheusListener net.Listener
	prometheusRegistry *prometheus.Registry

	plugin Plugin

	// Channel used by directoryWatcherLoop to communicate the next file the
	// main loop should use.
	newFilenameChan chan string

	bytesReadTotal prometheus.Counter
	bytesReadSinceLastPersist int64
}

func NewPGFisher(dbh *bolt.DB, prometheusAddr string) *PGFisher {
	listener, err := net.Listen("tcp", prometheusAddr)
	if err != nil {
		log.Fatalf("could not start listening on %s: %s", prometheusAddr, err)
	}
	registry := prometheus.NewPedanticRegistry()

	startTime := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "pgfisher_start_time_seconds",
			Help: "Start time of the process since unix epoch in seconds.",
		},
	)
	startTime.Set(float64(time.Now().UnixNano()) / 1e9)
	registry.MustRegister(startTime)

	bytesReadTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pgfisher_read_bytes_total",
			Help: "The total number of log bytes read since process start.",
		},
	)
	registry.MustRegister(bytesReadTotal)

	pgf := &PGFisher{
		dbh: dbh,
		prometheusListener: listener,
		prometheusRegistry: registry,
		newFilenameChan: make(chan string, 1),
		bytesReadTotal: bytesReadTotal,
		bytesReadSinceLastPersist: 0,
	}
	return pgf
}

func (pgf *PGFisher) MainLoop() {
	pgf.newFilenameChan = make(chan string, 1)

	err := pgf.loadPlugin()
	if err != nil {
		log.Fatalf("could not initialize plugin: %s", err)
	}

	go func() {
		muxer := http.NewServeMux()
		handler := promhttp.HandlerFor(
			pgf.prometheusRegistry,
			promhttp.HandlerOpts{
				ErrorLog: log.Default(),
			},
		)
		muxer.Handle("/metrics", handler)
		s := &http.Server{
			Handler: muxer,
		}
		log.Fatal(s.Serve(pgf.prometheusListener))
	}()

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
	var err error
	args := PluginInitArgs{
		dbh: pgf.dbh,
		prometheusRegistry: pgf.prometheusRegistry,
		args: "",
	}
	pgf.plugin, err = PGFisherPluginInit(args)
	return err
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

			// If we hit EOF, consider changing to the next file in the select
			// below.
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
		if len(record) < 23 {
			log.Fatalf("unexpected record length %d", len(record))
		}

		err = pgf.plugin.Process(record)
		if err != nil {
			log.Fatalf("the plugin's Process function failed: %s", err)
		}

		streamPos.Offset += reader.ByteOffset
		pgf.bytesReadTotal.Add(float64(reader.ByteOffset))
		pgf.bytesReadSinceLastPersist += reader.ByteOffset
		if pgf.bytesReadSinceLastPersist >= 1024 * 1024 * 32 {
			pgf.persistLogStreamPosition(streamPos)
		}
	}
}

func (pgf *PGFisher) persistLogStreamPosition(pos *LogStreamPosition) {
	err := pgf.dbh.Update(func (tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("pgfisher"))
		if bucket == nil {
			panic("nil bucket")
		}
		err := bucket.Put([]byte("filename"), []byte(pos.Filename))
		if err != nil {
			return err
		}
		err = bucket.Put([]byte("filepos"), []byte(strconv.FormatInt(pos.Offset, 10)))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Fatalf("could not write to %s: %s", pgf.dbh.Path(), err)
	}
	pgf.bytesReadSinceLastPersist = 0
}

func (pgf *PGFisher) fetchInitialLogStreamPosition() (*LogStreamPosition, error) {
	var initialFilename []byte
	var initialFilepos []byte

	err := pgf.dbh.View(func (tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("pgfisher"))
		if bucket == nil {
			panic("nil bucket")
		}
		initialFilename = bucket.Get([]byte("filename"))
		initialFilepos = bucket.Get([]byte("filepos"))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("could not fetch initial position from %s: %s", pgf.dbh.Path(), err)
	}
	if (initialFilename != nil) != (initialFilepos != nil) {
		panic(fmt.Sprintf("corrupt database: initialFilename %v initialFilepos %v", initialFilename, initialFilepos))
	}
	if initialFilename != nil {
		offset, err := strconv.ParseInt(string(initialFilepos), 10, 64)
		if err != nil {
			panic(fmt.Sprintf("corrupt database: initialFilepos %v", initialFilepos))
		}
		return &LogStreamPosition{
			Filename: string(initialFilename),
			Offset: offset,
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
	for i := range files {
		files[i] = filepath.Base(files[i])
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
