# Firefox Embed POC

Goals:

* Must at least work on Windows (done)
* Have address bar representing the URL
* Have title bar with the title
* Show favicon

### Building and Running

To build, simply:

    go build

To build on Windows without console, instead do:

    go build -ldflags "-H windowsgui"

But note, without console, you can't see console logs so tweak the logger in main if necessary.