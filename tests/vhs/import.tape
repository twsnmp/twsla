Output images/import.mp4

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

# Run import
Type "./twsla import -d test.db testlog/syslog.log" Enter
Sleep 10s

# Run search to show it's imported
Type "./twsla search -d test.db" Enter
Sleep 5s
Type "q" Sleep 1s

# Cleanup
Hide
Type "rm -f test.db twsla" Enter
Show
