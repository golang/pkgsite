import marked from 'marked';
import fs from 'fs/promises';

/**
 * parse extracts code snippets from markdown files in component
 * directories for use as html in unit tests.
 * @param file path to a markdown file.
 * @returns code snippet from markdown file suitable to use
 * in static unit tests.
 */
export async function parse(file: string): Promise<string> {
  marked.use({ renderer: { code: code => code } });
  return marked(await fs.readFile(file, { encoding: 'utf-8' }));
}
