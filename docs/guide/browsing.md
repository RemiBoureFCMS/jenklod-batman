# Browsing & builds

The root screen lists your [watched jobs](./watching) first, then the top-level folders and jobs. `▸` marks a folder. A coloured dot shows a job's last result, and a spinner means it's building.

| Key | Action |
|---|---|
| <kbd>enter</kbd> <kbd>l</kbd> <kbd>→</kbd> | Open a folder, then a job's builds, then a build's log |
| <kbd>h</kbd> <kbd>←</kbd> <kbd>esc</kbd> | Back |
| <kbd>j</kbd>/<kbd>k</kbd>, <kbd>g</kbd>/<kbd>G</kbd>, <kbd>ctrl+d</kbd>/<kbd>ctrl+u</kbd> | Move |
| <kbd>r</kbd> | Refresh |
| <kbd>o</kbd> | Open in the browser: the selected job or folder, or from a job's builds or a build's log, the job's page |
| <kbd>O</kbd> | From a job's builds or a build's log: open that build, or its console, in the browser |

## Builds

A job's screen lists its last 30 builds with result, start time, duration and cause. A running build shows a progress bar against Jenkins' estimate.

The list keeps itself up to date while a build is queued or running: every 2 seconds while one waits in the queue, every 5 while one runs. A queued build has no number yet, so it shows as a `QUEUED` row with Jenkins' reason (a quiet period, no free executor…) until it starts. As new builds arrive, the cursor stays on the build it was on.

![The builds of a job: one running, the rest finished](/media/builds.png)

- <kbd>b</kbd> starts a build. It always asks y/n. For a job with parameters, the form opens first: <kbd>tab</kbd> moves between fields, <kbd>←</kbd>/<kbd>→</kbd> or <kbd>space</kbd> change choices and checkboxes, and <kbd>enter</kbd> builds. The cursor moves to the new build once it starts, unless you've moved it meanwhile. If you've left the screen, the status bar says when it starts, or that it was cancelled.
- <kbd>x</kbd> aborts the selected build if it's running, after a y/n.
- <kbd>enter</kbd> opens the build's console log.

From the jobs list, you don't have to open a job first: <kbd>b</kbd> builds the selected job, <kbd>R</kbd> rebuilds its last build, <kbd>L</kbd> opens its last build's log, and <kbd>x</kbd> aborts its last build.

### Rebuilding

<kbd>R</kbd> opens the build form filled in with the parameters an earlier build ran with. It uses the selected build in the builds list, the build you're reading in a log, or the job's last build in the jobs list. Change anything you like, then <kbd>enter</kbd> asks y/n as usual.

If the job's parameters have changed since, values that still fit are kept and the rest start at their default. The status bar lists what was dropped, reset or new. A job without parameters just asks y/n, like <kbd>b</kbd>. Rebuilt from a log, the screen switches to the new build's log once it starts.

Jobs with a password or file parameter can't be rebuilt, because Jenkins doesn't hand those values back. Use <kbd>b</kbd> instead.

## Logs

The log follows new output while the build runs (<kbd>f</kbd> toggles following). <kbd>g</kbd>/<kbd>G</kbd> jump to the top or bottom. Jenkins' hidden console annotations are stripped.

![A build log following a pipeline that is waiting for input](/media/log.png)
