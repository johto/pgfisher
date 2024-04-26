package plugin

import (
	"fmt"

	pgfplugin "github.com/johto/pgfisher/plugin_interface"
	bolt "go.etcd.io/bbolt"
)

// This struct should implement plugin_interface.Plugin.
type ExamplePlugin struct {
	dbh *bolt.DB
}

func PGFisherPluginInit(args pgfplugin.PluginInitArgs) (pgfplugin.Plugin, error) {
	plugin := &ExamplePlugin{
		dbh: args.DBH,
	}
	return plugin, nil
}

func (p *ExamplePlugin) Process(streamPos *pgfplugin.LogStreamPosition, record []string) error {
	le, err := pgfplugin.NewLogEntry(record)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", le.Message())
	return nil
}
