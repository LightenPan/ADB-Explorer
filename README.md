<p align="center">
  <img alt="ADB Explorer Logo" src="icons/Store_icon_2023.png" width="150px" />
  <h1 align="center">ADB Explorer</h1>
</p>


<p align="center">
	<a href="https://hosted.weblate.org/engage/adb-explorer/">
		<img src="https://hosted.weblate.org/widget/adb-explorer/resources/287x66-grey.png" alt="Translation status" /></a>
	<br></br>
	<a title="Platform" target="_blank">
		<img src="https://img.shields.io/badge/Platform-Windows-blue" alt="Platform" /></a>
 	<br></br>
	<a href="https://github.com/Alex4SSB/ADB-Explorer/issues">
		<img alt="Issues" src="https://img.shields.io/github/issues/Alex4SSB/ADB-Explorer" /></a>
	<a href="https://github.com/Alex4SSB/ADB-Explorer/releases">
		<img alt="Downloads" src="https://img.shields.io/github/downloads/Alex4SSB/ADB-Explorer/total" /></a>
	<a href="https://github.com/Alex4SSB/ADB-Explorer/commits">
  		<img alt="GitHub last commit" src="https://img.shields.io/github/last-commit/Alex4SSB/ADB-Explorer?label=Last%20commit"></a>
</p>

An interface to ADB that allows browsing, transferring, and editing of files. The client in this repository is the native Go app under [`go/`](go/).

From `go/`:

```
go run ./cmd/adbexplorer
```



<h2 align="left"/>Security</h2>

For information on reporting security vulnerabilities, please see our [Security Policy](SECURITY.md).

<h2 align="left"/>Translation</h2>
Help with translation is most welcome - please visit our translation project on Weblate: https://hosted.weblate.org/engage/adb-explorer/

<br></br>
<h2 align="left"/>App Files</h2>

The Go client stores `settings.json` in its app data folder.

The settings file contains user app settings as well as other settings not directly accessible.
The file can be edited, but the format must be preserved.
An unrecognized entry will be overwritten.
The file can be deleted to restore app settings to their defaults.
