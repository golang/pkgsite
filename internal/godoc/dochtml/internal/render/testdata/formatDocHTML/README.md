# Test files for formatDocHTML.

The files are in txtar format.

Each file must have the following sections:

- doc: the comment doc, an input
- want: the HTML, the return value

By default, "want" is tested with extractLinks set to both true and false.

The following sections are optional:

- want:links: the output when extractLinks = true
- links: the extracted links, one per line
  Each line has text and href separated by a single space.
- decl: A Go declaration to be passed to formatDocHTML. Default is nil.
