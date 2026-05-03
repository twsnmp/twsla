Output images/anomaly.mp4

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 800
Set WindowBar Colorful
Set Theme "Chalk"

# Setup: Build the binary and initialize the DB
Hide
Type "go build -o twsla ." Enter
Type "rm -f test.db" Enter
Show
Type "./twsla import -d test.db testlog/syslog.log" Enter
Sleep 5s

Type "./twsla anomaly -d test.db" Enter
Sleep 5s

# Interact with the UI (scrolling the table)
Type "j" Sleep 500ms
Type "j" Sleep 500ms
Type "j" Sleep 500ms
Type "k" Sleep 500ms
Type "k" Sleep 500ms

# Show different mode (SQL injection mode)
Type "q" Sleep 1s
Type "./twsla anomaly -d test.db -m sql" Enter
Sleep 5s
Type "q"

# Cleanup
Hide
Type "rm -f test.db twsla" Enter
Show
