import marked from 'marked';
import fs from 'fs/promises';

export async function parse(file: string): Promise<string> {
  marked.use({ renderer: { code: code => code } });
  return marked(await fs.readFile(file, { encoding: 'utf-8' }));
}
