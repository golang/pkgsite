/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * MainLayoutController calculates dynamic height values for header elements
 * to support variable size sticky positioned elements in the header so that
 * banners and breadcumbs may overflow to multiple lines.
 */
export class MainLayoutController {
  private stickyHeader: Element | null;
  private stickyNav: Element | null;
  private headerObserver: IntersectionObserver;
  private navObserver: IntersectionObserver;

  constructor(private mainHeader?: Element | null, private mainNav?: Element | null) {
    this.stickyHeader = mainHeader.querySelector('.js-stickyHeader') ?? mainHeader.lastElementChild;
    this.stickyNav = mainNav.lastElementChild;
    this.headerObserver = new IntersectionObserver(
      ([e]) => {
        if (e.intersectionRatio < 1) {
          this.mainHeader?.classList.add('go-Main-header--fixed');
        } else {
          this.mainHeader?.classList.remove('go-Main-header--fixed');
          this.handleResize(null);
        }
      },
      { threshold: 1 }
    );
    this.navObserver = new IntersectionObserver(
      ([e]) => {
        if (e.intersectionRatio < 1) {
          this.mainNav?.classList.add('go-Main-nav--fixed');
        } else {
          this.mainNav?.classList.remove('go-Main-nav--fixed');
        }
      },
      { threshold: 1, rootMargin: `-${this.stickyHeader?.clientHeight ?? 0 + 1}px` }
    );
    this.init();
  }

  private init() {
    this.handleResize(null);
    window.addEventListener('resize', this.handleResize);
    this.stickyHeader?.addEventListener('dblclick', this.handleDoubleClick);

    const headerSentinel = document.createElement('div');
    this.mainHeader?.prepend(headerSentinel);
    this.headerObserver.observe(headerSentinel);

    const navSentinel = document.createElement('div');
    this.mainNav?.prepend(navSentinel);
    this.navObserver.observe(navSentinel);
  }

  private handleDoubleClick: EventListener = e => {
    const target = e.target;
    if (target === this.stickyHeader) {
      window.getSelection()?.removeAllRanges();
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
  };

  private handleResize: EventListener = () => {
    const setProp = (name: string, value: string) =>
      document.documentElement.style.setProperty(name, value);
    setProp('--js-main-header-height', '0');
    setTimeout(() => {
      const mainHeaderHeight = (this.mainHeader?.getBoundingClientRect().height ?? 0) / 16;
      const stickyHeaderHeight = (this.stickyHeader?.getBoundingClientRect().height ?? 0) / 16;
      const stickyNavHeight = (this.stickyNav?.getBoundingClientRect().height ?? 0) / 16;
      setProp('--js-main-header-height', `${mainHeaderHeight}rem`);
      setProp('--js-sticky-header-height', `${stickyHeaderHeight}rem`);
      setProp('--js-sticky-nav-height', `${stickyNavHeight}rem`);
    });
  };
}
