# Test files for formatDocHTML.

The files are in txtar format.

Each file must have the following sections:

- doc: the comment doc, an input
- want: the HTML, the return value

By default, "want" is tested with extractLinks set to both true and false.

The following sections are optional:

- want:links: the output when extractLinks = true
- links: must be present if want:links is present; the extracted
  links, one per line, each line has text and href separated by a single space.

