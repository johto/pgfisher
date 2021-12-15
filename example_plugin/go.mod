module your.org/example_plugin

go 1.17

require (
	github.com/johto/pgfisher/plugin_interface v0.0.0-00010101000000-000000000000
	go.etcd.io/bbolt v1.3.6
)

replace github.com/johto/pgfisher/plugin_interface => ../plugin_interface

require golang.org/x/sys v0.0.0-20200923182605-d9f96fdee20d // indirect
