/*!
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
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
export class PlaygroundExampleController {
  constructor(exampleEl) {
    var _a, _b, _c, _d;
    this.exampleEl = exampleEl;
    this.exampleEl = exampleEl;
    this.anchorEl = exampleEl.querySelector('a');
    this.errorEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_ERROR);
    this.playButtonEl = exampleEl.querySelector(PlayExampleClassName.PLAY_BUTTON);
    this.shareButtonEl = exampleEl.querySelector(PlayExampleClassName.SHARE_BUTTON);
    this.formatButtonEl = exampleEl.querySelector(PlayExampleClassName.FORMAT_BUTTON);
    this.runButtonEl = exampleEl.querySelector(PlayExampleClassName.RUN_BUTTON);
    this.inputEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_INPUT);
    this.outputEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_OUTPUT);
    (_a = this.playButtonEl) === null || _a === void 0
      ? void 0
      : _a.addEventListener('click', () => this.handleShareButtonClick());
    (_b = this.shareButtonEl) === null || _b === void 0
      ? void 0
      : _b.addEventListener('click', () => this.handleShareButtonClick());
    (_c = this.formatButtonEl) === null || _c === void 0
      ? void 0
      : _c.addEventListener('click', () => this.handleFormatButtonClick());
    (_d = this.runButtonEl) === null || _d === void 0
      ? void 0
      : _d.addEventListener('click', () => this.handleRunButtonClick());
    if (!this.inputEl) return;
    this.resize();
    this.inputEl.addEventListener('keyup', () => this.resize());
    this.inputEl.addEventListener('keydown', e => this.onKeydown(e));
  }
  getAnchorHash() {
    var _a;
    return (_a = this.anchorEl) === null || _a === void 0 ? void 0 : _a.hash;
  }
  expand() {
    this.exampleEl.open = true;
  }
  resize() {
    var _a;
    if ((_a = this.inputEl) === null || _a === void 0 ? void 0 : _a.value) {
      const numLineBreaks = (this.inputEl.value.match(/\n/g) || []).length;
      this.inputEl.style.height = `${(20 + numLineBreaks * 20 + 12 + 2) / 16}rem`;
    }
  }
  onKeydown(e) {
    if (e.key === 'Tab') {
      document.execCommand('insertText', false, '\t');
      e.preventDefault();
    }
  }
  setInputText(output) {
    if (this.inputEl) {
      this.inputEl.value = output;
    }
  }
  setOutputText(output) {
    if (this.outputEl) {
      this.outputEl.innerHTML = output;
    }
  }
  setErrorText(err) {
    if (this.errorEl) {
      this.errorEl.textContent = err;
    }
    this.setOutputText('An error has occurred…');
  }
  handleShareButtonClick() {
    var _a;
    const PLAYGROUND_BASE_URL = 'https://play.golang.org/p/';
    this.setOutputText('Waiting for remote server…');
    fetch('/play/share', {
      method: 'POST',
      body: (_a = this.inputEl) === null || _a === void 0 ? void 0 : _a.value,
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
  handleFormatButtonClick() {
    var _a, _b;
    this.setOutputText('Waiting for remote server…');
    const body = new FormData();
    body.append(
      'body',
      (_b = (_a = this.inputEl) === null || _a === void 0 ? void 0 : _a.value) !== null &&
        _b !== void 0
        ? _b
        : ''
    );
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
  handleRunButtonClick() {
    var _a;
    this.setOutputText('Waiting for remote server…');
    fetch('/play/compile', {
      method: 'POST',
      body: JSON.stringify({
        body: (_a = this.inputEl) === null || _a === void 0 ? void 0 : _a.value,
        version: 2,
      }),
    })
      .then(res => res.json())
      .then(async ({ Events, Errors }) => {
        if (Errors) {
          this.setOutputText(Errors);
        }
        this.setOutputText('');
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
  const exampleHashEl = document.getElementById(exampleHashRegex[1]);
  if (exampleHashEl) {
    exampleHashEl.open = true;
  }
}
const exampleHrefs = [...document.querySelectorAll(PlayExampleClassName.PLAY_HREF)];
const findExampleHash = playContainer =>
  exampleHrefs.find(ex => {
    return ex.hash === playContainer.getAnchorHash();
  });
for (const el of document.querySelectorAll(PlayExampleClassName.PLAY_CONTAINER)) {
  const playContainer = new PlaygroundExampleController(el);
  const exampleHref = findExampleHash(playContainer);
  if (exampleHref) {
    exampleHref.addEventListener('click', () => {
      playContainer.expand();
    });
  } else {
    console.warn('example href not found');
  }
}
//# sourceMappingURL=playground.js.map
