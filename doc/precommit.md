### Install pre-commit hook

The `all.bash` script runs quickly enough so that you can use it as a git
pre-commit hook. If you don't already have a pre-commit hook, you can install
it with

```
ln -s -f ../../all.bash .git/hooks/pre-commit
```

To add `all.bash` to an existing pre-commit hook, edit `.git/hooks/pre-commit` and add the line

```
./all.bash
```

You may prefer to run `all.bash` only when you push (which happens when you run
`git codereview mail`). You'll need a small script to discard the arguments
that the pre-push hook is called with. Make this the contents of
`.git/hooks/pre-push`:

```
#!/bin/sh
./all.bash
```

and then

```
chmod +x .git/hooks/pre-push
```

### Running Linters/Formatters/Tests

The `all.bash` script can be used to selectively run actions on the source
(e.g. linters, code formatters, or tests). Run `./all.bash help` to see a list
of supported actions.

Some actions are not run by the default invocation of `./all.bash` that is
executed in the commit hook. Notably, the `prettier` command is not run,
because it has a dependency on nodejs, which is otherwise not needed and which
not all developers have installed on their system.

If you are modifying CSS or Javascript and you do not have docker installed,
install prettier as described at https://prettier.io/docs/en/install.html,
and run `./all.bash prettier` to format your changes before mailing your CL.
