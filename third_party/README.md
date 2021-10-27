This directory holds non-Go libraries that were not developed
as part of the pkgsite project.

Some of the libraries are used by the frontend UI; others are for testing.

To add a library here, place it in a subdirectory. If the frontend UI needs it,
then add the subdirectory to the `//go:embed` line in `fs.go`.
