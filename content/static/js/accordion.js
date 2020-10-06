/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export class AccordionController {
  constructor(accordion) {
    this.accordion = accordion;
    this.triggers = [...this.accordion.querySelectorAll('.js-accordionTrigger')];
    this.activeTrigger = this.triggers[0];
    this.init();
  }

  init() {
    this.accordion.addEventListener('click', e => {
      if (e.target.classList.contains('js-accordionTrigger')) {
        this.select(e.target);
      }
    });

    this.accordion.addEventListener('keydown', e => {
      if (e.target.classList.contains('js-accordionTrigger')) {
        this.handleKeyPress(e);
      }
    });
  }

  select(target) {
    const isExpanded = target.getAttribute('aria-expanded') === 'true';
    if (!isExpanded) {
      target.setAttribute('aria-expanded', 'true');
      document
        .getElementById(target.getAttribute('aria-controls'))
        .setAttribute('aria-hidden', 'false');
    }
    if (this.activeTrigger !== target) {
      this.activeTrigger.setAttribute('aria-expanded', 'false');
      document
        .getElementById(this.activeTrigger.getAttribute('aria-controls'))
        .setAttribute('aria-hidden', 'true');
    }
    this.activeTrigger = target;
  }

  handleKeyPress(e) {
    const target = e.target;
    const key = e.which;
    const PAGE_UP = 33;
    const PAGE_DOWN = 34;
    const END = 35;
    const HOME = 36;
    const ARROW_UP = 38;
    const ARROW_DOWN = 40;

    switch (key) {
      case PAGE_UP:
      case PAGE_DOWN:
      case ARROW_UP:
      case ARROW_DOWN:
        const index = this.triggers.indexOf(target);
        const direction = [PAGE_UP, ARROW_UP].includes(key) ? -1 : 1;
        const newIndex = (index + this.triggers.length + direction) % this.triggers.length;
        this.triggers[newIndex].focus();
        e.preventDefault();
        break;
      case END:
        this.triggers[this.triggers.length - 1].focus();
        e.preventDefault();
        break;
      case HOME:
        this.triggers[0].focus();
        e.preventDefault();
        break;

      default:
        break;
    }
  }
}
