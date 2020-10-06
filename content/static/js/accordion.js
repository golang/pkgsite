/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * Controller for an accordion element. This class will add event
 * listeners to close and open the panels of an accordion. When
 * initialized it will select the active panel based on the URL
 * hash for the requested page.
 *
 * Accordions must have the following structure:
 *
 *  <div class="js-accordion">
 *    <a role="button" class="js-accordionTrigger" href="#panel-1" aria-expanded="false" aria-controls="first-panel" id="first-accordion">
 *      Title
 *    </a>
 *    <div class="js-accordionPanel" id="first-panel" role="region" aria-labelledby="first-accordion" aria-hidden="true">
 *      Panel Content
 *    </div>
 *    <a role="button" class="js-accordionTrigger" href="#panel-2" aria-expanded="false" aria-controls="second-panel" id="second-accordion">
 *      Title
 *    </a>
 *    <div class="js-accordionPanel" id="second-panel" role="region" aria-labelledby="second-accordion" aria-hidden="true">
 *      Panel Content
 *    </div>
 *  </div>
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

    const activeHash = document.querySelector(`a[href=${JSON.stringify(window.location.hash)}]`);
    const initialTrigger =
      this.triggers.find(
        trigger => this.getPanel(trigger).contains(activeHash) || trigger.contains(activeHash)
      ) || this.activeTrigger;

    this.select(initialTrigger);
  }

  getPanel(trigger) {
    return document.getElementById(trigger.getAttribute('aria-controls'));
  }

  select(target) {
    const isExpanded = target.getAttribute('aria-expanded') === 'true';
    if (!isExpanded) {
      target.setAttribute('aria-expanded', 'true');
      this.getPanel(target).setAttribute('aria-hidden', 'false');
    }
    if (this.activeTrigger !== target) {
      this.activeTrigger.setAttribute('aria-expanded', 'false');
      this.getPanel(this.activeTrigger).setAttribute('aria-hidden', 'true');
    }
    this.activeTrigger = target;
  }

  handleKeyPress(e) {
    const target = e.target;
    const key = e.which;
    const SPACE = 32;
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
      case SPACE:
        this.select(target);

      default:
        break;
    }
  }
}
