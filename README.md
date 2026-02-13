# twsla
TWSLA is a simple log analysis tool of the TWSNMP series.
Works on Linux/Mac OS/Windows.

[日本語のREADME](README-ja.md)

## Install

It is recommended to install on Linux/macOS using the shell script.

```terminal
$ curl -sS https://lhx98.linkclub.jp/twise.co.jp/download/install.sh | sh
```

On Linux/macOS, it can also be installed via Homebrew.

```terminal
$ brew install twsnmp/tap/twsla
```

On Windows, download the zip file from the releases page or install via scoop.

```terminal
> scoop bucket add twsnmp https://github.com/twsnmp/scoop-bucket
> scoop install twsla
```

## Basic usage

- Create a work directory.
- cd to that directory.
- Import the log with the import command.
- search commands are searched.
- The results can be output such as CSV.

```
~$mkdir test
~$cd test
~$twsla import -s <Log file path>
~$twsla search
```

## Command explanation

[Command system](cli-map.md)

[Illustration of command system](cli-map-v.html)

You can check the commands that support the Help command.

```
$twsla help
Simple Log Analyzer by TWSNMP

Usage:
  twsla [command]

Available Commands:
  ai          AI-powered log analysis
  anomaly     Anomaly log detection
  completion  Generate the autocompletion script for the specified shell
  count       Count log
  delay       Search for delays in the access log
  email       Search or count email logs
  extract     Extract data from log
  heatmap     Command to tally log counts by day of the week and time of day
  help        Help about any command
  import      Import log from source
  mcp         MCP server
  relation    Relation Analysis
  search      Search logs.
  sigma       Detect threats using SIGMA rules
  tfidf       Log analysis using TF-IDF
  time        Time analysis
  twlogeye    Inmport notify,logs and report from twlogeye
  twsnmp      Get information and logs from TWSNMP FC
  version     Show twsla version

Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -h, --help               help for twsla
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range

Use "twsla [command] --help" for more information about a command.
```

When the command is illustrated

