module github.com/johto/pgfisher

go 1.17

require (
	github.com/fsnotify/fsnotify v1.5.1
	github.com/johto/go-csvt v0.0.0-20170705123905-f671c8103082
	github.com/johto/pgfisher/plugin v0.0.0-00010101000000-000000000000
	github.com/prometheus/client_golang v1.11.0
	go.etcd.io/bbolt v1.3.6
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/golang/protobuf v1.4.3 // indirect
	github.com/johto/pgfisher/plugin_interface v0.0.0-00010101000000-000000000000 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.26.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	google.golang.org/protobuf v1.26.0-rc.1 // indirect
)

replace github.com/johto/pgfisher/plugin => ./plugin

replace github.com/johto/pgfisher/plugin_interface => ./plugin_interface
