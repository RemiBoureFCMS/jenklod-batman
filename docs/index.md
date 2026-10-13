---
layout: home
title: jenklod-batman
titleTemplate: Jenkins, from the Batcave

hero:
  name: jenklod-batman
  text: Jenkins, from the Batcave.
  tagline: A terminal UI for Jenkins. Watch the jobs you care about, search every folder, approve deploys, and chain builds into macros without opening a browser.
  image:
    src: /logo-mark.png
    alt: A black cat with bat wings in a yellow spotlight, above a terminal prompt
  actions:
    - theme: brand
      text: Get started
      link: /guide/install
    - theme: alt
      text: Keys
      link: /reference/keys
    - theme: alt
      text: GitHub
      link: https://github.com/krank56/jenklod-batman

features:
  - icon: 👁
    title: Watched jobs
    details: The jobs you pin sit at the top of the screen with their last build, a live progress bar and who started it. Build, abort or open the log from there.
    link: /guide/watching
  - icon: 🔎
    title: Search every folder
    details: Press / and type a few letters. Fuzzy matching on the full path finds a job in any folder, however deep, and takes you straight there.
    link: /guide/search
  - icon: 🧰
    title: Macros
    details: Save sequences such as "abort, redeploy to prod, approve, wait for green". Run them in the background from the TUI, or from a script as jk ship v2.8.0, with the version as the macro's argument.
    link: /guide/macros
  - icon: 🔁
    title: Rebuild, pick a branch
    details: R refills the build form from an earlier build. ctrl+g lists the job's GitHub branches for a text field. The new build shows up as queued, then takes its place in the list.
    link: /guide/browsing#rebuilding
  - icon: ⏸
    title: Pipeline inputs
    details: A build waiting on "Deploy to prod?" is flagged in the list and in the log. Press i to proceed or abort.
    link: /guide/inputs
  - icon: 🔔
    title: Desktop notifications
    details: Get a notification when a watched build finishes or stops to ask for input, on macOS or Linux.
    link: /guide/watching#notifications
  - icon: 🔐
    title: Token in the keyring
    details: Your API token goes to Keychain or the Secret Service, never to disk. The config file holds only the URL, your user and your preferences.
    link: /guide/first-run
  - icon: ⬆️
    title: Stays up to date
    details: The header says when a new release is out. jk update installs it the way you installed it, and the first launch after shows what's new.
    link: /guide/install#upgrading
---

<div class="home-demo">

<Demo name="tour" caption="The tour: the splash, the watched jobs, a search across folders, a live log paused on an input step." />

```sh
brew install krank56/tap/jenklod-batman
```

</div>

<style>
.home-demo {
  max-width: 1152px;
  margin: 0 auto;
  padding: 0 24px 64px;
}
@media (min-width: 640px) {
  .home-demo { padding: 0 48px 64px; }
}
@media (min-width: 960px) {
  .home-demo { padding: 0 64px 64px; }
}
</style>
