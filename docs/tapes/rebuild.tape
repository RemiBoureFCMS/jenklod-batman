# Rebuild the running deploy with its parameters: it waits in the queue,
# then takes its place in the list with the cursor on it.
Source docs/tapes/settings.tape
Output docs/public/media/rebuild.mp4

Hide
Type "clear && jenklod-batman" Enter
Sleep 1.5s
Space
Show
Sleep 1.5s
Enter
Sleep 2s
Type "R"
Sleep 1.8s
Screenshot docs/public/media/rebuild-form.png
Sleep 500ms
Left
Sleep 800ms
Enter
Sleep 1s
Type "y"
Sleep 500ms
Screenshot docs/public/media/queued.png
Sleep 3.5s
Screenshot docs/public/media/rebuild-started.png
Sleep 2s
Type "q"
Sleep 300ms
Type "q"
Sleep 300ms
Type "q"
Sleep 500ms
