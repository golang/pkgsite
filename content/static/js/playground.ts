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
};

/**
 * This controller enables playground examples to expand their dropdown or
 * generate shareable Go Playground URLs.
 */
export class PlaygroundExampleController {
  private readonly anchorEl: HTMLAnchorElement | null;
  private readonly errorEl: Element | null;
  private readonly playButtonEl: Element | null;
  private readonly inputEl: Element | null;
  private readonly outputEl: Element | null;

  /**
   * @param exampleEl - The div that contains playground content for the given example.
   */
  constructor(private exampleEl: HTMLDetailsElement) {
    // Used to indicate when to terminate before the eventlistener is added.
    let hasError = false;

    if (!exampleEl) {
      console.warn('Must provide playground example element');
      hasError = true;
    }
    /**
     * The example container
     */
    this.exampleEl = exampleEl;

    /**
     * The anchor tag used to identify the container with an example href.
     * There is only one in an example container div.
     */
    const anchorEl = exampleEl.querySelector('a');
    if (!anchorEl) {
      console.warn('anchor tag is not detected');
      hasError = true;
    }
    this.anchorEl = anchorEl;

    /**
     * The error element
     */
    const errorEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_ERROR);
    if (!errorEl) {
      hasError = true;
    }
    this.errorEl = errorEl;

    /**
     * Button that redirects to an example's playground, this element
     * only exists in executable examples.
     */
    const playButtonEl = exampleEl.querySelector(PlayExampleClassName.PLAY_BUTTON);
    if (!playButtonEl) {
      hasError = true;
    }
    this.playButtonEl = playButtonEl;

    /**
     * The executable code of an example.
     */
    const inputEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_INPUT);
    if (!inputEl) {
      console.warn('Input element is not detected');
      hasError = true;
    }
    this.inputEl = inputEl;

    /**
     * The output of the given example code. This only exists if the
     * author of the package provides an output for this example.
     */
    this.outputEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_OUTPUT);

    if (hasError) {
      return;
    }

    this.playButtonEl?.addEventListener('click', () => this.handlePlayButtonClick());
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
  private handlePlayButtonClick() {
    const PLAYGROUND_BASE_URL = '//play.golang.org/p/';

    this.setOutputText('Waiting for remote server…');

    fetch('/play/', {
      method: 'POST',
      body: this.inputEl?.textContent,
    })
      .then(res => res.text())
      .then(shareId => {
        window.open(PLAYGROUND_BASE_URL + shareId);
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
