package pgfplugin

type Plugin interface {
	Process(record []string) error
}
