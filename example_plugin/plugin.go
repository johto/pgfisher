package plugin

import (
	bolt "go.etcd.io/bbolt"
	"fmt"
	pgfplugin "github.com/johto/pgfisher/plugin_interface"
)

func PGFisherPluginInit(args pgfplugin.PluginInitArgs) (pgfplugin.Plugin, error) {
	plugin := &ExamplePlugin{
		dbh: args.DBH,
	}
	return plugin, nil
}

type ExamplePlugin struct {
	dbh *bolt.DB
}

func (p *ExamplePlugin) Process(streamPos *pgfplugin.LogStreamPosition, record []string) error {
	le, err := pgfplugin.NewLogEntry(record)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", le.Message())
	return nil
}
