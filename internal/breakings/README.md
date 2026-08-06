# breakings: Finding breaking changes.

The number and severity of breaking changes between tagged versions of a module
at v1 or higher is a signal of the module's quality. Many and frequent breaking
changes show that the module is unstable, and that the authors aren't fully
considering the impact on users.

Summary of this subtree, by directory:

## syntax

Collection of packages that analyze Go AST and syntax to detect API changes
without full type checking.

### syntax/apidiff

Finds compatible and breaking API changes in Go packages using AST syntax comparisons.

### syntax/internal

Internal shared utilities for `syntax` packages.

### syntax/module

Parses Go modules and packages into AST syntax structures.

### syntax/types

Provides a syntactic approximation of Go semantic types and symbol definitions.

## tri

Defines a three-valued boolean type (`Yes`, `No`, `Maybe`).
