# Pick a GitHub branch for a build parameter with ctrl+g. The demo Jenkins
# also plays GitHub, through JB_GITHUB_API.
Source docs/tapes/settings.tape
Env JB_GITHUB_API "http://127.0.0.1:8765/api/graphql"
Output docs/public/media/branches.mp4

Hide
Type "clear && jenklod-batman" Enter
Sleep 1.5s
Space
Show
Sleep 1.5s
Enter
Sleep 2s
Type "b"
Sleep 1.2s
Tab
Sleep 400ms
Tab
Sleep 600ms
Ctrl+U
Sleep 600ms
Ctrl+G
Sleep 2s
Screenshot docs/public/media/branches.png
Sleep 500ms
Type "sig"
Sleep 1.2s
Screenshot docs/public/media/branches-filtered.png
Sleep 500ms
Enter
Sleep 1.5s
Screenshot docs/public/media/branches-picked.png
Sleep 500ms
Escape
Sleep 600ms
Type "q"
Sleep 300ms
Type "q"
Sleep 300ms
Type "q"
Sleep 500ms
