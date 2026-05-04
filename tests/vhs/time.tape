Output images/time.mp4

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
Type "./twsla import -d test.db testlog/syslog.log" Enter
Sleep 5s
Show

# Run time analysis
Type "./twsla time -d test.db" Enter
Sleep 5s

# Mark some logs
Type "m" Sleep 500ms
Type "j" Type "m" Sleep 500ms

# Quit
Type "q" Sleep 1s

# Cleanup
Hide
Type "rm -f test.db twsla" Enter
Show
