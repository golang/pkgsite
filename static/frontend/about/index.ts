/**
 * @license
 * Copyright 2022 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * Left Navigation.
 */
export const initJumpLinks = async function () {
  const pagesWithJumpLinks = ['/about'];
  if (!pagesWithJumpLinks.includes(window.location.pathname)) {
    // stop the file from doing anything else if the page doesn't have jumplinks
    return;
  }

  // these might be generated or not so don't grab references to the elements until actually need them.
  const titles = 'h2, h3, h4';
  const nav = '.LeftNav a';
  // these are always in the dom so we can get them now and throw errors if they're not.
  const leftNav = document.querySelector('.LeftNav');
  const siteContent = document.querySelector('.go-Content');
  let isObserverDisabled = false;

  /**
   * El function
   * @example el('h1', {className: 'title'}, 'Welcome to the site');
   * @example el('ul', {className: 'list'}, el('li', {}, 'Item one'), el('li', {}, 'Item two'), el('li', {}, 'Item three'));
   * @example el('img', {src: '/url.svg'});
   */
  function el(
    type = '',
    props: { [key: string]: string } = {},
    ...children: (HTMLElement | HTMLElement[] | string | undefined)[]
  ) {
    // Error, no type declared.
    if (!type) {
      throw new Error('Provide `type` to create document element.');
    }

    // Create element with optional attribute props
    const docEl = Object.assign(document.createElement(type), props);

    // Children: array containing strings or elements
    children.forEach(child => {
      if (typeof child === 'string') {
        docEl.appendChild(document.createTextNode(child));
      } else if (Array.isArray(child)) {
        child.forEach(c => docEl.appendChild(c));
      } else if (child instanceof HTMLElement) {
        docEl.appendChild(child);
      }
    });

    return docEl;
  }
  /**  Build Nav if data hydrate is present. */
  function buildNav() {
    return new Promise((resolve, reject) => {
      let navItems: { id: string; label: string; subnav?: { id: string; label: string }[] }[] = [];
      let elements: HTMLElement[] = [];

      if (!siteContent || !leftNav) {
        return reject('.SiteContent not found.');
      }
      if (leftNav instanceof HTMLElement && !leftNav?.dataset?.hydrate) {
        return resolve(true);
      }

      for (const title of siteContent.querySelectorAll(titles)) {
        if (title instanceof HTMLElement && !title?.dataset?.ignore) {
          switch (title.tagName) {
            case 'H2':
              navItems = [
                ...navItems,
                {
                  id: title.id,
                  label: title?.dataset?.title ? title.dataset.title : title.textContent ?? '',
                },
              ];
              break;

            case 'H3':
            case 'H4':
              if (!navItems[navItems.length - 1]?.subnav) {
                navItems[navItems.length - 1].subnav = [
                  {
                    id: title.id,
                    label: title?.dataset?.title ? title.dataset.title : title.textContent ?? '',
                  },
                ];
              } else if (navItems[navItems.length - 1].subnav) {
                navItems[navItems.length - 1].subnav?.push({
                  id: title.id,
                  label: title?.dataset?.title ? title.dataset.title : title.textContent ?? '',
                });
              }
              break;
          }
        }
      }

      for (const navItem of navItems) {
        const link = el('a', { href: '#' + navItem.id }, el('span', {}, navItem.label));
        elements = [...elements, link];
        if (navItem?.subnav) {
          let subLinks: HTMLElement[] = [];
          for (const subnavItem of navItem.subnav) {
            const subItem = el(
              'li',
              {},
              el(
                'a',
                { href: '#' + subnavItem.id },
                el('img', { src: '/static/frontend/about/dot.svg', width: '5', height: '5' }),
                el('span', {}, subnavItem.label)
              )
            );
            subLinks = [...subLinks, subItem];
          }
          const list = el('ul', { className: 'LeftSubnav' }, subLinks);
          elements = [...elements, list];
        }
      }

      elements.forEach(element => leftNav.appendChild(element));

      return resolve(true);
    });
  }
  /**
   * Set the correct active element.
   */
  function setNav() {
    return new Promise(resolve => {
      if (!document.querySelectorAll(nav)) return resolve(true);
      for (const a of document.querySelectorAll(nav)) {
        if (a instanceof HTMLAnchorElement && a.href === location.href) {
          setElementActive(a);
          break;
        }
      }
      resolve(true);
    });
  }
  /** resetNav: removes all .active from nav elements */
  function resetNav() {
    return new Promise(resolve => {
      if (!document.querySelectorAll(nav)) return resolve(true);
      for (const a of document.querySelectorAll(nav)) {
        a.classList.remove('active');
      }
      resolve(true);
    });
  }
  /** setElementActive: controls resetting nav and highlighting the appropriate nav items */
  function setElementActive(element: HTMLAnchorElement) {
    if (element instanceof HTMLAnchorElement) {
      resetNav().then(() => {
        element.classList.add('active');
        const parent = element?.parentNode?.parentNode;
        if (parent instanceof HTMLElement && parent?.classList?.contains('LeftSubnav')) {
          parent.previousElementSibling?.classList.add('active');
        }
      });
    }
  }
  /** setLinkManually: disables observer and selects the clicked nav item. */
  function setLinkManually() {
    delayObserver();
    const link = document.querySelector('[href="' + location.hash + '"]');
    if (link instanceof HTMLAnchorElement) {
      setElementActive(link);
    }
  }
  /** delayObserver: Quick on off switch for intersection observer. */
  function delayObserver() {
    isObserverDisabled = true;
    setTimeout(() => {
      isObserverDisabled = false;
    }, 200);
  }
  /** observeSections: kicks off observation of titles as well as manual clicks with hashchange */
  function observeSections() {
    window.addEventListener('hashchange', setLinkManually);

    if (siteContent?.querySelectorAll(titles)) {
      const callback: IntersectionObserverCallback = entries => {
        if (!isObserverDisabled && Array.isArray(entries) && entries.length > 0) {
          for (const entry of entries) {
            if (entry.isIntersecting && entry.target instanceof HTMLElement) {
              const { id } = entry.target;
              const link = document.querySelector('[href="#' + id + '"]');
              if (link instanceof HTMLAnchorElement) {
                setElementActive(link);
              }
              break;
            }
          }
        }
      };
      // rootMargin is important when multiple sections are in the observable area **on page load**.
      // they will still be highlighted on scroll because of the root margin.
      const ob = new IntersectionObserver(callback, {
        threshold: 0,
        rootMargin: '0px 0px -50% 0px',
      });
      for (const title of siteContent.querySelectorAll(titles)) {
        if (title instanceof HTMLElement && !title?.dataset?.ignore) {
          ob.observe(title);
        }
      }
    }
  }

  try {
    await buildNav();
    await setNav();
    if (location.hash) {
      delayObserver();
    }
    observeSections();
  } catch (e) {
    if (e instanceof Error) {
      console.error(e.message);
    } else {
      console.error(e);
    }
  }
};
