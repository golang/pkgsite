# Adding new module to modproxy testdata

To add a new module for testing purposes, populate the `./modproxy/modules` directory with appropriate `.go`, `go.mod`, and `LICENSE` files, then update the `for` loop in `defaultTestVersions` in `../test_helper.go`.
