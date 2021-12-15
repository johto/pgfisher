package main

import (
	bolt "go.etcd.io/bbolt"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type PluginInitArgs struct {
	dbh *bolt.DB
	prometheusRegistry *prometheus.Registry
	args string
}

type LogStreamPosition struct {
	Filename string `json:"filename"`
	Offset int64 `json:"offset"`
	BytesReadTotal int64 `json:"bytesReadTotal"`
}

type Plugin interface {
	Process(streamPos *LogStreamPosition, record []string) error
}

const (
	LogTimeAttno = iota
	UserNameAttno
	DatabaseNameAttno
	ProcessIDAttno
	ConnectionFromAttno
	SessionIDAttno
	SessionLineNumAttno
	CommandTagAttno
	SessionStartTimeAttno
	VirtualTransactionIDAttno
	TransactionIDAttno
	ErrorSeverityAttno
	SQLStateAttno
	MessageAttno
	DetailAttno
	HintAttno
	InternalQueryAttno
	InternalQueryPosAttno
	ContextAttno
	QueryAttno
	QueryPosAttno
	LocationAttno
	ApplicationNameAttno
	BackendTypeAttno
	LeaderPidAttno
	QueryIDAttno
)

type LogEntry struct {
	record []string
}

func NewLogEntry(record []string) (*LogEntry, error) {
	// All supported versions should have the fields up to and including
	// application_name.
	if len(record) < 1 + ApplicationNameAttno {
		return nil, fmt.Errorf("unexpected record length of %d; expected at least %d", len(record), 1 + ApplicationNameAttno)
	}
	return &LogEntry{
		record: record,
	}, nil
}

func (le *LogEntry) LogTime(loc *time.Location) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05 MST", le.record[LogTimeAttno], loc)
}

func (le *LogEntry) LogTimeString() string {
	return le.record[LogTimeAttno]
}

func (le *LogEntry) UserName() string {
	return le.record[UserNameAttno]
}

func (le *LogEntry) DatabaseName() string {
	return le.record[DatabaseNameAttno]
}

func (le *LogEntry) ProcessID() int {
	n, err := strconv.ParseInt(le.record[ProcessIDAttno], 10, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid process id %s", le.record[ProcessIDAttno]))
	}
	return int(n)
}

func (le *LogEntry) ConnectionFrom() string {
	return le.record[ConnectionFromAttno]
}

func (le *LogEntry) SessionID() string {
	return le.record[DatabaseNameAttno]
}

func (le *LogEntry) SessionLineNum() int64 {
	n, err := strconv.ParseInt(le.record[SessionLineNumAttno], 10, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid session line num %s", le.record[SessionLineNumAttno]))
	}
	return n
}

func (le *LogEntry) CommandTag() string {
	return le.record[CommandTagAttno]
}

func (le *LogEntry) SessionStartTime(loc *time.Location) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05 MST", le.record[SessionStartTimeAttno], loc)
}

func (le *LogEntry) SessionStartTimeString() string {
	return le.record[SessionStartTimeAttno]
}

func (le *LogEntry) VirtualTransactionID() string {
	return le.record[VirtualTransactionIDAttno]
}

func (le *LogEntry) TransactionID() int64 {
	n, err := strconv.ParseInt(le.record[TransactionIDAttno], 10, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid transaction id %s", le.record[TransactionIDAttno]))
	}
	return n
}

func (le *LogEntry) ErrorSeverity() string {
	return le.record[ErrorSeverityAttno]
}

func (le *LogEntry) SQLState() string {
	return le.record[SQLStateAttno]
}

func (le *LogEntry) Message() string {
	return le.record[MessageAttno]
}

func (le *LogEntry) Detail() string {
	return le.record[DetailAttno]
}

func (le *LogEntry) Hint() string {
	return le.record[HintAttno]
}

func (le *LogEntry) InternalQuery() string {
	return le.record[InternalQueryAttno]
}

func (le *LogEntry) InternalQueryPos() int {
	n, err := strconv.ParseInt(le.record[InternalQueryPosAttno], 10, 32)
	if err != nil {
		panic(fmt.Sprintf("invalid internal query pos %s", le.record[InternalQueryPosAttno]))
	}
	return int(n)
}

func (le *LogEntry) Context() string {
	return le.record[ContextAttno]
}

func (le *LogEntry) Query() string {
	return le.record[QueryAttno]
}

func (le *LogEntry) QueryPos() int {
	n, err := strconv.ParseInt(le.record[QueryPosAttno], 10, 32)
	if err != nil {
		panic(fmt.Sprintf("invalid query pos %s", le.record[QueryPosAttno]))
	}
	return int(n)
}

func (le *LogEntry) Location() string {
	return le.record[LocationAttno]
}

func (le *LogEntry) ApplicationName() string {
	return le.record[ApplicationNameAttno]
}
