/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { ExpandableRowsTableController } from '../_table/table.js';

document
  .querySelectorAll<HTMLTableElement>('.js-expandableTable')
  .forEach(
    el =>
      new ExpandableRowsTableController(
        el,
        document.querySelector<HTMLButtonElement>('.js-expandAllDirectories')
      )
  );

/**
 * MainLayoutController calculates dynamic height values for header elements
 * to support variable size sticky positioned elements in the header so that
 * banners and breadcumbs may overflow to multiple lines.
 */

const headerHeight = 3.5;

export class MainLayoutController {
  private headerObserver: IntersectionObserver;
  private navObserver: IntersectionObserver;

  constructor(private mainHeader?: Element | null, private mainNav?: Element | null) {
    this.headerObserver = new IntersectionObserver(
      ([e]) => {
        if (e.intersectionRatio < 1) {
          for (const x of document.querySelectorAll('[class^="go-Main-header"')) {
            x.setAttribute('data-fixed', 'true');
          }
        } else {
          for (const x of document.querySelectorAll('[class^="go-Main-header"')) {
            x.removeAttribute('data-fixed');
          }
          this.handleResize();
        }
      },
      { threshold: 1 }
    );
    this.navObserver = new IntersectionObserver(
      ([e]) => {
        if (e.intersectionRatio < 1) {
          this.mainNav?.classList.add('go-Main-nav--fixed');
          this.mainNav?.setAttribute('data-fixed', 'true');
        } else {
          this.mainNav?.classList.remove('go-Main-nav--fixed');
          this.mainNav?.removeAttribute('data-fixed');
        }
      },
      { threshold: 1, rootMargin: `-${(headerHeight ?? 0) * 16 + 10}px` }
    );
    this.init();
  }

  private init() {
    this.handleResize();
    window.addEventListener('resize', this.handleResize);
    this.mainHeader?.addEventListener('dblclick', this.handleDoubleClick);
    if (this.mainHeader?.hasChildNodes()) {
      const headerSentinel = document.createElement('div');
      this.mainHeader?.prepend(headerSentinel);
      this.headerObserver.observe(headerSentinel);
    }
    if (this.mainNav?.hasChildNodes()) {
      const navSentinel = document.createElement('div');
      this.mainNav?.prepend(navSentinel);
      this.navObserver.observe(navSentinel);
    }
  }

  private handleDoubleClick: EventListener = e => {
    const target = e.target;
    if (target === this.mainHeader?.lastElementChild) {
      window.getSelection()?.removeAllRanges();
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
  };

  private handleResize = () => {
    const setProp = (name: string, value: string) =>
      document.documentElement.style.setProperty(name, value);
    setProp('--js-unit-header-height', '0');
    setTimeout(() => {
      const mainHeaderHeight = (this.mainHeader?.getBoundingClientRect().height ?? 0) / 16;
      setProp('--js-unit-header-height', `${mainHeaderHeight}rem`);
      setProp('--js-sticky-header-height', `${headerHeight}rem`);
      setProp('--js-unit-header-top', `${(mainHeaderHeight - headerHeight) * -1}rem`);
    });
  };
}

const el = <T extends HTMLElement>(selector: string) => document.querySelector<T>(selector);
new MainLayoutController(el('.js-mainHeader'), el('.js-mainNav'));
