Output images/count.mp4

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
Sleep 2s

# Run count (default time aggregation)
Type "./twsla count -d test.db" Enter
Sleep 3s

# Scroll down
Type "j" Sleep 500ms
Type "j" Sleep 500ms

# Run count with extraction (extracting process names like sshd, su, kernel)
Type "q" Sleep 1s
Type "./twsla count -d test.db -e ip" Enter
Sleep 3s
# Scroll down
Type "j" Sleep 500ms
Type "j" Sleep 500ms
Type "q"

# Cleanup
Hide
Type "rm -f test.db twsla" Enter
Show
