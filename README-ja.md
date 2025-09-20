# twsla
twslaはTWSNMPシリーズのシンプルログ分析ツールです。
Linux/Mac OS/Windowsで動作します。


## インストール

Linxu/Mac OSはシェルスクリプトでインストールするのがオススメです。

```terminal
$curl -sS https://lhx98.linkclub.jp/twise.co.jp/download/install.sh | sh
```

Linux/Mac OSはhomebrewでもインストールできます。

```terminal
$brew install twsnmp/tap/twsla
```

Winddowsは、リリースからZIPファイルをダウンロードするかscoop
でインストールします。

```terminal
>scoop bucket add twsnmp https://github.com/twsnmp/scoop-bucket
>scoop install twsla
```

## 基本的な使い方

- 作業用のディレクトリを作成します。
- そのディレクトリに移動します。
- ログをimportコマンドでインポートします。
- searchコマンドで検索します。
- 結果をCSVなどの出力できます。

```
~$mkdir test
~$cd test
~$twsla import -s <Log file path>
~$twsla search
```

## コマンドの説明

helpコマンドで対応しているコマンドを確認できます。

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
      --sixel              show chart by sixel
  -t, --timeRange string   Time range

Use "twsla [command] --help" for more information about a command.
```

コマンドを図示すると

![](https://assets.st-note.com/img/1731635423-vj6JTY1yz0eEg9l4pdIskRKh.png?width=1200)

### importコマンド

ログをインポートするためのコマンドです。時系列に検索可能なデータベースに保存します。コマンドの引数は、

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

-sまたは--sourceで読み込むログの場所を指定します。
最新のバージョンでは-sオプションなしでファイルやディレクトリ名を引数で指定できます。
ファイルを指定すれば、指定したファイルだけ読み込みます。これはわかりやすいです。
実行すれば

```terminal
$twsla import -s ~/Downloads/Linux_2k.log

/ Loading path=/Users/ymimacmini/Downloads/Linux_2k.log line=2,000 byte=212 kB
  Total file=1 line=2,000 byte=212 kB time=138.986218ms
```

のような感じで、読み込んだログの件数、サイズ、かかった時間を表示します。
ディレクトリを指定するとディレクトリの中のファイルを読み込みます。-pまたは--filePatで、ファイルのパターンを指定すれば、ディレクトリの中のファイルを限定できます。パターンの指定は、シンプルフィルターです。


```
＄twsla import -s ~/Downloads -p "Linux*"

/ Loading path=/Users/ymimacmini/Downloads/Linux_2k.log line=2,000 byte=212 kB
  Total file=1 line=2,000 byte=212 kB time=75.410115ms
```

v1.17.0からインポートのステータス表示が変わりました。

![](https://assets.st-note.com/img/1758319263-BqyMKkbUO0PT91w75IvZucgi.png?width=1200)

スパークラインを表示します。


ZIPファイルやtar.gz形式のファイルから読み込む場合もファイル名のパターンを指定できます。

読み込む時に、シンプルフィルター、正規表現のフィルターや時間範囲を指定することができます。読み込む量を減らすことができます。

SCP、SSHやTWSNMPのログを読み込むためには、URLを指定します。
`scp://root@192.168.1.210/var/log/messages`
のような形式です。SSHの鍵の登録が必要です。
v1.4.0からTWSNMP FCのWeb　API に対応しました。
-sオプションのURLに`twsnmp://192.168.1.250:8080` と指定して
--apiを指定すれば、Web　API経由でログをインポートできます。
--logTypeでsyslog以外のログも取得可能です。

v1.1.0からevtxファイルを読み込む時に--jsonを指定すれば、WindowsのイベントログをJSON形式で読み込みます。詳しい情報が表示できます。

