# twsla
TWSLA is a simple log analysis tool of the TWSNMP series.
Works on Linux/Mac OS/Windows.

[日本語のREADME](README-ja.md)

## Install

It is recommended to install the Linux/Mac OS with a shell script.

```terminal
$curl -sS https://lhx98.linkclub.jp/twise.co.jp/download/install.sh | sh
```

Linux/Mac OS can be installed on Homebrew.

```terminal
$brew install twsnmp/tap/twsla
```

Winddows downloads zip files from the release or scoop
Install in.

```terminal
>scoop bucket add twsnmp https://github.com/twsnmp/scoop-bucket
>scoop install twsla
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

You can check the commands that support the Help command.

```
Simple Log Analyzer by TWSNMP

Usage:
  twsla [command]

Available Commands:
  ai          ai command
  anomaly     Anomaly log detection
  completion  Generate the autocompletion script for the specified shell
  count       Count log
  delay       Search for delays in the access log
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
  twsnmp      Get information and logs from TWSNMP FC
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

When the command is illustrated

![](https://assets.st-note.com/img/1731635423-vj6JTY1yz0eEg9l4pdIskRKh.png?width=1200)

### import command

It is a command to import the log.Save in a time series that can be searched in a database.The argument of the command is

```
＄twsla help import
Import log from source
source is file | dir | scp | ssh | twsnmp

Usage:
  twsla import [flags]

Flags:
      --api              TWSNMP FC API Mode
  -c, --command string   SSH Command
  -p, --filePat string   File name pattern
  -h, --help             help for import
      --json             Parse JSON windows evtx
  -k, --key string       SSH Key
  -l, --logType string   TWSNNP FC log type (default "syslog")
      --noDelta          Check delta
      --skip             TWSNMP FC API skip verify certificate (default true)
  -s, --source string    Log source
      --tls              TWSNMP FC API TLS
      --utc              Force UTC

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range

```

Specify the location of the log to read in -s or --source.
In the latest version, you can specify files and directory names with arguments without -s options.
If you specify the file, read only the specified file.This is easy to understand.

If it runs

```terminal
$twsla import -s ~/Downloads/Linux_2k.log

/ Loading path=/Users/ymimacmini/Downloads/Linux_2k.log line=2,000 byte=212 kB
  Total file=1 line=2,000 byte=212 kB time=138.986218ms
```

It displays the number of logs, size, and time it takes.
When you specify the directory, read the file in the directory.If you specify the file pattern in -p or --filePat, you can limit the files in the directory.The design is a simple filter.

```
＄twsla import -s ~/Downloads -p "Linux*"

/ Loading path=/Users/ymimacmini/Downloads/Linux_2k.log line=2,000 byte=212 kB
  Total file=1 line=2,000 byte=212 kB time=75.410115ms
```

 Starting with v1.17.0, the import status display has changed.

![](https://assets.st-note.com/img/1758319263-BqyMKkbUO0PT91w75IvZucgi.png?width=1200)

Displays sparklines.

You can also specify the file name pattern when reading from a zip file or Tar.gz format file.

When reading, you can specify a simple filter, regular expression filter and time range.You can reduce the amount you read.

To read SCP, SSH or TWSNMP log, specify the URL.

`scp://root@192.168.1.210/var/log/messages`

It is a form like.SSH key is required.
Compatible with TWSNMP FC's web API from v1.4.0.
Specify `twsnmp: //192.168.1.250: 8080` in the URL of the -S option
If you specify-API, you can import logs via Web API.
-rogtype can also obtain logs other than syslog.

If you specify --json when reading the EVTX file from v1.1.0, read the Windows event log in JSON format.Detailed information can be displayed.

![](https://assets.st-note.com/img/1717709455800-myzsaGfpvI.png?width=1200)

The log destination of the log is specified in the -d option.BBolt database.If you omit it, it will be TWSLA.db of the current directory.
By specifying --nodelta from v1.8.0, it is possible not to perform the time difference and save the process.This will increase the speed a little.
The speed of IMPORT is faster when the log is lined up in chronological order.A random log is slow.

### search command

You can search when the log is read.

```
twsla  help search
Search logs.
Simple filters, regular expression filters, and exclusion filters can be specified.

Usage:
  twsla search [flags]

Flags:
  -c, --color string   Color mode
  -h, --help           help for search

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

You can narrow down the log by specifying the simple filter, regular expression filter and time range.In the current version, it is an inverted filter when you start with a simple filter with an argument.


```
＄twsla  search -f fail
```

If you search with a feeling like

![](https://assets.st-note.com/img/1716672574781-gcWleWK4jC.png?width=1200)


A key input help is displayed at the top right of the search result screen.
You can save the result with the S key.The display is reversed with the R key.Q key is over.
The search results of the log can be displayed from v1.5.0.
Search the log to specify -c, --color as the option of the sEACH command.For the key

|Key|Descr|
|---|---|
| IP | Color display of IP address |
| Mac | Color display of MAC address |
| Email | Color display of email address |
| URL | Color display of URL |
| Filter | Color display the character string specified in the filter |
| REGEXP/Pattern/Color | Display the character string that matches the regular expression in a specified color |

Can be specified.

Same log

```
twsla search -f Failed -c "regex/user\s+\S+/9,ip,filter"
```

When displayed as specified like

![](https://assets.st-note.com/img/1726436365-hzP1IyTxiYNnQGakBf2pb6uL.png?width=1200)

You can display the color like this.

From v1.6.0, you can specify the color display from the search results screen.
Press the C key to display the input screen.When you press the M key


![](https://assets.st-note.com/img/1729478132-JVbuz3MD1LrvKYPxFHmAnpSg.png?width=1200)

Displays the input screen of the marker.Following a simple filter or regex:, you can specify the regular expression filler and mark the corresponding character string of the log.This is an example of an IP color and a Fail with a marker.

![](https://assets.st-note.com/img/1729484628-MxPyZJRoNU0bqCkeXmh7cAEG.png?width=1200)

### count command

It is a command that ties the number of logs into an hourly basis, or the data in the log is used as a key.

```terminal
＄twsla  help  count
Count the number of logs.
Count logs for each specified period
Number of occurrences of items extracted from the log.
Count normalized logs by pattern
 $twsla count -e normalize
Count word in logs.
 $twsla count -e word
Count json key.
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
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

You can filter in the same way as search.
If the data to be extracted with -e options is specified, the data is aggregated in this data units.If you do not specify, the number of time logs is aggregated.
Time -by -time tabulation is


```terminal
$twsla  count -f fail
```

![](https://assets.st-note.com/img/1717709793390-R450RHfeJN.png?width=1200)


The result is.The time interval is specified in the -I option.If you omit it, it should be set.
The difference time (Delta) from the previous log from v1.1.0 is also displayed.The average interval is also displayed at the top.
You can sort depending on the number of counts with the C key.It is a sort with time with K key.
You can save the result with the S key.If the extension is made to PNG, it will be a graph.

![](https://assets.st-note.com/img/1716674447895-OPrP8zMSUQ.png?width=1200)

You can save the graph of the HTML file by saving the extension from V1.5.0 with HTML.A graph that can be operated interactively.

![](https://assets.st-note.com/img/1716674531194-O7j5QXhIHo.png?width=1200)


The result is.You can also sort this.If you save it on the graph

![](https://assets.st-note.com/img/1716674623362-MkHGX4qUZ2.png?width=1200)

The ratio of TOP10 is the graph like this.

A delay time filter has been added in v1.16.0.

```
--delay <number>
```
Specifying this will cause the delay command to display the delay time specified by the number or higher from the logs.


```
  -q, --timePos int      Specify second time stamp position
      --utc              Force UTC
```
is a mode that detects the time difference between two timestamps in the log,
similar to the delay command.

Translated with DeepL.com (free version)

### extract command

It is a command that removes specific data from the log.

```terminal
$twsla  help extract
Extract data from the log.
Numeric data, IP addresses, MAC addresses, email addresses
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
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

You can specify the same filter as the search.The specification of the data to be extracted is the same as the count command.

```terminal
$twsla  extract -f fail -e ip
```

If you run with a command like

![](https://assets.st-note.com/img/1716674893720-WqYN0wwrvt.png?width=1200)

It will be time series data like this.You can also sort with the key.You can save the results on a graph.

![](https://assets.st-note.com/img/1716675034354-UvMuVYryxl.png?width=1200)


The numerical data is used as a graph, but for items such as IP addresses, the number of the item is graphed.

![](https://assets.st-note.com/img/1736891736-Mg2ahHbtJqSws7KPcUznTvkQ.png?width=1200)

Press the i key while the numerical data is extracted to display the statistical information of the numerical data.

![](https://assets.st-note.com/img/1736891837-3wLoHPGn5ANfEsgDmyqxTKVh.png?width=1200)

Press the s key and save it with CSV.


### tfidf command

Find an unusual log using TF-IDF.

```terminal
＄twsla  help tfidf
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
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

When executed

![](https://assets.st-note.com/img/1716675268711-yeoAjdEYAx.png?width=1200)

The result is like.I have found three rare logs in 2000 cases.
-l can be specified in the values ​​and -c.Because it is for experts
I'm going to write a detail in another article.
From v1.10, you can get a rare top N case with -n.

### anomaly command

It is a command added in v1.1.0.A command that analyzes the log and finds something unusual.

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
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

Specify the mode in -m.TFIDF creates a vector of logs in TF-IDF.SQL, OS, DIR creates log characteristics vectors from the number of keywords that appear in SQL injection, OS injection, etc.Number creates a characteristic vector from the numbers appearing in the log.
You can specify the position of the numerical value with the -e option.

```
start*end
```

If you specify like

```
11:00 start 0.1  0.2 1.4 end
```

Adopted only three of the logs of 0.1 0.2 1.4.

The analysis result

![](https://assets.st-note.com/img/1717710550350-NG6evcVbRm.png?width=1200)

It will be displayed likeThe larger the score, the more abnormal.SQL injection and Walu are effective in analyzing Web server access logs.

### delay command

It is a command added in v1.3.0.This is a command to detect the delay of processing from the Access log.Apache's Access log records the time stamp the time stamp when the HTTP request is accepted.It actually outputs to the log after the process is over and the response is returned.For this, the time stamp of the log may be recorded back and forth.It means that the log of the time before the recorded earlier will be recorded later.Using this reversal phenomenon can detect the delay in processing.It is a delay, such as processing requests and downloading large files.
Transfer your Apache Access log to syslog and record two time stamps.These two or more time stamps may have a time difference between the logs of logs.I also made a mode to detect this.

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
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

If you add 1 or more values ​​to the -q option, it will be a mode that processes two or more timestamps.If -q is omitted or specifying 0, it will be a mode to detect delays using the reversal phenomenon of Access log.


![](https://assets.st-note.com/img/1723064539386-Xo4AG4qm3Y.png?width=1200)


If the delay cannot be detected, nothing will be displayed.
The right end is the delay time.Select a log and press the Enter key to display the log in detail.Sort in order of time with T key.Sort in order of delay in the D key.You can save it to the file with the S key.When the extension is png, the graph image is saved.

![](https://assets.st-note.com/img/1723064799604-VwdzrZ3bSg.png?width=1200)


### twsnmp command

This is a command to link with TWSNMP FC added in v1.4.0.

```terminal
Get information adn logs from TWSNMP FC
[taget] is node | polling | eventlog | syslog | trap |
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
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

Specify the URL of TWSNMP FC linked with --twsnmp.If you have changed your user ID and password, specify it in this URL.
http: // user ID: password@192.168.1.250: 8080, etc.

Acquisition of node list

```terminal
twsla twsnmp node --twsnmp http://192.168.1.253:8081
17ea9e198e6dce8e        eve-ng-ymi.     normal  192.168.13.1
17ea9e1c9574f616        192.168.13.2    low     192.168.13.2    00:50:79:66:68:05(Private)
17ea9f2747b86b64        Switch1 repair  192.168.13.3    50:00:00:01:80:01(Unknown)
17eaa033358f42c5        Switch2 low     192.168.13.4    50:00:00:02:80:01(Unknown)
17eaa11396dcdfa5        Switch3 low     192.168.13.5    50:00:00:03:80:01(Unknown)
17eaa113ae173e88        Switch4 low     192.168.13.6    50:00:00:04:80:01(Unknown)
17eb3bd030fd9f81        Router  low     192.168.1.242   24:FE:9A:07:D2:A9(CyberTAN Technology Inc.)
```

You can do it with a command like.
Basically, it outputs with TAB separation text.You can save it in the file by redirection.
If --jsonout is specified, it will be in JSON format output.I think this is convenient when using it from the program.

### relation command

List the relationship between two or more items in the log line.It can also be output to the intrajected graph.

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
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

The items that can be specified are

|key|descr|
| ---- | ---- |
| IP | IP address |
| Mac | MAC address |
| Email | Email address |
| URL | URL |
| REGEXP/Pattern/| Character string that matches regular expression |


```terminal
$twsla relation  -f Failed -r user "regex/user\s+\S+/" ip
```

With a command like

![](https://assets.st-note.com/img/1726436651-dajM1gPELX8vny6GBW5Yz9b7.png?width=1200)


You can aggregate like this.If you devise a filter and narrow down the number

![](https://assets.st-note.com/img/1726436651-c86jxm75eoIZaDSHNuBr9Cd1.png?width=1200)


You can also output a graph like.S: Specify the extension of the output file of the save command in HTML.

### heatmap command

This is a command for displaying the time when there is a lot of logs on a day or date unit on a heat map.

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
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

If the -w option is specified, it will be calculated on a daily basis.If you do not specify, it is a date unit.

The date unit is

![](https://assets.st-note.com/img/1726436714-pUIb1AKFhWPGuxLV2gelzRJM.png?width=1200)


When the file of the extension HTML is saved

![](https://assets.st-note.com/img/1726436714-pb7ZIGOX6tPBoHY4aChR9Jzk.png?width=1200)

You can save a graph like.
The day of the week is

![](https://assets.st-note.com/img/1726436714-UjtvDC3bVpgRHYa9hK47yfkd.png?width=1200)


### time command

This is a command that analyzes the time difference between logs.It is a command added in v1.6.0.

```terminal
Time analysis

Usage:
  twsla time [flags]

Flags:
  -h, --help   help for time

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range

```

When executed

![](https://assets.st-note.com/img/1729485261-4NeIF2sytM0lxYwTrnHfikQg.png?width=1200)

The time difference from the marked log is Diff.
The time difference from the previous log is Delta.
Once you select, you will display Diff and Delta in an easy -to -understand manner in the second line.
The second line displays the average value (Mean) of Delta, the median (Median), the maximum (mode), and the standard deviation (stddiv).
In this example, you can see that it is log or recorded every 24 hours.
Press the M key to mark the selected log.
When you save it with HTML or PNG, it outputs Delta to the graph.

![](https://assets.st-note.com/img/1729485332-0ES73fO8nqMzcLBZQtsomj19.png?width=1200)

### sigma command

Standard format SIGMA that detects threats from logs

https://sigmahq.io/

Corresponded to.


```terminal
Detect threats using SIGMA rules.
	About SIGAMA
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
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

Specify a saved directory with the SIGMA rule in the S option.The log is assumed in the format saved by JSON.If you want to handle non -JSON logs, you need to extract data with GROK.
Specify the definition of GROCK in -G option.If you do not specify, specify the default definition, if you specify FULL, you will use the all -built definition.If you specify the path to the definition file, read the definition.
The executed GROCK definition is

https://github.com/elastic/go-grok

See, please.
If you define it yourself

```regexp
TEST  from\s+%{IP}
```

Like
Definition <sp> Definition
And
-X Specify the definition name in the option.
Specify the SIGMA configuration file in the -c option.For Windows event rigs

```yaml

title: Sigma config for windows event log
backends:
  - github.com/bradleyjkemp/sigma-go

fieldmappings:
  Image: $.Event.EventData.Image
  CommandLine: $.Event.EventData.CommandLine
  ParentProcessName: $.Event.EventData.ParentProcessName
  NewProcessName:  $.Event.EventData.NewProcessName
  User:  $.Event.EventData.User
  ParentUser:  $.Event.EventData.ParentUser
  Channel:  $.Event.System.Channel
  Computer:  $.Event.System.Computer
  EventID:  $.Event.System.EventID
  Level:  $.Event.System.Level
  Provider.Guid:  $.Event.System.Provider.Guid
  Provider.Name:  $.Event.System.Provider.Name

```

The file of the format is incorporated.If you specify -c Windows, use this definition.The variable name is being converted in the FieldMappings part.
What is written in the Sigma rule is the value of $ .EvenTdata.image in the event log.Specify in josnpath.

When the SIGMA command is executed

![](https://assets.st-note.com/img/1731635833-qlgh6Id4OZj27BNMse8aYSQP.png?width=1200)

The result is displayed.Displays information on the detected SIGMA rule.Press the return key to display a detailed log, including the target log.

![](https://assets.st-note.com/img/1731635833-SWxOoXL9CMaAVirgnBf0RD81.png?width=1200)

If you press the C key, it will be the displayed display for each detected rule.

![](https://assets.st-note.com/img/1731635833-dxpwm309QjiPok5Ss1eMz6NR.png?width=1200)


Displays the graph with the G key or H key.
You can save data and graphs in the file with the S key.

### twlogeye command

Import notify , logs and report from TwLogEye

https://twsnmp.github.io/twlogeye/

https://github.com/twsnmp/twlogeye


```terminal
Import notify,logs and report from twlogeye
twsla twlogeye <target> [<sub target>] [<anomaly report type>]
  taregt: notify | logs | report
        logs sub target: syslog | trap | netflow | winevent
        report sub target: syslog | trap | netflow | winevent | monitor | anomaly
        anomaly report type: syslog | trap | netflow | winevent | monitor | anomaly

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
      --level string       Notfiy level

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

### ai Command

This command analyzes logs in conjunction with LLM.
Significant changes were made in v1.17.0.


![](https://assets.st-note.com/img/1758318692-ujPGHdgEcA40JOQLNhCVz7bU.png?width=1200)



```terminal
AI-powered log analysis
Using environment variable for API key.
 GOOGLE_API_KEY : gemini
 ANTHROPIC_API_KEY : claude
 OPENAI_API_KEY: openai

Usage:
  twsla ai <filter>... [flags]

Flags:
      --aiBaseURL string       AI base URL
      --aiErrorLevels string   Words included in the error level log (default "error,fatal,fail,crit,alert")
      --aiLang string          Language of the response
      --aiModel string         LLM Model name
      --aiProvider string      AI provider(ollama|gemini|openai|claude)
      --aiSampleSize int       Number of sample log to be analyzed by AI (default 50)
      --aiTopNError int        Number of error log patterns to be analyzed by AI (default 10)
      --aiWarnLevels string    Words included in the warning level log (default "warn")
  -h, --help                   help for ai

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

To analyze logs with the ai command, specify the AI (LLM) provider, model, API key, and the filter to search the logs before launching.
The API key is specified via environment variables.
GOOGLE_API_KEY : gemini
ANTHROPIC_API_KEY : claude
OPENAI_API_KEY: openai

No API key is required for Ollama.


```terminal
$twsla ai -aiProvider -aiModel ollama qwen3:latest <Filter>
```
Searching the logs displays a screen like this:

![](https://assets.st-note.com/img/1758318933-VnEzfqCPXT3a9hY0k6KLpy1o.png?width=1200)

A screen like this will appear.

Select a log and press the e key

![](https://assets.st-note.com/img/1758352154-IHNFWpQdTta6fS97AD8e2nGV.png?width=1200)

An AI-generated explanation about the log will be displayed.

Press the a key to see the AI's analysis of the entire searched log.


![](https://assets.st-note.com/img/1758352084-BnFKucxeG4mqSoYCT6tNprH3.png?width=1200)

The AI's response remains in memory until the screen is closed. Pressing the a key or selecting a log and pressing the e key
will immediately display the screen.


### mcp command

MCP server

```terminal
$twsla help mcp
MCP server for AI agent

Usage:
  twsla mcp [flags]

Flags:
      --clients string     IP address of MCP client to be allowed to connect Specify by comma delimiter
      --endpoint string    MCP server endpoint(bind address:port) (default "127.0.0.1:8085")
      --geoip string       geo IP database file
  -h, --help               help for mcp
      --transport string   MCP server transport(stdio/sse/stream) (default "stdio")

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

### System prompt for MCP Server

```
# TWSLA Log Analysis AI - System Prompt

You are an AI assistant for TWSLA (TWSNMP Log Analyzer). Your primary role is to help users analyze logs stored in the TWSLA database. You can search, count, extract data from, and summarize logs.

## Available Tools

You have access to the following tools to interact with the TWSLA log database.

### 1. `search_log`

Use this tool to search for log entries that match specific criteria.

**Parameters:**

*   `filter` (string, optional): A regular expression to filter logs. If empty, no filter is applied.
*   `limit` (integer, optional): The maximum number of log entries to return. (Min: 100, Max: 10000, Default: 100)
*   `start` (string, optional): The start date and time for the search (e.g., "2025/10/26 11:00:00"). Defaults to the beginning of time if empty.
*   `end` (string, optional): The end date and time for the search (e.g., "2025/10/26 12:00:00"). Defaults to the current time if empty.

**Example:**
To search for logs containing the word "error" in the last hour:
`search_log(filter="error", start="-1h")`

### 2. `count_log`

Use this tool to count log entries, grouped by a specific unit. This is useful for statistical analysis.

**Parameters:**

*   `filter` (string, optional): A regular expression to filter logs before counting.
*   `unit` (string, optional): The unit for counting. (Default: "time")
    *   `time`: Group by time interval.
    *   `ip`: Group by source IP address.
    *   `email`: Group by email address.
    *   `mac`: Group by MAC address.
    *   `host`: Group by hostname (requires DNS resolution).
    *   `domain`: Group by domain name.
    *   `country`: Group by country (requires GeoIP database).
    *   `loc`: Group by geographic location (requires GeoIP database).
    *   `word`: Group by individual words in the log message.
    *   `field`: Group by a specific field (space-separated).
    *   `normalize`: Group by normalized log pattern.
*   `unit_pos` (integer, optional): The position of the unit if `unit` is "field". (Default: 1)
*   `top_n` (integer, optional): The number of top results to return. (Default: 10)
*   `interval` (integer, optional): The aggregation interval in seconds when `unit` is "time". (Default: auto)
*   `start` (string, optional): The start time for the search.
*   `end` (string, optional): The end time for the search.

**Example:**
To count the top 10 source IP addresses from the last 24 hours:
`count_log(unit="ip", top_n=10, start="-24h")`

### 3. `extract_data_from_log`

Use this tool to extract specific pieces of information (like IP addresses, email addresses, or custom patterns) from log entries.

**Parameters:**

*   `filter` (string, optional): A regular expression to filter logs before extraction.
*   `pattern` (string, required): The pattern of data to extract.
    *   `ip`, `mac`, `email`, `number`
    *   Or a custom regular expression.
*   `pos` (integer, optional): The position of the data to extract if the pattern finds multiple matches. (Default: 1)
*   `start` (string, optional): The start time for the search.
*   `end` (string, optional): The end time for the search.

**Example:**
To extract all IP addresses from logs containing "failed login" in the last day:
`extract_data_from_log(filter="failed login", pattern="ip", start="-1d")`

### 4. `import_log`

Use this tool to import new logs into the TWSLA database from a file or directory.

**Parameters:**

*   `path` (string, required): The path to the log file or directory. It can handle compressed files like `.zip`, `.tar.gz`, and `.gz`.
*   `pattern` (string, optional): A regular expression to filter filenames within a directory or archive.

**Example:**
To import all `.log` files from the `/var/log/` directory:
`import_log(path="/var/log/", pattern=".*\.log")`

### 5. `get_log_summary`

Use this tool to get a high-level summary of the logs for a given period. The summary includes total entries, error and warning counts, and the top error patterns.

**Parameters:**

*   `filter` (string, optional): A regular expression to filter logs.
*   `top_n` (integer, optional): The number of top error patterns to return. (Default: 10)
*   `start` (string, optional): The start time for the summary.
*   `end` (string, optional): The end time for the summary.

**Example:**
To get a summary of all logs from yesterday:
`get_log_summary(start="-1d", end="today")`

## General Instructions

*   Always analyze the user's request carefully to choose the most appropriate tool.
*   When dealing with time, you can use relative durations (e.g., "-1h", "-24h") or absolute timestamps.
*   Combine tools to answer complex questions. For example, you might first `search_log` to get a sense of the data, then use `count_log` or `extract_data_from_log` for detailed analysis.
*   If a user's request is ambiguous, ask for clarification before executing a tool.

```

### **Server Configuration**
- **Transport**: `stdio` (console), `sse` (server-sent events), or `stream` (HTTP with client filtering).
- **Endpoints**: Default `127.0.0.1:8085`.
- **Clients**: Comma-separated IP whitelist for stream transport.

These tools enable log analysis, data extraction, and database management via the MCP protocol. 

### compression command

It is a command that generates a script to complement commands.
The corresponding shell is

```terminal
  bash        Generate the autocompletion script for bash
  fish        Generate the autocompletion script for fish
  powershell  Generate the autocompletion script for powershell
  zsh         Generate the autocompletion script for zsh
```

In the Bash environment of Linux
/etc/bash_Completion.d/
You can save the script.

```terminal
$twsall completion bash > /etc/bash_completion.d/twsla
```


In Zsh of Mac OS
~/.zsh/compression/
Save the script in.

```terminal
$mkdir -p ~/.zsh/completion/
$twsla completion zsh > ~/.zsh/completion/_twsla
```

after that,
~/.zshrc

```sh:~/.zshrc
fpath=(~/.zsh/completion $fpath)
autoload -Uz compinit && compinit -i
```
I will add.Restart the shell.

```terminal
$exec $SHELL -l
```

The easiest thing is to close the terminal and open it again.

In the case of Windows PowerShell

```terminal
>twsla completion powershell | Out-String | Invoke-Expression
```

It looks good.It seems that you can save TWSLA.PS1 and script file and register with PowerShell profile.

### Version command
Displays the Tesla version.

```terminal
$twsla version
twsla v1.8.0(94cb1ad24408c2dc38f7d178b2d78eaf5f6ad600) 2024-12-15T21:07:47Z
```

## Basic explanation

### Compatible logs

As of 2024/9

- The text file has a time stamp for each line
- Windows EVTX format
- Twsnmp's internal log


is.Text -type files can be read directly in ZIP and Tar.gz.It also supports files that are compressed by GZ.

```
Jun 14 15:16:01 combo sshd(pam_unix)[19939]: authentication failure; logname= uid=0 euid=0 tty=NODEVssh ruser= rhost=218.188.2.4
```

It is a file like.
Time stamps support various formats using magic.In the old syslog, it may be a new format defined by RFC or UNIX time.If you have a number of time stamps, use the leftmost time stamp.
You can read the log file directly from the server in SCP or SSH.
You can also read from TWSNMP FC/FK.

### Simple filter

If you are familiar with regular expression, you can use a regular expression filter, but we have prepared a simple filter for those who do not.It is for me.Specify with LS or DIR command*or? Indicates that there are some strings and characters.
If you write like Message*, it will be a regular expression Message.*.

If you write $, you can also specify it.
When specifying an IP address filter with regular expression,

```
192.168.2.1
```

is not work.

```
192\.168\.\2\.1
```

is OK.

It is troublesome, but the simple filter is used as it is.
Specify with -f for the command option.This is the method of the file name.The regular expression is specified in -R.
Until v1.1.0, the only one for the -f and -r filters was effective, but after v1.2.0, both and later were changed to both and conditions.Because this is more convenient.
In v1.6.0 or later, the filter can be specified by argument.

Starting with **v1.15.0**, keyword support has been added.

| Keyword | Content |
|---|---|
| `#IP` | Includes IP addresses |
| `#MAC` | Includes MAC addresses |
| `#LOCAL_IP` | Includes local IP addresses |
| `#EMAIL` | Includes email addresses |
| `#URL` | Includes URLs |


### exclusion filter

When there is an unnecessary line in the log, you may want to exclude more and more.I attached the same thing as the Grep-V option.This is specified in regular expression.
If the first of the filter specified by the argument is set, it will be an exclusion filter.

### Designation of time range

The specification of the time range is particular about about input.
```
2024/01/01T00:00:00+900-2024/01/02T00:00:00+900
```
It is troublesome to input like every time.
This
```
2024/1/1,1d
```
You can input like that.

Start, period

Start, end

End, period

It supports 3 patterns.
-T option.

Simple specification of data extraction pattern
GROK is famous as a way to extract data from logs, but since it is troublesome to learn, we have made a simple method that can be specified.
Specify with -e options and -p options.
-E is a pattern

|Key|Descr|
|---|---|
| IP | IP address |
| Mac | MAC address |
| Number | Numbers |
| Email | Email address |
| LOC | Location information |
| Country | Country code |
| HOST | Host name |
| Domain | Domain name |

You can specify it as simple.Loc and Country require an IP position information database.-geoip specifies the file.
-P is the position.
Take out what you discovered the second in -p 2.If there are two or more IP addresses, specify the second one.
You can also specify a little more complicated.

```
count=%{number}
```

It is a form like.If you write`%{something}`in a simple filter
Remove only the part of %{something}.Something has Word in addition to the IP and Email.

Data extraction by 

### GROK and JSON

Added data extraction mode by GROK and JSON to the EXTRACT command from v1.70 and the count command.

```terminal
Count the number of logs.
Number of logs per specified time
Number of occurrences of items extracted from the log

Usage:
  twsla count [flags]

Flags:
      --delay int        Delay filter
  -e, --extract string   Extract pattern
      --geoip string     geo IP database file
  -g, --grok string      grok pattern definitions
  -x, --grokPat string   grok pattern
  -h, --help             help for count
  -i, --interval int     Specify the aggregation interval in seconds.
      --ip string        IP info mode(host|domain|loc|country)
  -n, --name string      Name of key (default "Key")
  -p, --pos int          Specify variable location (default 1)
  -q, --timePos int      Specify second time stamp position
      --utc              Force UTC

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
      --sixel              show chart by sixel
  -t, --timeRange string   Time range
```

#### GROK mode

If you specify GROK in the -e option, it will be in GROK mode.In this case, you need to specify the GROK pattern in the -x option.Specify the definition of GROK in the -G option.The same method as the SIGMA command.Specify the data name extracted to -n.

```terminal
$twsla count -x IP -n IP -e grok
```

It feels likePreviously

```terminal
$twsla count -e ip
```

Is almost the same result.But GROK is slower.GROK seems to be used for complicated extraction.

#### JSON mode

Logs saved in JSON format, such as Windows event logs and ZEEK JSON logs, can be extracted with JSONPATH.
Specify JSON in the -e option and specify jsonPath for -n option.


### Save graph
When the result screen of the count or EXTRACT command is executed, the extension is png to save the graph image instead of a text file.

### Display of graphs

The graph can be displayed by typing the G key or H key in the display of the command that can save the graph.If you specify --sixel in the boot parameter from v1.9.0 or specify Twsal_sixel = true as an environment variable, you can display the graph in the terminal.

![](https://assets.st-note.com/production/uploads/images/169827737/picture_pc_df187d1aaa63d79b7546e8eb48156d53.gif?width=1200)


### IP Information (DNS/Geoip) analysis

This is a function that obtains information such as position information such as country, city, latitude and longitude, host name, domain name, etc. from the IP address in the log.
We supported from v1.8.0.

--Geoip specifies the path of the IP position information database.
The database file of IP position information is

Please get it from.

https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/


-IP Specify the type of IP information to be obtained.

|Key|Descr|
|---|---|
| host | Host name |
| domain | Domain name |
| loc | Location information |
| country | Country code |

It corresponds to.Only LOC and Country have an IP position information database.

for example,

```terminal
$twsla count -e ip --ip country --geoip ~/Desktop/GeoLite2-City_20241119/GeoLite2-City.mmdb  Failed password
```

If you aggregate like

![](https://assets.st-note.com/img/1734471673-IFsHxby4QXcP7VWe5Mw9Jg1p.png?width=1200)

You can aggregate like this.It can be tabulated by country, not individual IP addresses.
If you tabulate it with LOC

![](https://assets.st-note.com/img/1734471770-kohDelUswg1B3GfH8YLxTyX0.png?width=1200)

It feels likeIf the latitude and longitude are added and the city name is known, this is also added.
When tied in Domain

![](https://assets.st-note.com/img/1734471721-RzXjkHbfCnN5OegZKVUGPmIs.png?width=1200)

is.It's pretty late because you contact the DNS server.
The target log is the SSH server log downloaded from the log sample site.You can clearly see information about the IP address of the access source that has failed.
The parameter for the EXTRACT command is the same.When the same log is displayed in LOC

![](https://assets.st-note.com/img/1734471801-biraOlZA2QtuzSkchsNLRdU3.png?width=1200)


### Configuration file and environmental variable

V1.9.0 supports configuration files and environment variables.

#### Setting file

Use the file specified in --config or the home directory /.twsla.yaml as the configuration file.
YAML format.It corresponds to the following keys.

| Key | Descr |
| --- | --- |
| timeRange | Time range |
| filter | Simple Filter |
| regex | Regular expression filter |
| not | Inverted filter |
| extract | extraction pattern |
| name | Variable name |
| grokPat ||
| ip | IP Information Mode |
| color | Color Mode |
| Rules | Sigma Rules Pass |
| sigmaconfig | Sigma Settings |
| twsnmp | TWSNMP FC URL |
| interval | Agricultural intervals |
| jsonOut | JSON format output |
| checkCert | Verification of server certificate |
| datastore | Datstore Pass |
| geoip | Geoipdb's path |
| grok | GROK definition |
| sixel | Display in the graph terminal |

#### environmental variables

The following environment variables are available.

| Key | Descr |
| --- | ---- |
| TWSLA_DATASTOTE | Datstore path |
| TWSLA_GEOIP | GEOIP database path |
| TWSLA_GROK | Definition of GROK |
| TWSLA_SIXEL | Use Sixel for graph display |

## Example logs

For those who want to get the sample log used for this explanation

https://github.com/logpai/loghub

This is a log in the Linux folder.


## Build

Use go-task for builds.
https://taskfile.dev/

```terminal
$task
```


## Copyright

see ./LICENSE

```
Copyright 2024 Masayuki Yamai
```
