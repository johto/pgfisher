package pgfplugin

import (
	"fmt"
)

type Plugin interface {
	Process(record []string) error
}

const (
	LogTimeAttno = iota
	UsernameAttno
	DatabaseNameAttno
	ProcessIDAttno
	ConnectionFromAttno
	SessionIDAttno
	SessionLineNumAttno
	CommandTagAttno
	SessionStartTimeAttno
	VirtualTransactionIdAttno
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
)

type LogEntry struct {
	record []string
}

func NewLogEntry(record []string) (*LogEntry, error) {
	if len(record) < 1 + ApplicationNameAttno {
		return nil, fmt.Errorf("unexpected record length of %d; expected at least %d", len(record), 1 + ApplicationNameAttno)
	}
	return &LogEntry{
		record: record,
	}, nil
}

func (le *LogEntry) Username() string {
	return le.record[UsernameAttno]
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
