Output images/search.mp4

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

# Run search (Searching for authentication failures)
Type "./twsla search -d test.db -f 'authentication failure' -c filter" Enter
Sleep 5s

# Scroll down
Type "j" Sleep 500ms
Type "j" Sleep 500ms
Type "j" Sleep 500ms

# Search for something else (Searching for kernel logs)
Type "q" Sleep 1s
Type "./twsla search -d test.db -f 'kernel' -c filter" Enter
Sleep 5s
Type "q"

# Cleanup
Hide
Type "rm -f test.db twsla" Enter
Show
