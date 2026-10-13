# Macros from a shell: list them, run one with its argument and an override.
Source docs/tapes/settings.tape
Set Height 620
Output docs/public/media/cli.mp4

Hide
Type "clear" Enter
Show
Sleep 500ms
Type "jk run --list" Enter
Sleep 2s
Type "jk ship v2.8.0 ENV=prod -y" Enter
Sleep 26s
Screenshot docs/public/media/cli.png
Sleep 2s