![](https://assets.st-note.com/img/1717709455800-myzsaGfpvI.png?width=1200)

ログの読み込み先は、-dオプションで指定します。bboltのデータベースです。省略すれば、カレントディレクトリのtwsla.dbになります。
v1.8.0から--noDeltaを指定することで、時間差を取得して保存する処理を行わないようにできます。これで、少し速度アップします。
importの速度は、ログが時系列に並んでいるほうが高速です。タイムスタンプがランダムなログは遅くなります。

### search コマンド

ログの読み込みが終われば、検索できます。

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

シンプルフィルター、正規表現のフィルターや時間範囲を指定してログを絞り込んでいけます。現在のバージョンでは引数でシンプルフィルターと^から始めると反転フィルターです。


```
＄twsla  search -f fail
```

のような感じで検索すると

![](https://assets.st-note.com/img/1716672574781-gcWleWK4jC.png?width=1200)


検索結果の画面の右上にキー入力のヘルプが表示されます。
ｓキーで結果を保存できます。rキーで表示を逆順にします。qキー終了です。
v1.5.0からログの検索結果をカラー表示できるようになっています。
ログを検索するseachコマンドのオプションに-c,--colorを指定します。キーには

|Key|Descr|
|---|---|
|ip|IPアドレスをカラー表示|
|mac|MACアドレスをカラー表示|
|email|メールアドレスをカラー表示|
|url|URLをカラー表示|
|filter|フィルターで指定した文字列をカラー表示|
|regexp/パターン/カラー|正規表現にマッチした文字列を指定した色で表示|

を指定できます。

同じログを
```
twsla search -f Failed -c "regex/user\s+\S+/9,ip,filter"
```

のような指定で表示すると

![](https://assets.st-note.com/img/1726436365-hzP1IyTxiYNnQGakBf2pb6uL.png?width=1200)

のようにカラー表示できます。

v1.6.0からカラー表示の指定を検索結果画面からできるようになっています。
cキーを押すと入力画面が表示さえます。mキーを押すと

![](https://assets.st-note.com/img/1729478132-JVbuz3MD1LrvKYPxFHmAnpSg.png?width=1200)

マーカーの入力画面を表示します。シンプルフィルターかregex:に続けて正規表現フィッルターを指定してログの該当文字列にマークをつけることができます。ipのカラーとfailにマーカーをつけた例です。

![](https://assets.st-note.com/img/1729484628-MxPyZJRoNU0bqCkeXmh7cAEG.png?width=1200)

### countコマンド

ログの件数を時間単位に集計したり、ログの中のデータをキーにして集計したりするコマンドです

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

検索と同じようにフィルターをかけることができます。
-e オプションで抽出するデータを指定した場合、このデータ単位で集計します。指定しない場合は、時間単位のログの数を集計します。
時間単位の集計は、

```terminal
$twsla  count -f fail
```

![](https://assets.st-note.com/img/1717709793390-R450RHfeJN.png?width=1200)


のような結果になります。時間の間隔は-iオプションで指定します。省略すれば、よしなに設定されるはずです。
v1.1.0から前のログからの差分時間(Delta)も表示されます。上部に、平均の間隔も表示されます。
cキーでカウント数によってソートできます。kキーで時間でソートです。
sキーで結果を保存できます。拡張子をpngにすれば、グラフになります。

![](https://assets.st-note.com/img/1716674447895-OPrP8zMSUQ.png?width=1200)

v1.5.0から拡張子をhtmlで保存するとHTMLファイルのグラフを保存できます。インターラクティブに操作できるグラフです。

![](https://assets.st-note.com/img/1716674531194-O7j5QXhIHo.png?width=1200)


のような結果になります。こちらもソートできます。グラフに保存すると

![](https://assets.st-note.com/img/1716674623362-MkHGX4qUZ2.png?width=1200)

のようにTOP10の割合がグラフになります。

v1.16.0から遅延時間のフィルターを追加しました。

```
--delay <数値>
```
を指定するとdelayコマンドで表示される遅延時間が指定の数値以上のログから
集計します。

```
  -q, --timePos int      Specify second time stamp position
      --utc              Force UTC
```
は、delayコマンドと同様にログの中の２つのタイムスタンプの時差を
検知するモードです。


### extractコマンド

ログから特定のデータを取り出すコマンドです。

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

検索と同じフィルターが指定できます。抽出するデータの指定もcountコマンドと同じです。

```terminal
$twsla  extract -f fail -e ip
```

のようなコマンドで実行すると

![](https://assets.st-note.com/img/1716674893720-WqYN0wwrvt.png?width=1200)

のような時系列のデータになります。キーでソートもできます。結果をグラフに保存もできます。

![](https://assets.st-note.com/img/1716675034354-UvMuVYryxl.png?width=1200)


数値データは、そのままグラフにしますが、IPアドレスなどの項目は、項目の番号をグラフにします。

![](https://assets.st-note.com/img/1736891736-Mg2ahHbtJqSws7KPcUznTvkQ.png?width=1200)

のような数値データを抽出した状態でiキーを押すと数値データの統計情報を表示します。

![](https://assets.st-note.com/img/1736891837-3wLoHPGn5ANfEsgDmyqxTKVh.png?width=1200)

sキーを押してCSVで保存することもできます。

### tfidfコマンド

TF-IDFを使って、珍しいログを探します。

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

実行すると

![](https://assets.st-note.com/img/1716675268711-yeoAjdEYAx.png?width=1200)

のような結果になるます。２０００件の中の珍しログ３件を見つけています。
-lでしきい値、-cで許容回数を指定できます。玄人向けなので
詳しいことは別の記事に書くつもりです。
v1.10から-nで珍しい上位N件を取得できるようになりました。

### anomalyコマンド

v1.1.0で追加したコマンドです。ログをAI分析して異常なものを見つけるコマンドです。

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

-mでモードを指定します。tfidfはTF-IDFでログの特徴ベクターを作成します。sql,os,dirは、SQLインジェクション、OSインジェクションなどに登場するキーワードの数からログの特徴ベクターを作成します。Numberは、ログに登場する数値から特徴ベクターを作成します。
-eオプションで数値の位置を指定できます。
start*end
のように指定すると　
11:00 start 0.1  0.2 1.4 end
のようなログの 0.1  0.2 1.4の３つだけ採用します。

分析結果は

![](https://assets.st-note.com/img/1717710550350-NG6evcVbRm.png?width=1200)

のような感じで表示されます。Scoreが大きいほど異常と判断しています。SQLインジェクションやWALUはWebサーバーのアクセスログの分析に効果があります。

### delayコマンド

v1.3.0で追加したコマンドです。Accessログから処理の遅延を検知するためのコマンドです。ApacheのAccessログはHTTPのリクエストを受け付けた時点の時刻をタイムスタンプに記録します。実際にログに出力するのは、処理が終わって応答を返してからです。このためにログのタイムスタンプが前後して記録さる場合があります。先に記録されたものより前の時刻のログが後から記録されるという意味です。この逆転現象を利用すると処理の遅延を検知できます。リクエストの処理や大きなファイルのダウンロードに時間がかかるなどの遅延です。
ApacheのAccessログをSyslogに転送して記録するとタイムスタンプが２つあるログになります。この２つ以上タイムスタンプのあるログの時間差が処理の遅延を表している場合があります。これを検知するモードも作りました。

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

-q のオプションに1以上の値をつけると２つ以上のタイムスタンプを処理するモードになります。-qを省略するか0を指定するとAccessログの逆転現象を利用して遅延を検知するモードになります。


![](https://assets.st-note.com/img/1723064539386-Xo4AG4qm3Y.png?width=1200)


遅延を検知できない場合は、何も表示されません。
右端が遅延時間です。ログを選択してEnterキーを押せばログを詳しく表示します。tキーで時刻順にソートします。dキーで遅延の大きさ順にソートします。sキーでファイルに保存できます。拡張子をpngにするとグラフ画像を保存します。

![](https://assets.st-note.com/img/1723064799604-VwdzrZ3bSg.png?width=1200)


### twsnmpコマンド

v1.4.0で追加したTWSNMP FCと連携するためのコマンドです。

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

--twsnmpで連携するTWSNMP FCのURLを指定します。ユーザーID、パスワードを変更している場合は、このURLで指定します。
http://ユーザーID:パスワード@192.168.1.250:8080などです。
ノードリストの取得は

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

のようなコマンドでできます。
基本的にTAB区切りのテキストで出力します。ファイルにリダイレクトで保存できます。
--jsonOutを指定すれば、JSON形式の出力になります。プログラムから利用する時は、こちらが便利だと思います。

### relationコマンド

ログの行にある２つ以上の項目の関係をリストアップします。有指向グラフに出力することもできます。

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

指定可能な項目は

|key|descr|
| ---- | ---- |
|ip|IPアドレス|
|mac|MACアドレス|
|email|メールアドレス|
|url|URL|
|regexp/パターン/|正規表現にマッチした文字列|

です。

```terminal
$twsla relation  -f Failed -r user "regex/user\s+\S+/" ip
```

のようなコマンドで

![](https://assets.st-note.com/img/1726436651-dajM1gPELX8vny6GBW5Yz9b7.png?width=1200)


のように集計できます。フィルターを工夫して件数を絞れば

![](https://assets.st-note.com/img/1726436651-c86jxm75eoIZaDSHNuBr9Cd1.png?width=1200)


のようなグラフも出力できます。s:Saveコマンドの出力ファイルの拡張子をhtmlに指定します。

### heatmapマップコマンド

曜日または日付単位でログの多い時間帯をヒートマップで表示するためのコマンドです。

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

-w オプションを指定すると曜日単位で集計します。指定しない場合に日付単位です。

日付単位は

![](https://assets.st-note.com/img/1726436714-pUIb1AKFhWPGuxLV2gelzRJM.png?width=1200)


拡張子htmlのファイルの保存すると

![](https://assets.st-note.com/img/1726436714-pb7ZIGOX6tPBoHY4aChR9Jzk.png?width=1200)

のようなグラフを保存できます。
曜日単位は

![](https://assets.st-note.com/img/1726436714-UjtvDC3bVpgRHYa9hK47yfkd.png?width=1200)

です。

### timeコマンド

ログ間の時間差を分析するコマンドです。v1.6.0で追加したコマンドです。

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

実行すると

![](https://assets.st-note.com/img/1729485261-4NeIF2sytM0lxYwTrnHfikQg.png?width=1200)

マークしたログとの時間差がDiffです。
前のログとの時間差がDeltaです。
選択すると２行目にDiffとDeltaを人間がわかりやすい形式で表示します。
また、２行目にはDeltaの平均値(Mean)、中央値(Median)、最頻値(Mode)、標準偏差(StdDiv)を表示します。
この例だと、約24時間毎にログか記録されていることろがわかります。
mキーを押すと選択したログにマークをつけます。
htmlまたは、pngで保存すると Deltaをグラフに出力します。

![](https://assets.st-note.com/img/1729485332-0ES73fO8nqMzcLBZQtsomj19.png?width=1200)

### sigmaコマンド

脅威検知の標準フォーマットsigma

https://sigmahq.io/

による検査を行うコマンドです。


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

sオプションでsigmaルールの保存されたディレクトリを指定してください。ログはjsonで保存された形式を前提としています。jsonではないログを扱う場合は、grokでデータを抽出する必要があります。
-gオプションでgrockの定義を指定します。指定しなければデフォルト定義、fullを指定すれば、全組み込み定義を利用します。定義ファイルへのパスを指定すれば、定義を読み込みます。
組み込みのgrock定義は

https://github.com/elastic/go-grok

を参照してください。
自分で定義する場合は、

```regexp
TEST  from\s+%{IP}
```

のように、
定義名<SP>定義
とします。
-xオプションで定義名を指定します。
-c オプションでsigmaの設定ファイルを指定します。windowsのイベントリグ用に

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

という形式のファイルを組み込んであります。-c windowsと指定すれば、この定義を利用します。fieldmappingsの部分で変数名を変換しています。
sigmaルールの中でImageと書いたものは、イベントログの$.Event.EventData.Imageの値になるという設定です。josnpathで指定します。

sigmaコマンドを実行すると

![](https://assets.st-note.com/img/1731635833-qlgh6Id4OZj27BNMse8aYSQP.png?width=1200)

のような結果表示になります。検知したsigmaルールの情報を表示します。リターンキーを押せば、対象のログを含む詳しいログを表示します。

![](https://assets.st-note.com/img/1731635833-SWxOoXL9CMaAVirgnBf0RD81.png?width=1200)

cキーを押せば、検知したルール毎に集計した表示になります。

![](https://assets.st-note.com/img/1731635833-dxpwm309QjiPok5Ss1eMz6NR.png?width=1200)


gキーまたはhキーでグラフを表示します。
ｓキーでデータやグラフをファイルに保存できます。

### twlogeyeコマンド

TwLogEye

https://github.com/twsnmp/twlogeye

https://twsnmp.github.io/twlogeye/

というログサーバーからgRPCで脅威検知通知やログをインポートします。

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

### ai コマンド

LLMと連携してログを分析するためのコマンドです。
v1.17.0で大きく変更しました。

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

aiコマンドでログを分析するためには、AI(LLM）のプロバイダー、モデル、APIキー
とログを検索するフィルターを指定して起動します。
APIキーは、環境変数で指定します。
 GOOGLE_API_KEY : gemini
 ANTHROPIC_API_KEY : claude
 OPENAI_API_KEY: openai

Ollamaの場合はAPIキーは必要ありません。


```terminal
$twsla ai -aiProvider -aiModel ollama qwen3:latest <Filter>
```
ログ検索して

![](https://assets.st-note.com/img/1758318933-VnEzfqCPXT3a9hY0k6KLpy1o.png?width=1200)

のような画面が表示されます。

ログを選択してeキーを押すと

![](https://assets.st-note.com/img/1758351613-VDajhwiCFATymgfnWGIYv67S.png?width=1200)

AIによるログに関する説明が表示されます。

aキーを押すと検索したログ全体に関するAIの分析が表示されます。


![](https://assets.st-note.com/img/1758351831-hxMeayErWLgRJ2CUduSXDcbA.png?width=1200)

AIの回答は画面を閉じるまで記憶しています。aキーやログを選択してeキーを
押した時に、即座に画面が表示されます。

### mcp コマンド

MCPサーバー

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

### MCP Serverツールの仕様

---

#### **1. `search_log` ツール**
**目的**: TWSLAのDBからログを検索する

**パラメータ**:
- `filter_log_content` (string): 正規表現のフィルター、空欄はフィルターなし.
- `limit_log_count` (number): ログの最大数 (100-10,000).
- `time_range` (string): ログの時間範囲 (e.g., `"2025/05/07 05:59:00,1h"` or `"start,end"`).

**出力**: ログの配列をJSON形式で

---

#### **2. `count_log` ツール**
**目的**: 指定の項目でログの件数を集計する (time, IP, domain, etc.).

**パラメータ**:
- `count_unit` (enum): 
  - `time` (指定の時間間隔で集計),
  - `ip`/`mac`/`email` (アドレス単位に集計),
  - `host`/`domain` (DNSのホスト名で集計),
  - `country`/`loc` (geo IPにより位置情報を取得して集計),
  - `normalize` (ログのパターンで集計).
- `time_range` (string): 集計するログの時間範囲.
- `top_n` (number): トップN件の集計.

**Output**: 集計結果をJSON形式で出漁 (例: IPアドレス別の件数).

---

#### **3. `extract_data_from_log` ツール**
**目的**: IPやメールアドレスなどのデータをログから抽出する

**パラメータ**:
- `extract_pattern` (string): 抽出項目、正規表現 (例: `ip=([0-9.]+)`).
- `time_range` (string): ログの時間範囲
- `pos` (number): 抽出する項目の位置

**出力**: 抽出したデータと日時の配列をJSON形式で出力

---

#### **4. `import_log` ツール**
**目的**: Import logs into the TWSLA database from files/directories.

**パラメータ**:
- `log_path` (string): File/directory path (supports ZIP, TAR, EVTX formats).
- `filename_pattern` (string): Regex to filter files.

**出力**: インポートしたログのサマリー (ファイル数, ライン数, バイト数).

---

#### **5. `get_log_summary` ツール**
**目的**: 指定時間範囲のログのサマリーを取得

**パラメータ**:
- `time_range` (string): 時間範囲
- `error_words`/`warning_words` (strings):  エラー、ワーニングを判断するキーワード
- `error_top_n` (number): トップNエラーパターンの件数

**出力**: ログの総数、エラー数、ワーニング数と上位エラーログパターンのリストをJSONで出力

---

### **MCP サーバーの設定**
- **トランスポート**: `stdio` (console), `sse` (server-sent events), or `stream` (HTTP with client filtering).
- **エンドポイント**: Default `127.0.0.1:8085`.
- **クライアント**:  IPのホワイトリストをカンマ区切りで指定.


### completionコマンド

コマンドの補完をするためのスクリプトを生成するコマンドです。
対応しているシェルは、

```terminal
  bash        Generate the autocompletion script for bash
  fish        Generate the autocompletion script for fish
  powershell  Generate the autocompletion script for powershell
  zsh         Generate the autocompletion script for zsh
```

Linuxのbash環境では
/etc/bash_completion.d/
にスクリプトを保存すればよいです。

```terminal
$twsall completion bash > /etc/bash_completion.d/twsla
```

です。
Mac OSのzshでは、
~/.zsh/completion/
にスクリプトを保存します。

```terminal
$mkdir -p ~/.zsh/completion/
$twsla completion zsh > ~/.zsh/completion/_twsla
```

その後、
~/.zshrcに


```sh:~/.zshrc
fpath=(~/.zsh/completion $fpath)
autoload -Uz compinit && compinit -i
```
を追加します。シェルを再起動します。

```terminal
$exec $SHELL -l
```

か、簡単なのは、ターミナルを閉じてもう一度開けばよいです。

WindowsのPowerShellの場合は、

```terminal
>twsla completion powershell | Out-String | Invoke-Expression
```

でよいみたいです。twsla.ps1とスクリプトファイルの保存して、PowerShellのプロファイルに登録すればよいらしいです。

### verisonコマンド

TWSLAのバージョンを表示します。

```terminal
$twsla version
twsla v1.8.0(94cb1ad24408c2dc38f7d178b2d78eaf5f6ad600) 2024-12-15T21:07:47Z
```

## 補足説明

### 対応しているログ
2024/9時点では

- テキストファイルで１行毎にタイムスタンプがあるもの
- Windowsのevtx形式
- TWSNMP FCの内部ログ


です。テキスト形式のファイルはZIPやtar.gzの中にあっても直接読み込めます。gzで圧縮されていてるファイルにも対応しています。

```
Jun 14 15:16:01 combo sshd(pam_unix)[19939]: authentication failure; logname= uid=0 euid=0 tty=NODEVssh ruser= rhost=218.188.2.4
```

のようなファイルです。
タイムスタンプは、魔法を使っていろんな形式に対応しています。昔のsyslogでもRFCで定義されている新しい形式でも、UNIXタイムの数値でもよいです。いくつもタイムスタンプがある場合は、一番左側にあるタイムスタンプを採用します。
SCPやSSHでサーバーから直接ログファイルを読み込むことができます。
TWSNMP FC/FKから読み込むこともできます。

### シンプルフィルター

正規表現に精通しているなら正規表現のフィルターを使えばよいのですが、そうでない人のためにシンプルフィルターを用意しました。私のためでもあります。lsやdirコマンドで指定する*や?で、何か文字列や文字があることを示します。
Message*のように書けば、正規表現のMessage.*になるようなものです。

 $を書けば、そこで終わりという指定もできます。
正規表現でIPアドレスのフィルターを指定する時は、

```
192.168.2.1
```

ではだめで

```
192\.168\.\2\.1
```

のような面倒なことになりますが、シンプルフィルターは、そのままかけます。
コマンドのオプションで-fで指定します。ファイル名のパータンも、この方法です。正規表現は-rで指定します。
v1.1.0までは、-fと-rのフィルターはどちらか片方だけが有効な仕様でしたがv1.2.0以降は、両方のAND条件に変更しました。このほうが便利なので。
v1.6.0以降では、フィルターを引数で複数指定可能にしました。

v1.15.0以降では、キーワードに対応しました。

|キーワード|内容|
|---|---|
|#IP|IPアドレスを含む|
|#MAC|MACアドレスを含む|
|#LOCAL_IP|ローカルIPアドレスを含む|
|#EMAIL|メールアドレスを含む|
|#URL|URLを含む|

### 除外フィルター

ログの中に不要な行がある時に、どんどん除外したい場合があります。grep の-vオプションと同じものをつけました。こちらは正規表現で指定します。
引数で指定するフィルターの先頭を^にすると除外フィルターになります。

### アバウトな時間範囲の指定

時間範囲の指定は、アバウトな入力にこだわっています。
```
2024/01/01T00:00:00+900-2024/01/02T00:00:00+900
```
のような入力を毎回するのは面倒です。
これを
```
2024/1/1,1d
```
のような感じで入力できます。

開始,期間

開始,終了

終了,期間

の３パターンに対応しています。
-tオプションです。

データ抽出パターンの簡易な指定
ログからデータを抽出する方法としてはGROKが有名ですが、覚えるのが面倒なので、簡易に指定できる方法をあみだしました。
-e オプションと-pオプションで指定します。
-eは、パターンで

|Key|Descr|
|---|---|
|ip|IPアドレス|
|mac|MACアドレス|
|number|数値|
|email|メールアドレス|
|loc|位置情報|
|country|国コード|
|host|ホスト名|
|domain|ドメイン名|

のように簡易な指定できます。locとcountryは、IP位置情報データベースが必要です。--geoip でファイルを指定します。
-pは位置です。
-p 2で２番目に発見したものを取り出します。IPアドレスが２つ以上ある場合に２番目のものを指定するとかです。
もう少し複雑な指定もできます。

```
count=%{number}
```

のような形式です。シンプルフィルターの中に`%{何か}`のように書けば
%{何か}の部分だけ取り出します。何かは、先程のipやemailの他にwordがあります。

### grokとjsonによるデータ抽出

v1.70からextractコマンド、countコマンドにgrokとjsonによるデータ抽出モードを追加しました。

```terminal
Count the number of logs.
Number of logs per specified time
Number of occurrences of items extracted from the log

Usage:
  twsla count [flags]

Flags:
  -e, --extract string   Extract pattern
  -g, --grok string      grok pattern definitions
  -x, --grokPat string   grok pattern
  -h, --help             help for count
  -i, --interval int     Specify the aggregation interval in seconds.
  -n, --name string      Name of key (default "Key")
  -p, --pos int          Specify variable location (default 1)

Global Flags:
      --config string      config file (default is $HOME/.twsla.yaml)
  -d, --datastore string   Bblot log db (default "./twsla.db")
  -f, --filter string      Simple filter
  -v, --not string         Invert regexp filter
  -r, --regex string       Regexp filter
  -t, --timeRange string   Time range
```

#### GROKモード

-e オプションにgrokを指定するとgrokモードになります。この場合、-xオプションにgrokのパターンを指定する必要があります。-gオプションでgrokの定義を指定します。sigmaコマンドと同じ方法です。-nに抽出するデータ名を指定します。

```terminal
$twsla count -x IP -n IP -e grok
```

のような感じです。以前からある


```terminal
$twsla count -e ip
```

をほぼ同じ結果になります。でもgrokのほうが遅いです。grokは複雑な抽出に使ったほうがようです。

#### JSONモード

WindowsのイベントログやzeekのjsonログなどJSON形式で保存されたログは、JSONPATHで抽出できます。
-e オプションにjsonを指定して-nオプションにJSONPATHを指定します。


### グラフの保存
countやextractコマンドの結果画面が保存を実行する時に拡張子をpngにすれば、結果をテキストファイルではなくグラフ画像を保存します。

### グラフの表示

グラフを保存できるコマンドの表示中のgキーまたは、hキーをタイプするとグラフを表示できます。v1.9.0から起動パラメータに--sixelを指定するか環境変数にTWSAL_SIXEL=trueを指定すると、Sixelを使ってターミナル内にグラフを表示できまます。

![](https://assets.st-note.com/production/uploads/images/169827737/picture_pc_df187d1aaa63d79b7546e8eb48156d53.gif?width=1200)


### IP情報(DNS/GeoIP)の分析

ログの中のIPアドレスから国、都市、緯度経度などの位置情報、ホスト名、ドメイン名などの情報を取得して集計する機能です。
v1.8.0から対応しました。

--geoipでIP位置情報データベースのパスを指定します。
IP位置情報のデータベースファイルは

https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/

から入手してください。

--ip 取得するIP情報の種類を指定します。

|Key|Descr|
|---|---|
|host|ホスト名|
|domain|ドメイン名|
|loc|位置情報|
|country|国コード|


に対応しています。locとcountryだけIP位置情報データベースが必須になります。

例えば、
```terminal
$twsla count -e ip --ip country --geoip ~/Desktop/GeoLite2-City_20241119/GeoLite2-City.mmdb  Failed password
```

のように集計すると

![](https://assets.st-note.com/img/1734471673-IFsHxby4QXcP7VWe5Mw9Jg1p.png?width=1200)

のように集計できます。個々のIPアドレスではなく国別に集計できます。
locで集計すると

![](https://assets.st-note.com/img/1734471770-kohDelUswg1B3GfH8YLxTyX0.png?width=1200)

のような感じです。緯度経度が追加されて、都市名がわかる場合には、これも追加します。
domainで集計すると

![](https://assets.st-note.com/img/1734471721-RzXjkHbfCnN5OegZKVUGPmIs.png?width=1200)

です。DNSサーバーへ問い合わせるので、かなり遅いです。
対象のログは、ログのサンプルサイトからダウンロードしたSSHサーバーのログです。ログイン失敗しているアクセス元のIPアドレスに関する情報がよくわかります。
extractコマンドもパラメータは同じです。同じログをlocで表示すると

![](https://assets.st-note.com/img/1734471801-biraOlZA2QtuzSkchsNLRdU3.png?width=1200)

### 設定ファイルと環境変数

v1.9.0から設定ファイルと環境変数に対応しました。

#### 設定ファイル

--configで指定したファイルか、ホームディレクトリ/.twsla.yamlを設定ファイルとして使用します。
yaml形式です。以下のキーに対応しています。

|Key|Descr|
|---|---|
|timeRange|時間範囲|
|filter|シンプルフィルター|
|regex|正規表現フィルター|
|not|反転フィルター|
|extract|抽出パターン|
|name|変数名|
|grokPat||
|ip|IP情報モード|
|color|カラーモード|
|rules|Sigmaルールパス|
|sigmaConfig|Sigma設定|
|twsnmp|TWSNMP FCのURL|
|interval|集計間隔|
|jsonOut|JSON形式の出力|
|checkCert|サーバー証明書の検証|
|datastore|データストアのパス|
|geoip|GeoIPDBのパス|
|grok|GROK定義|
|sixel|グラフのターミナル内に表示|

#### 環境変数

以下の環境変数が利用できます。

|Key|Descr|
|---|----|
|TWSLA_DATASTOTE|データストアのパス|
|TWSLA_GEOIP|GeoIPデータベースのパス|
|TWSLA_GROK|GROKの定義|
|TWSLA_SIXEL|グラフ表示にSixelを利用|


## 説明に使ったログの入手

この説明に使ったサンプルのログを手に入れたい人は

https://github.com/logpai/loghub

のLinuxのフォルダにあるログです。


## ビルド方法

ビルドには

https://taskfile.dev/

を利用します。

```terminal
$task
```


## Copyright

see ./LICENSE

```
Copyright 2024 Masayuki Yamai
```
