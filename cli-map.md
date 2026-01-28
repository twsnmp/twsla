# twsla CLI

## Global Flags
- `--config`: config file (default $HOME/.twsla.yaml)
- `-d, --datastore`: Bblot log db (default "./twsla.db")
- `-t, --timeRange`: Time range
- `-f, --filter`: Simple filter
- `-r, --regex`: Regexp filter
- `-v, --not`: Invert regexp filter
- `--sixel`: show chart by sixel

## Commands

### ai
- `ai <filter>...`: AI-powered log analysis
- Flags
    - `--aiProvider`: AI provider (ollama|gemini|openai|claude)
    - `--aiBaseURL`: AI base URL
    - `--aiModel`: LLM Model name
    - `--aiErrorLevels`: Words included in the error level log (default "error,fatal,fail,crit,alert")
    - `--aiWarnLevels`: Words included in the warning level log (default "warn")
    - `--aiTopNError`: Number of error log patterns to be analyzed by AI (default 10)
    - `--aiSampleSize`: Number of sample log to be analyzed by AI (default 50)
    - `--aiLang`: Language of the response

### anomaly
- `anomaly`: Anomaly log detection
- Flags
    - `-m, --mode`: Detection modes (tfidf|sql|os|dir|walu|number) (default "tfidf")
    - `-e, --extract`: Extract pattern

### count
- `count`: Count log
- Flags
    - `-i, --interval`: Specify the aggregation interval in seconds.
    - `-p, --pos`: Specify variable location (default 1)
    - `--delay`: Delay filter
    - `-e, --extract`: Extract pattern or mode (json,grok,word,normalize)
    - `-n, --name`: Name of key
    - `-x, --grokPat`: grok pattern
    - `-g, --grok`: grok pattern definitions
    - `--geoip`: geo IP database file
    - `--ip`: IP info mode (host|domain|loc|country)
    - `-q, --timePos`: Specify second time stamp position
    - `--utc`: Force UTC

### delay
- `delay`: Search for delays in the access log
- Flags
    - `-q, --timePos`: Specify second time stamp position
    - `--utc`: Force UTC

### email
- `email [search|count]`: Search or count email logs
- Flags
    - `--emailCountBy`: Count by field (default "time")
    - `--checkSPF`: Check SPF

### extract
- `extract`: Extract data from log
- Flags
    - `-e, --extract`: Extract pattern
    - `-p, --pos`: Specify variable location (default 1)
    - `-n, --name`: Name of value
    - `-x, --grokPat`: grok pattern
    - `-g, --grok`: grok pattern definitions
    - `--geoip`: geo IP database file
    - `--ip`: IP info mode (host|domain|loc|country)

### heatmap
- `heatmap`: Command to tally log counts by day of the week and time of day
- Flags
    - `-w, --week`: Week mode

### import
- `import`: Import log from source (file | dir | scp | ssh | twsnmp | imap | pop3)
- Flags
    - `--utc`: Force UTC
    - `-b, --size`: Batch Size (default 10000)
    - `--noDelta`: Disable delta check
    - `--api`: TWSNMP FC API Mode
    - `--tls`: TWSNMP FC API TLS
    - `--skip`: TWSNMP FC API skip verify certificate (default true)
    - `--noTS`: Import no time stamp file
    - `-s, --source`: Log source
    - `-c, --command`: SSH Command
    - `-k, --key`: SSH Key
    - `-p, --filePat`: File name pattern
    - `-l, --logType`: TWSNNP FC log type (default "syslog")
    - `--imapFolder`: List IMAP folder names
    - `--emailTLS`: IMAP use start TLS
    - `--emailUser`: IMAP or POP3 user name
    - `--emailPassword`: IMAP or POP3 password

### mcp
- `mcp`: MCP server for AI agent
- Flags
    - `--transport`: MCP server transport (stdio/sse/stream) (default "stdio")
    - `--endpoint`: MCP server endpoint (default "127.0.0.1:8085")
    - `--clients`: IP address of MCP client to be allowed to connect
    - `--geoip`: geo IP database file

### relation
- `relation <data1> <data2>...`: Relation Analysis
    - data entry: ip | mac | email | url | regex/\<pattern\>/\<color\>

### search
- `search [simple filter...]`: Search logs
- Flags
    - `-c, --color`: Color mode
    - `-w, --wrap`: Wrap or scroll x

### sigma
- `sigma`: Detect threats using SIGMA rules
- Flags
    - `-s, --rules`: Sigma rules path
    - `--strict`: Strict rule check
    - `-c, --sigmaConfig`: config path
    - `-x, --grokPat`: grok pattern if empty json mode
    - `-g, --grok`: grok definitions

### tfidf
- `tfidf`: Log analysis using TF-IDF
- Flags
    - `-l, --limit`: Similarity threshold between logs (default 0.5)
    - `-c, --count`: Number of threshold crossings to exclude
    - `-n, --top`: Top N

### time
- `time`: Time analysis

### twlogeye
- `twlogeye`: Import notify, logs and report from twlogeye
- Arguments
    - target: notify | logs | report
    - sub target: syslog | trap | netflow | winevent | otel | mqtt | monitor | anomaly
- Flags
    - `--apiServer`: twlogeye api server ip address
    - `--apiPort`: twlogeye api port number (default 8081)
    - `--ca`: CA Cert file path
    - `--cert`: Client cert file path
    - `--key`: Client key file path
    - `--filter`: Log search text
    - `--level`: Notfiy level
    - `--anomaly`: Anomaly report type (default "monitor")

### twsnmp
- `twsnmp [target]`: Get information and logs from TWSNMP FC
- Arguments
    - target: node | polling | eventlog | syslog | trap | netflow | ipfix | sflow | sflowCounter | arplog | pollingLog
- Flags
    - `--jsonOut`: output json format
    - `--checkCert`: TWSNMP FC API verify certificate
    - `--twsnmp`: TWSNMP FC URL (default "http://localhost:8080")

### version
- `version`: Show twsla version
- Flags
    - `--color`: Version color