![](https://assets.st-note.com/img/1731635423-vj6JTY1yz0eEg9l4pdIskRKh.png?width=1200)

### import command

This command imports logs and saves them in a searchable time-series database. The command arguments are:

```
＄twsla help import
Import log from source
source is file | dir | scp | ssh | twsnmp | imap | pop3

Usage:
  twsla import [flags]

Flags:
      --api                    TWSNMP FC API Mode
  -c, --command string         SSH Command
      --emailPassword string   IMAP or POP3 password
      --emailTLS               IMAP use start TLS
      --emailUser string       IMAP or POP3 user name
  -p, --filePat string         File name pattern
  -h, --help                   help for import
      --imapFolder             List IMAP folder names
  -k, --key string             SSH Key
  -l, --logType string         TWSNNP FC log type (default "syslog")
      --noDelta                Disable delta check
      --noTS                   Import no time stamp file
  -b, --size int               Batch Size (default 10000)
      --skip                   TWSNMP FC API skip verify certificate (default true)
  -s, --source string          Log source
      --tls                    TWSNMP FC API TLS
      --utc                    Force UTC

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

Specify the location of the log to read with `-s` or `--source`.
In the latest version, you can specify files and directory names as arguments without the `-s` option.
If you specify a file, only that file will be read.

If it runs:

```terminal
＄twsla import ~/Downloads/SSH.tag.gz
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│/ Loading path=/Users/ymi/Downloads/SSH.tar.gz:SSH.log line=655,147 byte=72 MB           │
│  Total file=1 line=655,147 byte=72 MB time=1.709061625s                                 │
│▆▆▆▆▆▆▆▆▆▆▆▆▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇█████████████████████████ 72 MB/Sec│
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

It displays the number of logs, size, and the time taken.
When you specify a directory, it reads the files in that directory. If you specify a file pattern with `-p` or `--filePat`, you can limit the files read from the directory. This uses a simple filter.

```
＄twsla import -s ~/Downloads -p "Linux*"

/ Loading path=/Users/ymimacmini/Downloads/Linux_2k.log line=2,000 byte=212 kB
  Total file=1 line=2,000 byte=212 kB time=75.410115ms
```

 Starting with v1.17.0, the import status display has changed.

![](https://assets.st-note.com/img/1758319263-BqyMKkbUO0PT91w75IvZucgi.png?width=1200)

Displays sparklines.

You can also specify the filename pattern when reading from a ZIP or Tar.gz file.

When reading, you can specify a simple filter, regular expression filter, and time range to reduce the amount of data imported.

To read SCP, SSH, or TWSNMP logs, specify the URL:

`scp://root@192.168.1.210/var/log/messages`

SSH key is required.
Compatible with TWSNMP FC's web API from v1.4.0.
Specify `twsnmp://192.168.1.250:8080` in the URL of the `-s` option.
If you specify `--api`, you can import logs via the Web API.
`--logType` can also obtain logs other than syslog.

From v1.20.0, support for importing from IMAP, POP3, and email files (.eml) has been added.

To import from an IMAP server:
`imap://user:password@imap.example.com:993/INBOX`

To list IMAP folders:
`twsla import --imapFolder -s imap://user:password@imap.example.com:993`

To import from a POP3 server:
`pop3://user:password@pop3.example.com:995`

You can also specify user and password via flags:
`twsla import -s imap://imap.example.com --emailUser user --emailPassword password --emailTLS`

To import from an email file (.eml):
`twsla import sample.eml`

If you specify `--json` when reading an EVTX file from v1.1.0, the Windows event log is read in JSON format, allowing detailed information to be displayed.

![](https://assets.st-note.com/img/1717709455800-myzsaGfpvI.png?width=1200)

The log destination is specified with the `-d` option (bbolt database). If you omit it, it defaults to `twsla.db` in the current directory.
By specifying `--noDelta` from v1.8.0, it is possible to skip the time difference calculation to speed up the process.
Importing is faster when logs are in chronological order. Random logs are slower.

### search command

You can search once the logs are imported.

```
twsla help search
Search logs.
Simple filters, regular expression filters, and exclusion filters can be specified.

Usage:
  twsla search [flags]

Flags:
  -c, --color string   Color mode
  -h, --help           help for search

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

You can narrow down the logs by specifying a simple filter, regular expression filter, and time range. In the current version, it acts as an inverted filter when starting with a simple filter argument.


```
＄twsla search -f fail
```

Example search:

![](https://assets.st-note.com/img/1716672574781-gcWleWK4jC.png?width=1200)


A key input help is displayed at the top right of the search result screen.
You can save the result with the `S` key. The display is reversed with the `R` key. `Q` key exits.
Log search results can be color-coded since v1.5.0.
Specify `-c` or `--color` as an option for the `search` command. For the key:

|Key|Descr|
|---|---|
| IP | Color display of IP address |
| Mac | Color display of MAC address |
| Email | Color display of email address |
| URL | Color display of URL |
| Filter | Color display of the string specified in the filter |
| REGEXP/Pattern/Color | Display the string that matches the regular expression in the specified color |

Example:

```
twsla search -f Failed -c "regex/user\s+\S+/9,ip,filter"
```

![](https://assets.st-note.com/img/1726436365-hzP1IyTxiYNnQGakBf2pb6uL.png?width=1200)

From v1.6.0, you can specify color displays from the search results screen.
Press the `C` key to display the input screen. When you press the `M` key:


![](https://assets.st-note.com/img/1729478132-JVbuz3MD1LrvKYPxFHmAnpSg.png?width=1200)

Displays the marker input screen. Following `simple filter` or `regex:`, you can specify a regular expression filter and mark the corresponding strings in the log. This is an example of an IP color and a marker for "Fail".

![](https://assets.st-note.com/img/1729484628-MxPyZJRoNU0bqCkeXmh7cAEG.png?width=1200)

### count command

This command aggregates the number of logs on an hourly basis, or uses data extracted from the log as a key.

```terminal
＄twsla help count
Count the number of logs.
Count logs for each specified period.
Number of occurrences of items extracted from the log.
Count normalized logs by pattern:
 $twsla count -e normalize
Count words in logs:
 $twsla count -e word
Count JSON keys:
 $twsla count -e json -n Score

Usage:
  twsla count [flags]

Flags:
      --delay int        Delay filter
  -e, --extract string   Extract pattern or mode. mode is json,grok,word,normalize
      --geoip string     geo IP database file
  -g, --grok string      grok pattern definitions
  -x, --grokPat string   grok pattern
  -h, --help             help for count
  -i, --interval int     Specify the aggregation interval in seconds.
      --ip string        IP info mode(host|domain|loc|country)
  -n, --name string      Name of key
  -p, --pos int          Specify variable location (default 1)
  -q, --timePos int      Specify second time stamp position
      --utc              Force UTC

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

You can filter in the same way as search.
If an extraction pattern is specified with the `-e` option, the data is aggregated by those units. If omitted, logs are aggregated by time.
Hourly aggregation:


```terminal
$twsla count -f fail
```

![](https://assets.st-note.com/img/1717709793390-R450RHfeJN.png?width=1200)


The time interval is specified with the `-i` option. If omitted, it is set automatically.
From v1.1.0, the time difference (Delta) from the previous log is also displayed. The average interval is shown at the top.
You can sort by count with the `C` key, or by time with the `K` key.
You can save the result with the `S` key. Saving with a `.png` extension will generate a graph.

![](https://assets.st-note.com/img/1716674447895-OPrP8zMSUQ.png?width=1200)

From v1.5.0, you can save interactive graphs by using the `.html` extension.

![](https://assets.st-note.com/img/1716674531194-O7j5QXhIHo.png?width=1200)


The TOP10 ratio graph looks like this:

![](https://assets.st-note.com/img/1716674623362-MkHGX4qUZ2.png?width=1200)

A delay time filter was added in v1.16.0.

```
--delay <number>
```
Specifying this will cause the command to display logs with a delay higher than the specified number.


```
  -q, --timePos int      Specify second time stamp position
      --utc              Force UTC
```
This mode detects the time difference between two timestamps in the log, similar to the `delay` command.


### email command

This command allows searching and counting email logs stored in the database.
It provides subcommands to search for specific emails or count emails by various fields such as From, To, Subject, Sender IP, and SPF status.
In the search results, you can check the delay time and relay count calculated from the email headers.
You can sort by delay with the `d` key and by relay count with the `r` key.

```terminal
twsla email [search|count]
```

Examples:
```
  twsla email search -t "last 1h"
  twsla email count --emailCountBy from -t "last 24h"
```

### extract command

This command extracts specific data from the logs.

```terminal
$twsla help extract
Extract data from the log.
Numeric data, IP addresses, MAC addresses, email addresses,
words, etc. can be extracted.

Usage:
  twsla extract [flags]

Flags:
  -e, --extract string   Extract pattern
      --geoip string     geo IP database file
  -h, --help             help for extract
  -n, --name string      Name of value (default "Value")
  -p, --pos int          Specify variable location (default 1)

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

You can specify the same filters as the `search` command. Data extraction patterns are specified the same way as the `count` command.

```terminal
$twsla extract -f fail -e ip
```

![](https://assets.st-note.com/img/1716674893720-WqYN0wwrvt.png?width=1200)

This displays time-series data. You can sort by key and save results as a graph.

![](https://assets.st-note.com/img/1716675034354-UvMuVYryxl.png?width=1200)


Numerical data is graphed directly, while items like IP addresses are graphed by occurrence count.

![](https://assets.st-note.com/img/1736891736-Mg2ahHbtJqSws7KPcUznTvkQ.png?width=1200)

Press the `i` key while numerical data is extracted to display statistical information.

![](https://assets.st-note.com/img/1736891837-3wLoHPGn5ANfEsgDmyqxTKVh.png?width=1200)

Press the `s` key to save as CSV.


### tfidf command

Find rare logs using TF-IDF.

```terminal
＄twsla help tfidf
Use TF-IDF to find rare logs.
You can specify a similarity threshold and the number of times the threshold is allowed to be exceeded.

Usage:
  twsla tfidf [flags]

Flags:
  -c, --count int     Number of threshold crossings to exclude
  -h, --help          help for tfidf
  -l, --limit float   Similarity threshold between logs (default 0.5)
  -n, --top int       Top N

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

![](https://assets.st-note.com/img/1716675268711-yeoAjdEYAx.png?width=1200)

The example shows three rare logs found in 2,000 entries. Thresholds can be adjusted with `-l` and `-c`. Detailed information for experts will be provided in another article.
Since v1.10, you can use `-n` to get the top N rare cases.

### anomaly command

Added in v1.1.0, this command analyzes logs to find anomalies.

```terminal

Anomaly log detection
	Detect anomaly logs using isolation forests.
	Detection modes include walu, SQL injection, OS command injections, and directory traverses.

Usage:
  twsla anomaly [flags]

Flags:
  -e, --extract string   Extract pattern
  -h, --help             help for anomaly
  -m, --mode string      Detection modes(tfidf|sql|os|dir|walu|number) (default "tfidf")

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

Specify the mode with `-m`. `tfidf` creates log vectors using TF-IDF. `sql`, `os`, and `dir` create vectors based on keywords related to those attacks. `number` creates vectors from numerical values in the log.
Numerical positions can be specified with `-e`.

```
start*end
```

Example:

```
11:00 start 0.1  0.2 1.4 end
```

Only the values between `start` and `end` will be analyzed.

Analysis results:

![](https://assets.st-note.com/img/1717710550350-NG6evcVbRm.png?width=1200)

Higher scores indicate greater anomalies. `sql` and `walu` modes are effective for web server access logs.

### delay command

Added in v1.3.0, this command detects processing delays from access logs. Apache's access log records the timestamp when a request is accepted, but outputs the entry after the response is sent. This can cause timestamps to appear out of order. This reversal can be used to detect delays in processing requests or downloading large files.
It can also detect delays between timestamps when access logs are transferred to syslog with multiple timestamps.

```terminal
Search for delays in the access log

Usage:
  twsla delay [flags]

Flags:
  -h, --help          help for delay
  -q, --timePos int   Specify second time stamp position
      --utc           Force UTC

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

If `-q` is 1 or more, it processes multiple timestamps. If omitted or 0, it detects delays using the Apache access log reversal phenomenon.


![](https://assets.st-note.com/img/1723064539386-Xo4AG4qm3Y.png?width=1200)


If no delays are detected, nothing is displayed. The rightmost column shows the delay time. Select a log and press `Enter` for details. Sort by time with `T` or by delay with `D`. Save with `S`; using `.png` saves a graph image.

![](https://assets.st-note.com/img/1723064799604-VwdzrZ3bSg.png?width=1200)


### twsnmp command

This command links with TWSNMP FC (added in v1.4.0).

```terminal
Get information and logs from TWSNMP FC
[target] is node | polling | eventlog | syslog | trap |
  netflow | ipfix | sflow |sflowCounter | arplog | pollingLog

Usage:
  twsla twsnmp [target] [flags]

Flags:
      --checkCert       TWSNMP FC API verify certificate
  -h, --help            help for twsnmp
      --jsonOut         output json format
      --twsnmp string   TWSNMP FC URL (default "http://localhost:8080")

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

Specify the TWSNMP FC URL with `--twsnmp`. Include credentials if necessary:
`http://user:password@192.168.1.250:8080`

Example: Fetching the node list:

```terminal
twsla twsnmp node --twsnmp http://192.168.1.253:8081
17ea9e198e6dce8e        eve-ng-ymi.     normal  192.168.13.1
17ea9e1c9574f616        192.168.13.2    low     192.168.13.2    00:50:79:66:68:05(Private)
...
```

Outputs are tab-separated text by default. Use `--jsonOut` for JSON format, which is useful for programmatic access.

### relation command

Analyzes relationships between multiple items in log lines. Results can be output as an interactive graph.

```terminal
$twsla help relation
Analyzes the relationship between two or more pieces of data extracted from a log,
such as the relationship between an IP address and a MAC address.
data entry is ip | mac | email | url | regex/<pattern>/<color>

Usage:
  twsla relation <data1> <data2>... [flags]

Flags:
  -h, --help   help for relation

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

Supported items:

|key|descr|
| ---- | ---- |
| IP | IP address |
| Mac | MAC address |
| Email | Email address |
| URL | URL |
| REGEXP/Pattern/| String matching regular expression |


```terminal
$twsla relation -f Failed -r user "regex/user\s+\S+/" ip
```

![](https://assets.st-note.com/img/1726436651-dajM1gPELX8vny6GBW5Yz9b7.png?width=1200)


Aggregations can be viewed as an interactive graph by saving with the `.html` extension.

![](https://assets.st-note.com/img/1726436651-c86jxm75eoIZaDSHNuBr9Cd1.png?width=1200)


### heatmap command

Displays log frequency over time or date using a heat map.

```terminal
twsla help heatmap
Command to tally log counts by day of the week and time of day
	Aggregate by date mode is also available.

Usage:
  twsla heatmap [flags]

Flags:
  -h, --help   help for heatmap
  -w, --week   Week mode

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

Use `-w` for weekly aggregation; otherwise, it aggregates by date.

Date-based heat map:

![](https://assets.st-note.com/img/1726436714-pUIb1AKFhWPGuxLV2gelzRJM.png?width=1200)


Saved as HTML:

![](https://assets.st-note.com/img/1726436714-pb7ZIGOX6tPBoHY4aChR9Jzk.png?width=1200)

Weekly heat map:

![](https://assets.st-note.com/img/1726436714-UjtvDC3bVpgRHYa9hK47yfkd.png?width=1200)


### time command

Analyzes time differences between logs (added in v1.6.0).

```terminal
Time analysis

Usage:
  twsla time [flags]

Flags:
  -h, --help   help for time

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range

```

![](https://assets.st-note.com/img/1729485261-4NeIF2sytM0lxYwTrnHfikQg.png?width=1200)

`Diff` shows the difference from the marked log. `Delta` shows the difference from the previous log. Selecting a log displays detailed statistics (Mean, Median, Mode, StdDev). Use `M` to mark a log. Graphs of Delta can be saved as HTML or PNG.

![](https://assets.st-note.com/img/1729485332-0ES73fO8nqMzcLBZQtsomj19.png?width=1200)

### sigma command

Detects threats using the standard SIGMA format (https://sigmahq.io/).

```terminal
Detect threats using SIGMA rules.
	About SIGMA
	https://sigmahq.io/

Usage:
  twsla sigma [flags]

Flags:
  -c, --config string    config path
  -g, --grok string      grok definitions
  -x, --grokPat string   grok pattern if empty json mode
  -h, --help             help for sigma
  -s, --rules string     Sigma rules path
      --strict           Strict rule check

Global Flags:
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

Specify the SIGMA rules directory with `-s`. Logs are expected in JSON format; non-JSON logs require GROK extraction via `-g` and `-x`. Use `-c Windows` for built-in Windows event log mappings.

![](https://assets.st-note.com/img/1731635833-qlgh6Id4OZj27BNMse8aYSQP.png?width=1200)

Detected rules are displayed. Press `Enter` for details, `C` for rule-based display, or `G`/`H` for graphs. Save data/graphs with `S`.

### twlogeye command

Imports notifications, logs, and reports from TwLogEye.

https://twsnmp.github.io/twlogeye/
https://github.com/twsnmp/twlogeye


```terminal
Import notify, logs and reports from TwLogEye
twsla twlogeye <target> [<sub target>] [<anomaly report type>]
  target: notify | logs | report
	logs sub target: syslog | trap | netflow | winevent | otel | mqtt
	report sub target: syslog | trap | netflow | winevent | otel | mqtt | monitor | anomaly
	anomaly report type: syslog | trap | netflow | winevent | otel | mqtt | monitor | anomaly

Usage:
  twsla twlogeye [flags]

Flags:
      --anomaly string     Anomaly report type (default "monitor")
      --apiPort int        twlogeye api port number (default 8081)
      --apiServer string   twlogeye api server ip address
      --ca string          CA Cert file path
      --cert string        Client cert file path
      --filter string      Log search text
  -h, --help               help for twlogeye
      --key string         Client key file path
      --level string       Notify level

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

### ai command

Analyzes logs using LLM (significant updates in v1.17.0).

![](https://assets.st-note.com/img/1758318692-ujPGHdgEcA40JOQLNhCVz7bU.png?width=1200)

```terminal
AI-powered log analysis
Using environment variables for API keys:
 GOOGLE_API_KEY : gemini
 ANTHROPIC_API_KEY : claude
 OPENAI_API_KEY : openai

Usage:
  twsla ai <filter>... [flags]

Flags:
      --aiBaseURL string       AI base URL
      --aiErrorLevels string   Words included in the error level log (default "error,fatal,fail,crit,alert")
      --aiLang string          Language of the response
      --aiModel string         LLM Model name
      --aiProvider string      AI provider(ollama|gemini|openai|claude)
      --aiSampleSize int       Number of sample logs to be analyzed by AI (default 50)
      --aiTopNError int        Number of error log patterns to be analyzed by AI (default 10)
      --aiWarnLevels string    Words included in the warning level log (default "warn")
  -h, --help                   help for ai

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

Specify provider, model, and filters. Environment variables are used for API keys. Ollama requires no key.

Example:
```terminal
$twsla ai --aiProvider ollama --aiModel qwen3:latest <Filter>
```

![](https://assets.st-note.com/img/1758318933-VnEzfqCPXT3a9hY0k6KLpy1o.png?width=1200)

Select a log and press `e` for an AI explanation, or `a` for a summary of all searched logs.

![](https://assets.st-note.com/img/1758352154-IHNFWpQdTta6fS97AD8e2nGV.png?width=1200)
![](https://assets.st-note.com/img/1758352084-BnFKucxeG4mqSoYCT6tNprH3.png?width=1200)

### mcp command

MCP (Model Context Protocol) server for AI agents.

```terminal
$twsla help mcp
MCP server for AI agent

Usage:
  twsla mcp [flags]

Flags:
      --clients string     IP address of MCP client to be allowed to connect (comma-separated)
      --endpoint string    MCP server endpoint (default "127.0.0.1:8085")
      --geoip string       geo IP database file
  -h, --help               help for mcp
      --transport string   MCP server transport(stdio/sse/stream) (default "stdio")

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

#### System Prompt for MCP Server Tools

```
# TWSLA Log Analysis AI - System Prompt

You are an AI assistant for TWSLA (TWSNMP Log Analyzer). Your primary role is to help users analyze logs stored in the TWSLA database. You can search, count, extract data, and summarize logs.

## Available Tools

To interact with the TWSLA log database, you have access to the following tools:

### 1. `search_log`

Use this tool to search for log entries matching specific criteria.

**Parameters:**

*   `filter` (string, optional): Regular expression to filter logs. If empty, no filter is applied.
*   `limit` (integer, optional): Maximum number of log entries to return. (Min: 100, Max: 10000, Default: 100)
*   `start` (string, optional): Start date and time for the search (e.g., "2025/10/26 11:00:00"). If empty, starts from the beginning.
*   `end` (string, optional): End date and time for the search (e.g., "2025/10/26 12:00:00"). If empty, defaults to current time.

**Example:**
To search for logs containing "error" in the last hour:
`search_log(filter="error", start="-1h")`

### 2. `count_log`

Use this tool to count log entries grouped by a specific unit. This is useful for statistical analysis.

**Parameters:**

*   `filter` (string, optional): Regular expression to filter logs before counting.
*   `unit` (string, optional): Unit of counting. (Default: "time")
    *   `time`: Group by time intervals.
    *   `ip`: Group by source IP address.
    *   `email`: Group by email address.
    *   `mac`: Group by MAC address.
    *   `host`: Group by hostname (requires DNS resolution).
    *   `domain`: Group by domain name.
    *   `country`: Group by country (requires GeoIP database).
    *   `loc`: Group by geographic location (requires GeoIP database).
    *   `word`: Group by individual words in the log message.
    *   `field`: Group by a specific field (space-separated).
    *   `normalize`: Group by normalized log patterns.
*   `unit_pos` (integer, optional): Position of the unit if `unit` is "field". (Default: 1)
*   `top_n` (integer, optional): Number of top results to return. (Default: 10)
*   `interval` (integer, optional): Aggregation interval in seconds if `unit` is "time". (Default: auto)
*   `start` (string, optional): Start time for the search.
*   `end` (string, optional): End time for the search.

**Example:**
To count the top 10 source IP addresses in the last 24 hours:
`count_log(unit="ip", top_n=10, start="-24h")`

### 3. `extract_data_from_log`

Use this tool to extract specific information (IP addresses, email addresses, custom patterns, etc.) from log entries.

**Parameters:**

*   `filter` (string, optional): Regular expression to filter logs before extraction.
*   `pattern` (string, required): Pattern of data to extract.
    *   `ip`, `mac`, `email`, `number`
    *   Or a custom regular expression.
*   `pos` (integer, optional): Position of data to extract if multiple matches are found. (Default: 1)
*   `start` (string, optional): Start time for the search.
*   `end` (string, optional): End time for the search.

**Example:**
To extract all IP addresses from logs containing "failed login" in the last day:
`extract_data_from_log(filter="failed login", pattern="ip", start="-1d")`

### 4. `import_log`

Use this tool to import new logs into the TWSLA database from a file or directory.

**Parameters:**

*   `path` (string, required): Path to the log file or directory. Can handle compressed files like `.zip`, `.tar.gz`, `.gz`.
*   `pattern` (string, optional): Regular expression to filter filenames within a directory or archive.

**Example:**
To import all `.log` files from the `/var/log/` directory:
`import_log(path="/var/log/", pattern=".*\.log")`

### 5. `get_log_summary`

Use this tool to get an overview of logs for a specified period. The summary includes total entries, error and warning counts, and top error patterns.

**Parameters:**

*   `filter` (string, optional): Regular expression to filter logs.
*   `top_n` (integer, optional): Number of top error patterns to return. (Default: 10)
*   `start` (string, optional): Start time for the summary.
*   `end` (string, optional): End time for the summary.

**Example:**
To get a summary of all logs from yesterday:
`get_log_summary(start="-1d", end="today")`

## General Instructions

*   Always analyze the user's request carefully to choose the most appropriate tool.
*   When dealing with time, you can use relative periods (e.g., "-1h", "-24h") or absolute timestamps.
*   Combine tools to answer complex questions. For example, use `search_log` first to get an overview of data, then use `count_log` or `extract_data_from_log` for detailed analysis.
*   If the user's request is ambiguous, ask for clarification before running any tools.
```

#### MCP Server Configuration

- **Transport**: `stdio` (console), `sse` (server-sent events), or `stream` (HTTP with client filtering).
- **Endpoint**: Default `127.0.0.1:8085`.
- **Clients**: Whitelist of IP addresses specified as comma-separated values.


### completion command

Generates shell autocompletion scripts for `bash`, `fish`, `powershell`, and `zsh`.

Bash (Linux):
```terminal
$ twsla completion bash | sudo tee /etc/bash_completion.d/twsla
```

Zsh (macOS):
```terminal
$ mkdir -p ~/.zsh/completion/
$ twsla completion zsh > ~/.zsh/completion/_twsla
```
Add to `~/.zshrc`:
```sh
fpath=(~/.zsh/completion $fpath)
autoload -Uz compinit && compinit -i
```

PowerShell (Windows):
```terminal
> twsla completion powershell | Out-String | Invoke-Expression
```

### version command
Displays the `twsla` version.

```terminal
$ twsla version
twsla v1.17.0(94cb1ad24408c2dc38f7d178b2d78eaf5f6ad600) 2024-12-15T21:07:47Z
```

## Basic explanation

### Supported logs

- Text files with timestamps per line
- Windows EVTX format
- TWSNMP's internal logs

Supports ZIP, Tar.gz, and GZ compression. Timestamps are automatically detected. SCP/SSH and TWSNMP FC/FK imports are also supported.

### Simple filter

A beginner-friendly alternative to regular expressions. Supports `*` and `?` wildcards. For example, `Message*` becomes `Message.*` internally. From v1.15.0, keywords are supported:

| Keyword | Descr |
| --- | --- |
| #IP | Contains IP address |
| #IPV6 | Contains IPv6 address |
| #MAC | Contains MAC address |
| #LOCAL_IP | Contains local IP address |
| #EMAIL | Contains email address |
| #URL | Contains URL |
| #CREDITCARD | Contains Credit Card number |
| #MYNUMBER | Contains My Number (Japan) |
| #PHONE_JP | Contains Japanese phone number |
| #PHONE_US | Contains US phone number |
| #PHONE_INTL | Contains international phone number |
| #ZIP_JP | Contains Japanese zip code |
| #UUID | Contains UUID |

### Exclusion filter

Exclude lines using the same logic as `grep -v`.

### Time range

Flexible input formats:
- `2024/01/01T00:00:00+0900-2024/01/02T00:00:00+0900`
- `2024/1/1,1d` (Start, duration)

### Data extraction patterns

Supported keys for `-e`: `IP`, `IPV6`, `MAC`, `Number`, `Email`, `CREDITCARD`, `MYNUMBER`, `PHONE_JP`, `PHONE_US`, `PHONE_INTL`, `ZIP_JP`, `UUID`, `LOC`, `Country`, `HOST`, `Domain`. Use `-p` to specify which occurrence to extract. Custom patterns like `count=%{number}` are also supported.

### GROK and JSON modes

Advanced extraction since v1.7.0.
- **GROK:** Use `-e grok -x <pattern>`.
- **JSON:** Use `-e json -n <jsonpath>`.

### Graphs

Save graphs as PNG or view interactive HTML versions. Graphs can also be displayed in the terminal using Sixel (`--sixel`).

### IP Information (DNS/GeoIP)

Enrich logs with GeoIP and DNS information. Requires a GeoLite2 database for `loc` and `country` modes.

```terminal
$ twsla count -e ip --ip country --geoip ./GeoLite2-City.mmdb "Failed password"
```

### Configuration and environment variables

Supports `~/.twsla.yaml` and environment variables like `TWSLA_DATASTORE`, `TWSLA_GEOIP`, `TWSLA_GROK`, and `TWSLA_SIXEL`.

| Key | Descr |
| --- | --- |
| timeRange | Time range |
| filter | Simple filter |
| regex | Regular expression filter |
| not | Inverted filter |
| extract | Extraction pattern |
| name | Variable name |
| grokPat | GROK pattern |
| ip | IP Information Mode |
| color | Color Mode |
| Rules | Sigma rules path |
| sigmaconfig | Sigma settings |
| twsnmp | TWSNMP FC URL |
| interval | Aggregation intervals |
| jsonOut | JSON format output |
| checkCert | Verify server certificate |
| datastore | Datastore path |
| geoip | GeoIP DB path |
| grok | GROK definition |
| sixel | Terminal graph display |

## Build

Built with `go-task`.
```terminal
$ task
```

## Copyright
See `./LICENSE`
Copyright 2024-2026 Masayuki Yamai
