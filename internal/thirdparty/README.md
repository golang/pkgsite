# Go Discovery Third Party Packages

This subrepository holds the source for various packages inside
https://go.googlesource.com/go/+/refs/heads/master/src/cmd/go/internal/.

They should not be manually edited.

To download a new package:

```
go run download.go -pkg=<name>
```

To update an existing package to the version at master:

```
go run download.go -pkg=<name> -update
```
