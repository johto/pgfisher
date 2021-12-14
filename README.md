pgfisher
========

Introduction
------------

_pgfisher_ provides a starting point for a program which tails and analyzes
PostgreSQL CSV log files.  It automatically follows log file changes without
missing lines, and provides an easy-to-use interface for parsing the data.
Requires PostgreSQL version 9.4 or later.

Writing a plugin
----------------

The code in this repository does not build as provided.  Any user of this
project is expected to write their own "plugin" and drop it into a file called
plugin.go.  An example of such a plugin can be found in
[plugin.go.example](https://github.com/johto/pgfisher/blob/master/plugin.go.example).

The plugin should only use APIs from plugin\_interface.go.
