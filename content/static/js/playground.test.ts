/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { PlaygroundExampleController } from './playground';
import { mocked } from 'ts-jest/utils';

const flushPromises = () => new Promise(setImmediate);
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

}
`;

describe('PlaygroundExampleController', () => {
  let example: HTMLDetailsElement;
  window.fetch = jest.fn();
  window.open = jest.fn();

  beforeEach(() => {
    document.body.innerHTML = `
      <details tabindex="-1" id="example-Sprintln" class="Documentation-exampleDetails js-exampleContainer">
      <summary class="Documentation-exampleDetailsHeader">Example <a href="#example-Sprintln">¶</a></summary>
      <div class="Documentation-exampleDetailsBody">
      <p>Code:</p>

      <textarea class="Documentation-exampleCode">${codeSnippet}</textarea>

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

  it('opens playground after pressing share', async () => {
    mocked(window.fetch).mockResolvedValue({
      text: () => Promise.resolve('abcdefg'),
    } as Response);
    el('[aria-label="Share Code"]').click();
    await flushPromises();

    expect(window.fetch).toHaveBeenCalledWith('/play/share', {
      body: codeSnippet,
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

  it('displays code output after pressing run', async () => {
    mocked(window.fetch).mockResolvedValue({
      json: () =>
        Promise.resolve({
          Events: [{ Message: '// mocked response', Kind: 'stdout', Delay: 0 }],
          Error: '',
        }),
    } as Response);
    el('[aria-label="Run Code"]').click();
    await flushPromises();

    expect(window.fetch).toHaveBeenCalledWith('/play/compile', {
      body: JSON.stringify({ body: codeSnippet, version: 2 }),
      method: 'POST',
    });
    expect(el('.Documentation-exampleOutput').textContent).toContain('// mocked response');
  });
});
