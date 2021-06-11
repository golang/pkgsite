/*!
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

// This file implements the playground implementation of the documentation
// page. The playground involves a "play" button that allows you to open up
// a new link to play.golang.org using the example code.

// The CSS is in content/static/css/stylesheet.css.

/**
 * CSS classes used by PlaygroundExampleController
 */
const PlayExampleClassName = {
  PLAY_HREF: '.js-exampleHref',
  PLAY_CONTAINER: '.js-exampleContainer',
  EXAMPLE_INPUT: '.Documentation-exampleCode',
  EXAMPLE_OUTPUT: '.Documentation-exampleOutput',
  EXAMPLE_ERROR: '.Documentation-exampleError',
  PLAY_BUTTON: '.Documentation-examplePlayButton',
  SHARE_BUTTON: '.Documentation-exampleShareButton',
  FORMAT_BUTTON: '.Documentation-exampleFormatButton',
  RUN_BUTTON: '.Documentation-exampleRunButton',
};

/**
 * This controller enables playground examples to expand their dropdown or
 * generate shareable Go Playground URLs.
 */
export class PlaygroundExampleController {
  /**
   * The anchor tag used to identify the container with an example href.
   * There is only one in an example container div.
   */
  private readonly anchorEl: HTMLAnchorElement | null;

  /**
   * The error element
   */
  private readonly errorEl: Element | null;

  /**
   * Buttons that redirect to an example's playground, this element
   * only exists in executable examples.
   */
  private readonly playButtonEl: Element | null;
  private readonly shareButtonEl: Element | null;

  /**
   * Button that formats the code in an example's playground.
   */
  private readonly formatButtonEl: Element | null;

  /**
   * Button that runs the code in an example's playground, this element
   * only exists in executable examples.
   */
  private readonly runButtonEl: Element | null;

  /**
   * The executable code of an example.
   */
  private readonly inputEl: HTMLTextAreaElement | null;

  /**
   * The output of the given example code. This only exists if the
   * author of the package provides an output for this example.
   */
  private readonly outputEl: Element | null;

