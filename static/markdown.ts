import fs from 'fs';

/**
 * parse extracts code snippets from markdown files in component
 * directories for use as html in unit tests. The result is the
 * concatenation of all fenced code blocks (```...```) in the file,
 * which mirrors the original behavior of feeding the markdown through
 * a marked renderer that returned each code block verbatim.
 *
 * @param file path to a markdown file.
 * @returns code snippets from markdown file suitable to use
 * in static unit tests.
 */
export async function parse(file: string): Promise<string> {
  const content = await fs.promises.readFile(file, { encoding: 'utf-8' });
  const blocks: string[] = [];
  const fence = /```[^\n]*\n([\s\S]*?)```/g;
  let m: RegExpExecArray | null;
  while ((m = fence.exec(content)) !== null) {
    blocks.push(m[1]);
  }
  return blocks.join('\n');
}
