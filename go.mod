module main

go 1.17

require (
	github.com/fsnotify/fsnotify v1.5.1
	github.com/johto/go-csvt v0.0.0-20170705123905-f671c8103082
	github.com/johto/pgfisher/plugin v0.0.0-00010101000000-000000000000
	github.com/lib/pq v1.10.4
)

exclude github.com/johto/pgfisher/plugin v0.0.0

require golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect

replace github.com/johto/pgfisher/plugin => ./plugin
