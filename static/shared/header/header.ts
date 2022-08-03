/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export function registerHeaderListeners(): void {
  const header = document.querySelector('.js-header') as HTMLElement;

  // Desktop menu hover state
  const menuItemHovers = document.querySelectorAll('.js-desktop-menu-hover');
  menuItemHovers.forEach(menuItemHover => {
    // when user clicks on the dropdown menu item on desktop or mobile,
    // force the menu to stay open until the user clicks off of it.
    menuItemHover.addEventListener('mouseenter', e => {
      const target = e.target as HTMLElement;
      const forced = document.querySelector('.forced-open') as HTMLElement;
      if (forced && forced !== menuItemHover) {
        forced.blur();
        forced.classList.remove('forced-open');
      }
      // prevents menus that have been tabbed into from staying open
      // when you hover over another menu
      target.focus();
      target.blur();
    });

    const toggleForcedOpen = (e: Event) => {
      const target = e.target as HTMLElement;
      const isForced = target?.classList.contains('forced-open');
      const currentTarget = e.currentTarget as HTMLElement;
      if (isForced) {
        currentTarget.removeEventListener('blur', () =>
          currentTarget.classList.remove('forced-open')
        );
        currentTarget.classList.remove('forced-open');
        currentTarget.classList.add('forced-closed');
        currentTarget.blur();
        currentTarget?.parentNode?.addEventListener('mouseout', () => {
          currentTarget.classList.remove('forced-closed');
        });
      } else {
        currentTarget.classList.remove('forced-closed');
        currentTarget.classList.add('forced-open');
        currentTarget.focus();
        currentTarget.addEventListener('blur', () => currentTarget.classList.remove('forced-open'));
        currentTarget?.parentNode?.removeEventListener('mouseout', () => {
          currentTarget.classList.remove('forced-closed');
        });
      }
    };
    menuItemHover.addEventListener('click', toggleForcedOpen);
  });

  // ensure desktop submenus are closed when esc is pressed
  const headerItems = document.querySelectorAll('.Header-menuItem');
  headerItems.forEach(header => {
    header.addEventListener('keyup', e => {
      const event = e as KeyboardEvent;
      if (event.key === 'Escape') {
        (event.target as HTMLElement)?.blur();
      }
    });
  });

  // Mobile menu subnav menus
  const headerbuttons = document.querySelectorAll('.js-headerMenuButton');
  headerbuttons.forEach(button => {
    button.addEventListener('click', e => {
      e.preventDefault();
      const isActive = header?.classList.contains('is-active');
      if (isActive) {
        handleNavigationDrawerInactive(header);
      } else {
        handleNavigationDrawerActive(header);
      }
      button.setAttribute('aria-expanded', isActive ? 'true' : 'false');
    });
  });

  const scrim = document.querySelector('.js-scrim');
  scrim?.addEventListener('click', e => {
    e.preventDefault();

    // find any active submenus and close them
    const activeSubnavs = document.querySelectorAll('.go-NavigationDrawer-submenuItem.is-active');
    activeSubnavs.forEach(subnav => handleNavigationDrawerInactive(subnav as HTMLElement));

    handleNavigationDrawerInactive(header);

    headerbuttons.forEach(button => {
      button.setAttribute(
        'aria-expanded',
        header?.classList.contains('is-active') ? 'true' : 'false'
      );
    });
  });

  const getNavigationDrawerMenuItems = (navigationDrawer: HTMLElement): HTMLElement[] => {
    if (!navigationDrawer) {
      return [];
    }

    const menuItems = Array.from(
      navigationDrawer.querySelectorAll(
        ':scope > .go-NavigationDrawer-nav > .go-NavigationDrawer-list > .go-NavigationDrawer-listItem > a, :scope > .go-NavigationDrawer-nav > .go-NavigationDrawer-list > .go-NavigationDrawer-listItem > .go-Header-socialIcons > a'
      ) || []
    );
    console.log(menuItems);

    const anchorEl = navigationDrawer.querySelector('.go-NavigationDrawer-header > a');
    if (anchorEl) {
      menuItems.unshift(anchorEl);
    }
    return menuItems as HTMLElement[];
  };

  const getNavigationDrawerIsSubnav = (navigationDrawer: HTMLElement) => {
    if (!navigationDrawer) {
      return;
    }
    return navigationDrawer.classList.contains('go-NavigationDrawer-submenuItem');
  };

  const handleNavigationDrawerInactive = (navigationDrawer: HTMLElement) => {
    if (!navigationDrawer) {
      return;
    }
    const menuItems = getNavigationDrawerMenuItems(navigationDrawer);
    navigationDrawer.classList.remove('is-active');
    const parentMenuItem = navigationDrawer
      .closest('.go-NavigationDrawer-listItem')
      ?.querySelector(':scope > a') as HTMLElement;
    parentMenuItem?.focus();
    menuItems?.forEach(item => item?.setAttribute('tabindex', '-1'));
    if (menuItems && menuItems[0]) {
      menuItems[0].removeEventListener('keydown', handleMenuItemTabLeftFactory(navigationDrawer));
      menuItems[menuItems.length - 1].removeEventListener(
        'keydown',
        handleMenuItemTabRightFactory(navigationDrawer)
      );
    }

    if (navigationDrawer === header) {
      headerbuttons && (headerbuttons[0] as HTMLElement)?.focus();
    }
  };

  const handleNavigationDrawerActive = (navigationDrawer: HTMLElement) => {
    const menuItems = getNavigationDrawerMenuItems(navigationDrawer);

    navigationDrawer.classList.add('is-active');
    menuItems.forEach(item => item.setAttribute('tabindex', '0'));
    menuItems[0].focus();

    menuItems[0].addEventListener('keydown', handleMenuItemTabLeftFactory(navigationDrawer));
    menuItems[menuItems.length - 1].addEventListener(
      'keydown',
      handleMenuItemTabRightFactory(navigationDrawer)
    );
  };

  const handleMenuItemTabLeftFactory = (navigationDrawer: HTMLElement) => {
    return (e: KeyboardEvent) => {
      if (e.key === 'Tab' && e.shiftKey) {
        e.preventDefault();
        handleNavigationDrawerInactive(navigationDrawer);
      }
    };
  };

  const handleMenuItemTabRightFactory = (navigationDrawer: HTMLElement) => {
    return (e: KeyboardEvent) => {
      if (e.key === 'Tab' && !e.shiftKey) {
        e.preventDefault();
        handleNavigationDrawerInactive(navigationDrawer);
      }
    };
  };

  const prepMobileNavigationDrawer = (navigationDrawer: HTMLElement) => {
    const isSubnav = getNavigationDrawerIsSubnav(navigationDrawer);
    const menuItems = getNavigationDrawerMenuItems(navigationDrawer);
    navigationDrawer.addEventListener('keyup', e => {
      if (e.key === 'Escape') {
        handleNavigationDrawerInactive(navigationDrawer);
      }
    });

    menuItems.forEach(item => {
      const parentLi = item.closest('li');
      if (parentLi && parentLi.classList.contains('js-mobile-subnav-trigger')) {
        const submenu = parentLi.querySelector('.go-NavigationDrawer-submenuItem') as HTMLElement;
        item.addEventListener('click', () => {
          handleNavigationDrawerActive(submenu);
        });
      }
    });
    if (isSubnav) {
      handleNavigationDrawerInactive(navigationDrawer);
      navigationDrawer
        ?.querySelector('.go-NavigationDrawer-header')
        ?.addEventListener('click', e => {
          e.preventDefault();
          handleNavigationDrawerInactive(navigationDrawer);
        });
    }
  };

  document
    .querySelectorAll('.go-NavigationDrawer')
    .forEach(drawer => prepMobileNavigationDrawer(drawer as HTMLElement));

  handleNavigationDrawerInactive(header);
}

export function registerSearchFormListeners(): void {
  const searchForm = document.querySelector('.js-searchForm');
  const expandSearch = document.querySelector('.js-expandSearch');
  const input = searchForm?.querySelector('input');
  const headerLogo = document.querySelector('.js-headerLogo');
  const menuButton = document.querySelector('.js-headerMenuButton');
  expandSearch?.addEventListener('click', () => {
    searchForm?.classList.add('go-SearchForm--expanded');
    headerLogo?.classList.add('go-Header-logo--hidden');
    menuButton?.classList.add('go-Header-navOpen--hidden');
    input?.focus();
  });
  document?.addEventListener('click', e => {
    if (!searchForm?.contains(e.target as Node)) {
      searchForm?.classList.remove('go-SearchForm--expanded');
      headerLogo?.classList.remove('go-Header-logo--hidden');
      menuButton?.classList.remove('go-Header-navOpen--hidden');
    }
  });
}