  /**
   * @param exampleEl The div that contains playground content for the given example.
   */
  constructor(private readonly exampleEl: HTMLDetailsElement) {
    this.exampleEl = exampleEl;
    this.anchorEl = exampleEl.querySelector('a');
    this.errorEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_ERROR);
    this.playButtonEl = exampleEl.querySelector(PlayExampleClassName.PLAY_BUTTON);
    this.shareButtonEl = exampleEl.querySelector(PlayExampleClassName.SHARE_BUTTON);
    this.formatButtonEl = exampleEl.querySelector(PlayExampleClassName.FORMAT_BUTTON);
    this.runButtonEl = exampleEl.querySelector(PlayExampleClassName.RUN_BUTTON);
    this.inputEl = this.makeTextArea(exampleEl.querySelector(PlayExampleClassName.EXAMPLE_INPUT));
    this.outputEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_OUTPUT);

    // This is legacy listener to be replaced the listener for shareButtonEl.
    this.playButtonEl?.addEventListener('click', () => this.handleShareButtonClick());
    this.shareButtonEl?.addEventListener('click', () => this.handleShareButtonClick());
    this.formatButtonEl?.addEventListener('click', () => this.handleFormatButtonClick());
    this.runButtonEl?.addEventListener('click', () => this.handleRunButtonClick());

    if (!this.inputEl) return;

    this.resize();
    this.inputEl.addEventListener('keyup', () => this.resize());
    this.inputEl.addEventListener('keydown', e => this.onKeydown(e));
  }

  /**
   * Replace the pre element with a textarea. The examples are initially rendered
   * as pre elements so they're fully visible when JS is disabled.
   */
  makeTextArea(el: Element | null): HTMLTextAreaElement {
    const t = document.createElement('textarea');
    t.classList.add('Documentation-exampleCode');
    t.spellcheck = false;
    t.value = el?.textContent ?? '';
    el?.parentElement?.replaceChild(t, el);
    return t;
  }

  /**
   * Retrieve the hash value of the anchor element.
   */
  getAnchorHash(): string | undefined {
    return this.anchorEl?.hash;
  }

  /**
   * Expands the current playground example.
   */
  expand(): void {
    this.exampleEl.open = true;
  }

  /**
   * Resizes the input element to accomodate the amount of text present.
   */
  private resize(): void {
    if (this.inputEl?.value) {
      const numLineBreaks = (this.inputEl.value.match(/\n/g) || []).length;
      // min-height + lines x line-height + padding + border
      this.inputEl.style.height = `${(20 + numLineBreaks * 20 + 12 + 2) / 16}rem`;
    }
  }

  /**
   * Handler to override keyboard behavior in the playground's
   * textarea element.
   *
   * Tab key inserts tabs into the example playground instead of
   * switching to the next interactive element.
   * @param e input element keyboard event.
   */
  private onKeydown(e: KeyboardEvent) {
    if (e.key === 'Tab') {
      document.execCommand('insertText', false, '\t');
      e.preventDefault();
    }
  }

  /**
   * Changes the text of the example's input box.
   */
  private setInputText(output: string) {
    if (this.inputEl) {
      this.inputEl.value = output;
    }
  }

  /**
   * Changes the text of the example's output box.
   */
  private setOutputText(output: string) {
    if (this.outputEl) {
      this.outputEl.textContent = output;
    }
  }

  /**
   * Sets the error message text and overwrites
   * output box to indicate a failed response.
   */
  private setErrorText(err: string) {
    if (this.errorEl) {
      this.errorEl.textContent = err;
    }
    this.setOutputText('An error has occurred…');
  }

  /**
   * Opens a new window to play.golang.org using the
   * example snippet's code in the playground.
   */
  private handleShareButtonClick() {
    const PLAYGROUND_BASE_URL = 'https://play.golang.org/p/';

    this.setOutputText('Waiting for remote server…');

    fetch('/play/share', {
      method: 'POST',
      body: this.inputEl?.value,
    })
      .then(res => res.text())
      .then(shareId => {
        const href = PLAYGROUND_BASE_URL + shareId;
        this.setOutputText(`<a href="${href}">${href}</a>`);
        window.open(href);
      })
      .catch(err => {
        this.setErrorText(err);
      });
  }

  /**
   * Runs gofmt on the example snippet in the playground.
   */
  private handleFormatButtonClick() {
    this.setOutputText('Waiting for remote server…');
    const body = new FormData();
    body.append('body', this.inputEl?.value ?? '');

    fetch('/play/fmt', {
      method: 'POST',
      body: body,
    })
      .then(res => res.json())
      .then(({ Body, Error }) => {
        this.setOutputText(Error || 'Done.');
        if (Body) {
          this.setInputText(Body);
          this.resize();
        }
      })
      .catch(err => {
        this.setErrorText(err);
      });
  }

  /**
   * Runs the code snippet in the example playground.
   */
  private handleRunButtonClick() {
    this.setOutputText('Waiting for remote server…');

    fetch('/play/compile', {
      method: 'POST',
      body: JSON.stringify({ body: this.inputEl?.value, version: 2 }),
    })
      .then(res => res.json())
      .then(async ({ Events, Errors }) => {
        this.setOutputText(Errors || '');
        for (const e of Events || []) {
          this.setOutputText(e.Message);
          await new Promise(resolve => setTimeout(resolve, e.Delay / 1000000));
        }
      })
      .catch(err => {
        this.setErrorText(err);
      });
  }
}

const exampleHashRegex = location.hash.match(/^#(example-.*)$/);
if (exampleHashRegex) {
  const exampleHashEl = document.getElementById(exampleHashRegex[1]) as HTMLDetailsElement;
  if (exampleHashEl) {
    exampleHashEl.open = true;
  }
}

// We use a spread operator to convert a nodelist into an array of elements.
const exampleHrefs = [
  ...document.querySelectorAll<HTMLAnchorElement>(PlayExampleClassName.PLAY_HREF),
];

/**
 * Sometimes exampleHrefs and playContainers are in different order, so we
 * find an exampleHref from a common hash.
 * @param playContainer - playground container
 */
const findExampleHash = (playContainer: PlaygroundExampleController) =>
  exampleHrefs.find(ex => {
    return ex.hash === playContainer.getAnchorHash();
  });

for (const el of document.querySelectorAll(PlayExampleClassName.PLAY_CONTAINER)) {
  // There should be the same amount of hrefs referencing examples as example containers.
  const playContainer = new PlaygroundExampleController(el as HTMLDetailsElement);
  const exampleHref = findExampleHash(playContainer);
  if (exampleHref) {
    exampleHref.addEventListener('click', () => {
      playContainer.expand();
    });
  } else {
    console.warn('example href not found');
  }
}
