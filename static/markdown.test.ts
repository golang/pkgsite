/**
 * @license
 * Copyright 2026 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { parse } from './markdown';
import fs from 'fs';

describe('parse', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('extracts single code block', async () => {
    const mockContent = `
# Title

\`\`\`html
<div>Hello</div>
\`\`\`
`;
    jest.spyOn(fs.promises, 'readFile').mockResolvedValue(mockContent);

    const result = await parse('dummy.md');
    expect(result).toBe('<div>Hello</div>\n');
  });

  it('extracts multiple code blocks', async () => {
    const mockContent = `
\`\`\`html
<div>Block 1</div>
\`\`\`
Some text
\`\`\`html
<div>Block 2</div>
\`\`\`
`;
    jest.spyOn(fs.promises, 'readFile').mockResolvedValue(mockContent);

    const result = await parse('dummy.md');
    expect(result).toBe('<div>Block 1</div>\n\n<div>Block 2</div>\n');
  });

  it('returns empty string if no code blocks', async () => {
    const mockContent = `
# Title
No code blocks here.
`;
    jest.spyOn(fs.promises, 'readFile').mockResolvedValue(mockContent);

    const result = await parse('dummy.md');
    expect(result).toBe('');
  });

  it('handles code blocks with language specifier', async () => {
    const mockContent = `
\`\`\`javascript
const x = 1;
\`\`\`
`;
    jest.spyOn(fs.promises, 'readFile').mockResolvedValue(mockContent);

    const result = await parse('dummy.md');
    expect(result).toBe('const x = 1;\n');
  });

  it('documents behavior with nested backticks (current limitation)', async () => {
    const mockContent = `
\`\`\`javascript
const x = \` \`\`\` \`;
\`\`\`
`;
    jest.spyOn(fs.promises, 'readFile').mockResolvedValue(mockContent);

    const result = await parse('dummy.md');
    // It stops at the first \`\`\` inside the string.
    expect(result).toBe('const x = ` ');
  });
});
