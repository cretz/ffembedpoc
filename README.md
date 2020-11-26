# Firefox Embed POC

This is an example of embedding Firefox into a Qt application. Current screenshot:

![Screenshot](screenshot.png?raw=true)

Goals:

* Must at least work on Windows (done)
* Have address bar representing the URL (done)
* Have tabs with the title (done)
* Show tabs favicon (done)
* Fix focus issues
* Add forward and back buttons

Notes:

* Only works on Windows at the moment
  * Linux support can happen probably quite easily
  * macOS support is unlikely until someone can easily embed a third party native window via Qt
* Uses raw Firefox debugging protocol
* Not built to be robust, just built to serve as an example

### Building and Running

Must have Go installed.

#### Without CGO

Without CGO has a couple of bugs still, but it is a faster build. To build, simply:

    go build -tags qtcgoless

Then the `ffembedpoc` executable will be in this directory. To build on Windows without console, instead do:

    go build -tags qtcgoless -ldflags "-H windowsgui"

But note, without console, you can't see console logs so tweak the logger in main if necessary.

#### With CGO

With CGO, the builds are slower but more items are supported. Make sure the Qt binding is
[installed](https://github.com/therecipe/qt/wiki/Installation) (doesn't necessarily matter whether global or
module-per-project mode).

To build without console:

    qtdeploy -tags qtcgo build desktop .

Then the `ffembedpoc` executable will be in the `deploy/GOOS` directory (where `GOOS` is the OS). Set
`QT_DEBUG_CONSOLE=true` environment variable to build on Windows with the GUI.