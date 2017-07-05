package pgfplugin

type PGFisherPlugin interface {
	Process(record []string) error
}
