all:

test: internal/plugin
	cd cmd/pgfisher && go build

internal/plugin: example_plugin/plugin.go
	cp -r example_plugin internal/plugin

.PHONY: test
