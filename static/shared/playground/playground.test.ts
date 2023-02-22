/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { PlaygroundExampleController } from './playground';
import { mocked } from 'ts-jest/utils';

const flushPromises = (ms = 0) => new Promise(fn => setTimeout(fn, ms));
const el = <T extends HTMLElement>(selector: string) => document.querySelector<T>(selector);
const codeSnippet = `package main

import (
  "fmt"
  "io"
  "os"
)

func main() {
  const name, age = "Kim", 22
  s := fmt.Sprintln(  name, "is", age, "years old.")

  io.WriteString( os.Stdout, s)    // Ignoring error for simplicity.

  // HTML special characters: & ' < > "
}
`;

const escapeHTML = (s: string) => {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/'/g, '&#39;')
    .replace(/"/g, '&#34;');
};

describe('PlaygroundExampleController', () => {
  let example: HTMLDetailsElement;
  const snippetWithModeFile = codeSnippet.concat(`
-- go.mod --
module play.ground

require example v1
`);
  const htmlEl = `
  <details tabindex="-1" id="example-Sprintln" class="Documentation-exampleDetails js-exampleContainer">
  <summary class="Documentation-exampleDetailsHeader">Example <a href="#example-Sprintln">¶</a></summary>
  <div class="Documentation-exampleDetailsBody">
  <p>Code:</p>

  <pre class="Documentation-exampleCode">${escapeHTML(codeSnippet)}</pre>

  <pre>
    <span class="Documentation-exampleOutputLabel">Output:</span>
    <span class="Documentation-exampleOutput">Kim is 22 years old.</span>
  </pre>
  </div>
  <div class="Documentation-exampleButtonsContainer">
    <p class="Documentation-exampleError" role="alert" aria-atomic="true"></p>
    <button class="Documentation-exampleShareButton" aria-label="Share Code">Share</button>
    <button class="Documentation-exampleFormatButton" aria-label="Format Code">Format</button>
    <button class="Documentation-exampleRunButton" aria-label="Run Code">Run</button>
  </div></details>
`;
  window.fetch = jest.fn();
  window.open = jest.fn();

  beforeEach(() => {
    document.body.innerHTML = htmlEl.concat(`
    <div class="js-playgroundVars" data-modulepath="example" data-version="v1" hidden></div>
`);
    example = el('.js-exampleContainer') as HTMLDetailsElement;
    new PlaygroundExampleController(example);
  });

  afterEach(() => {
    document.body.innerHTML = '';
    mocked(window.fetch).mockClear();
    mocked(window.open).mockClear();
  });

  it('expands and collapses example when summary is clicked', () => {
    const summary = example.firstElementChild as HTMLDetailsElement;
    summary.click();
    expect(example.open).toBeTruthy();
    summary.click();
    expect(example.open).toBeFalsy();
  });

  it('replaces the pre element with a text area', () => {
    const input = document.querySelector('.Documentation-exampleCode');
    expect(input.tagName).toBe('TEXTAREA');
  });

  it('opens playground after pressing share', async () => {
    mocked(window.fetch).mockResolvedValue({
      text: () => Promise.resolve('abcdefg'),
    } as Response);
    el('[aria-label="Share Code"]').click();
    await flushPromises();

    expect(window.fetch).toHaveBeenCalledWith('/play/share', {
      body: snippetWithModeFile,
      method: 'POST',
    });
    expect(window.open).toHaveBeenCalledWith('https://play.golang.org/p/abcdefg');
  });

  it('replaces textarea with formated code after pressing format', async () => {
    mocked(window.fetch).mockResolvedValue({
      json: () =>
        Promise.resolve({
          Body: '// mocked response',
          Error: '',
        }),
    } as Response);
    el('[aria-label="Format Code"]').click();
    const body = new FormData();
    body.append('body', codeSnippet);
    await flushPromises();

    expect(window.fetch).toHaveBeenCalledWith('/play/fmt', {
      body: body,
      method: 'POST',
    });
    expect(el<HTMLTextAreaElement>('.Documentation-exampleCode').value).toBe('// mocked response');
  });

  it('displays error message after pressing format with invalid code', async () => {
    mocked(window.fetch).mockResolvedValue({
      json: () =>
        Promise.resolve({
          Body: '',
          Error: '// mocked error',
        }),
    } as Response);
    el('[aria-label="Format Code"]').click();
    const body = new FormData();
    body.append('body', codeSnippet);
    await flushPromises();

    expect(window.fetch).toHaveBeenCalledWith('/play/fmt', {
      body: body,
      method: 'POST',
    });
    expect(el<HTMLTextAreaElement>('.Documentation-exampleCode').value).toBe(codeSnippet);
    expect(el('.Documentation-exampleOutput').textContent).toContain('// mocked error');
  });

  it('displays code output after pressing run', async () => {
    mocked(window.fetch).mockResolvedValue({
      json: () =>
        Promise.resolve({
          Events: [
            { Message: '// mocked message 1 ', Kind: 'stdout', Delay: 0 },
            { Message: '// mocked message 2', Kind: 'stdout', Delay: 1 },
          ],
          Errors: null,
        }),
    } as Response);
    el('[aria-label="Run Code"]').click();
    await flushPromises(10);

    expect(window.fetch).toHaveBeenCalledWith('/play/compile', {
      body: JSON.stringify({ body: snippetWithModeFile, version: 2 }),
      method: 'POST',
    });
    expect(el('.Documentation-exampleOutput').textContent).toContain(
      '// mocked message 1 // mocked message 2'
    );
  });

  it('displays error message after pressing run with invalid code', async () => {
    mocked(window.fetch).mockResolvedValue({
      json: () =>
        Promise.resolve({
          Events: null,
          Errors: '// mocked error',
        }),
    } as Response);
    el('[aria-label="Run Code"]').click();
    await flushPromises();

    expect(window.fetch).toHaveBeenCalledWith('/play/compile', {
      body: JSON.stringify({ body: snippetWithModeFile, version: 2 }),
      method: 'POST',
    });
    expect(el('.Documentation-exampleOutput').textContent).toContain('// mocked error');
  });

  it('escapes example output', async () => {
    mocked(window.fetch).mockResolvedValue({
      json: () =>
        Promise.resolve({
          Events: [{ Message: '\u003cinput required\u003e', Kind: 'stdout', Delay: 0 }],
          Errors: null,
        }),
    } as Response);
    el('[aria-label="Run Code"]').click();
    await flushPromises();

    expect(window.fetch).toHaveBeenCalledWith('/play/compile', {
      body: JSON.stringify({ body: snippetWithModeFile, version: 2 }),
      method: 'POST',
    });
    expect(el('.Documentation-exampleOutput').innerHTML).toBe('&lt;input required&gt;');
  });

  it('snippet without mod file for standard package', async () => {
    document.body.innerHTML = htmlEl.concat(`
    <div class="js-playgroundVars" data-modulepath="std" data-version="v1" hidden></div>
`);
    example = el('.js-exampleContainer') as HTMLDetailsElement;
    new PlaygroundExampleController(example);

    mocked(window.fetch).mockResolvedValue({
      json: () =>
        Promise.resolve({
          Events: [{ Message: '\u003cinput required\u003e', Kind: 'stdout', Delay: 0 }],
          Errors: null,
        }),
    } as Response);
    el('[aria-label="Run Code"]').click();
    await flushPromises();

    expect(window.fetch).toHaveBeenCalledWith('/play/compile', {
      body: JSON.stringify({ body: codeSnippet, version: 2 }),
      method: 'POST',
    });
    expect(el('.Documentation-exampleOutput').innerHTML).toBe('&lt;input required&gt;');
  });
});
