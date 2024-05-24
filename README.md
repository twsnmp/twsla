# twsla
Simple Log Analyzer by TWSNMP


## Usage
```
Usage:
  twsla [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  count       Count log
  extract     Extract data from log
  help        Help about any command
  import      Import log from source
  search      Search logs.
  tfidf       Log analysis using TF-IDF
  version     Show twsla version

Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -h, --help               help for twsla
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range

Use "twsla [command] --help" for more information about a command.
```

## Getting started

```
~$mkdir test
~$cd test
~$twsla import -s <Log file path>
~$twsla search
```
## Copyright

see ./LICENSE

```
Copyright 2024 Masayuki Yamai
```
