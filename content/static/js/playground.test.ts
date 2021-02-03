/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { PlaygroundExampleController } from './playground';
import { mocked } from 'ts-jest/utils';

const flushPromises = () => new Promise(setImmediate);

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

      <pre class="Documentation-exampleCode">package main

      import (
        "fmt"
        "io"
        "os"
      )

      func main() {
        const name, age = "Kim", 22
        s := fmt.Sprintln(name, "is", age, "years old.")

        io.WriteString(os.Stdout, s) <span class="comment">// Ignoring error for simplicity.</span>

      }
      </pre>

      <pre class="Documentation-exampleOutput">Kim is 22 years old.
      </pre>
      </div>
      <div class="Documentation-exampleButtonsContainer">
        <p class="Documentation-exampleError" role="alert" aria-atomic="true"></p>
        <button class="Documentation-examplePlayButton" aria-label="Play Code">Play</button>
      </div></details>
    `;
    example = document.querySelector('.js-exampleContainer') as HTMLDetailsElement;
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

  it('opens playground after pressing play', async () => {
    mocked(window.fetch).mockResolvedValue({
      text: () => Promise.resolve('abcdefg'),
    } as Response);
    document.querySelector<HTMLButtonElement>('[aria-label="Play Code"]').click();
    await flushPromises();
    expect(window.open).toHaveBeenCalledWith('//play.golang.org/p/abcdefg');
  });
});
