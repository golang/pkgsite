/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

// This file implements the playground implementation of the documentation
// page. The playground involves a "play" button that allows you to open up
// a new link to play.golang.org using the example code.

// The CSS is in content/static/css/stylesheet.css.

/**
 * CSS classes used by PlaygroundExampleController
 * @private @enum {string}
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
class PlaygroundExampleController {
  /**
   * @param {Element} exampleEl - The div that contains playground content for the given example.
   */
  constructor(exampleEl) {
    // Used to indicate when to terminate before the eventlistener is added.
    let hasError = false;

    if (!exampleEl) {
      console.warn('Must provide playground example element');
      hasError = true;
    }
    /**
     * The example container
     * @private {Element}
     */
    this._exampleEl = /** @type {!Element} */ (exampleEl);

    /**
     * The anchor tag used to identify the container with an example href.
     * There is only one in an example container div.
     * @private {Element}
     */
    const anchorEl = exampleEl.querySelector('a');
    if (!anchorEl) {
      console.warn('anchor tag is not detected');
      hasError = true;
    }
    this._anchorEl = /** @type {!Element} */ (anchorEl);

    /**
     * The error element
     * @private {Element}
     */
    const errorEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_ERROR);
    if (!errorEl) {
      hasError = true;
    }
    this._errorEl = /** @type {!Element} */ (errorEl);

    /**
     * Button that redirects to an example's playground, this element
     * only exists in executable examples.
     * @private {Element}
     */
    const playButtonEl = exampleEl.querySelector(PlayExampleClassName.PLAY_BUTTON);
    if (!playButtonEl) {
      hasError = true;
    }
    this._playButtonEl = /** @type {!Element} */ (playButtonEl);

    /**
     * The executable code of an example.
     * @private {Element}
     */
    const inputEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_INPUT);
    if (!inputEl) {
      console.warn('Input element is not detected');
      hasError = true;
    }
    this._inputEl = /** @type {!Element} */ (inputEl);

    /**
     * The output of the given example code. This only exists if the
     * author of the package provides an output for this example.
     * @private {Element}
     */
    this._outputEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_OUTPUT);

    if (hasError) {
      return;
    }

    this._playButtonEl.addEventListener('click', e =>
      this.handlePlayButtonClick(/** @type {!MouseEvent} */ (e))
    );
  }

  /**
   * Retrieve the hash value of the anchor element.
   * @returns {string}
   */
  getAnchorHash() {
    return this._anchorEl.hash;
  }

  /**
   * Expands the current playground example.
   */
  expand() {
    this._exampleEl.open = true;
  }

  /**
   * Changes the text of the example's output box.
   * @param {string} output
   */
  setOutputText(output) {
    if (this._outputEl) {
      this._outputEl.textContent = output;
    }
  }

  /**
   * Sets the error message text and overwrites
   * output box to indicate a failed response.
   * @param {string} err
   */
  setErrorText(err) {
    this._errorEl.textContent = err;
    this.setOutputText('An error has occurred…');
  }

  /**
   * Opens a new window to play.golang.org using the
   * example snippet's code in the playground.
   * @param {!MouseEvent} e
   * @private
   */
  handlePlayButtonClick(e) {
    const PLAYGROUND_BASE_URL = '//play.golang.org/p/';

    this.setOutputText('Waiting for remote server…');

    fetch('/play/', {
      method: 'POST',
      body: this._inputEl.textContent,
    })
      .then(res => res.text())
      .then(shareId => {
        window.open(PLAYGROUND_BASE_URL + shareId);
      })
      .catch(err => {
        this.setErrorText(/** @type {!string} */ (err));
      });
  }
}

const exampleHashRegex = location.hash.match(/^#(example-.*)$/);
if (exampleHashRegex) {
  const exampleHashEl = document.getElementById(exampleHashRegex[1]);
  if (exampleHashEl) {
    exampleHashEl.open = true;
  }
}

// We use a spread operator to convert a nodelist into an array of elements.
const /** @type {Array<Element>} */ exampleHrefs = [
    ...document.querySelectorAll(PlayExampleClassName.PLAY_HREF),
  ];

/**
 * Sometimes exampleHrefs and playContainers are in different order, so we
 * find an exampleHref from a common hash.
 * @param {PlaygroundExampleController} playContainer - playground container
 */
const findExampleHash = playContainer =>
  exampleHrefs.find(ex => {
    return ex.hash === playContainer.getAnchorHash();
  });

document.querySelectorAll(PlayExampleClassName.PLAY_CONTAINER).forEach(el => {
  // There should be the same amount of hrefs referencing examples as example containers.
  const playContainer = new PlaygroundExampleController(el);
  const exampleHref = findExampleHash(playContainer);
  if (exampleHref) {
    exampleHref.addEventListener('click', () => {
      playContainer.expand();
    });
  } else {
    console.warn('example href not found');
  }
});
