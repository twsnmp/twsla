Output images/winevent.mp4

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 800
Set WindowBar Colorful
Set Theme "Chalk"

# Setup: Build the binary and initialize the DB
Hide
Type "go build -o twsla ." Enter
Type "rm -f winevt.db" Enter
Show

# 1. Import Windows Event Log
Type "./twsla import -d winevt.db --logType summary testlog/winevent.evtx" Enter
Sleep 5s

# 2. Search logs
Type "./twsla search -d winevt.db" Enter
Sleep 5s
# Scroll down to see more logs
Type "j" Sleep 500ms
Type "j" Sleep 500ms
Type "j" Sleep 500ms
Type "q" Sleep 1s

# 3. Count by Event ID
Type "./twsla count -d winevt.db -e 'regex/EventID=(\d+)/1' -n EventID" Enter
Sleep 5s
# Scroll through the counts
Type "j" Sleep 500ms
Type "j" Sleep 500ms
Type "q" Sleep 1s

# Cleanup
Hide
Type "rm -f winevt.db twsla" Enter
Show
