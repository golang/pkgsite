/*!
 * @license
 * Copyright 2019-2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * Possible KeyboardEvent key values.
 * @private @enum {string}
 */
const Key = {
  UP: 'ArrowUp',
  DOWN: 'ArrowDown',
  LEFT: 'ArrowLeft',
  RIGHT: 'ArrowRight',
  ENTER: 'Enter',
  ASTERISK: '*',
  SPACE: ' ',
  END: 'End',
  HOME: 'Home',

  // Global keyboard shortcuts.
  // TODO(golang.org/issue/40246): consolidate keyboard shortcut handling to avoid
  // this duplication.
  Y: 'y',
  FORWARD_SLASH: '/',
  QUESTION_MARK: '?',
};

/**
 * The navigation tree component of the documentation page.
 */
class DocNavTreeController {
  /**
   * The currently selected element.
   */
  private selectedEl: HTMLElement | null;
  /**
   * The index of the currently focused item. Used when navigating the tree
   * using the keyboard.
   */
  private focusedIndex = 0;
  /**
   * The elements currently visible (not within a collapsed node of the tree).
   */
  private visibleItems: HTMLElement[] = [];
  /**
   * The current search string.
   */
  private searchString = '';
  /**
   * The timestamp of the last keydown event. Used to track whether to use the
   * current search string.
   */
  private lastKeyDownTimeStamp = -Infinity;

  /**
   * Instantiates a navigation tree.
   */
  constructor(private el: Element) {
    this.el = el;
    this.selectedEl = null;
    this.focusedIndex = 0;
    this.visibleItems = [];
    this.searchString = '';
    this.lastKeyDownTimeStamp = -Infinity;
    this.addEventListeners();
    this.updateVisibleItems();
    this.initialize();
  }

  /**
   * Initializes the tree. Should be called only once.
   */
  private initialize() {
    this.el.querySelectorAll(`[role='treeitem']`).forEach(el => {
      el.addEventListener('click', e => this.handleItemClick(e as MouseEvent));
    });

    // TODO: remove once safehtml supports aria-owns with dynamic values.
    this.el.querySelectorAll('[data-aria-owns]').forEach(el => {
      el.setAttribute('aria-owns', el.getAttribute('data-aria-owns') ?? '');
    });
  }

  private addEventListeners() {
    this.el.addEventListener('keydown', e => this.handleKeyDown(e as KeyboardEvent));
  }

  /**
   * Sets the visible item with the given index with the proper tabindex and
   * focuses it.
   */
  setFocusedIndex(index: number) {
    if (index === this.focusedIndex || index === -1) {
      return;
    }

    let itemEl = this.visibleItems[this.focusedIndex];
    itemEl.setAttribute('tabindex', '-1');

    itemEl = this.visibleItems[index];
    itemEl.setAttribute('tabindex', '0');
    itemEl.focus();

    this.focusedIndex = index;
  }

  /**
   * Marks the navigation node with the given ID as selected. If no ID is
   * provided, the first visible item in the tree is used.
   */
  setSelectedId(opt_id: string) {
    if (this.selectedEl) {
      this.selectedEl.removeAttribute('aria-selected');
      this.selectedEl = null;
    }
    if (opt_id) {
      this.selectedEl = this.el.querySelector(`[role='treeitem'][href='#${opt_id}']`);
    } else if (this.visibleItems.length > 0) {
      this.selectedEl = this.visibleItems[0];
    }

    if (!this.selectedEl) {
      return;
    }

    // Close inactive top level item if selected id is not in its tree.
    const topLevelExpanded = this.el.querySelector<HTMLElement>(
      '[aria-level="1"][aria-expanded="true"]'
    );
    if (topLevelExpanded && !topLevelExpanded.parentElement?.contains(this.selectedEl)) {
      this.collapseItem(topLevelExpanded);
    }

    if (this.selectedEl.getAttribute('aria-level') === '1') {
      this.selectedEl.setAttribute('aria-expanded', 'true');
    }
    this.selectedEl.setAttribute('aria-selected', 'true');
    this.expandAllParents(this.selectedEl);
    this.scrollElementIntoView(this.selectedEl);
  }

  /**
   * Expands all sibling items of the given element.
   */
  private expandAllSiblingItems(el: HTMLElement) {
    const level = el.getAttribute('aria-level');
    this.el.querySelectorAll(`[aria-level='${level}'][aria-expanded='false']`).forEach(el => {
      el.setAttribute('aria-expanded', 'true');
    });
    this.updateVisibleItems();
    this.focusedIndex = this.visibleItems.indexOf(el);
  }

  /**
   * Expands all parent items of the given element.
   */
  private expandAllParents(el: HTMLElement) {
    if (!this.visibleItems.includes(el)) {
      let owningItemEl = this.owningItem(el);
      while (owningItemEl) {
        this.expandItem(owningItemEl);
        owningItemEl = this.owningItem(owningItemEl);
      }
    }
  }

  /**
   * Scrolls the given element into view, aligning the element in the center
   * of the viewport. If the element is already in view, no scrolling occurs.
   */
  private scrollElementIntoView(el: HTMLElement) {
    const STICKY_HEADER_HEIGHT_PX = 55;
    const viewportHeightPx = document.documentElement.clientHeight;
    const elRect = el.getBoundingClientRect();
    const verticalCenterPointPx = (viewportHeightPx - STICKY_HEADER_HEIGHT_PX) / 2;
    if (elRect.top < STICKY_HEADER_HEIGHT_PX) {
      // Element is occluded at top of view by header or by being offscreen.
      this.el.scrollTop -=
        STICKY_HEADER_HEIGHT_PX - elRect.top - elRect.height + verticalCenterPointPx;
    } else if (elRect.bottom > viewportHeightPx) {
      // Element is below viewport.
      this.el.scrollTop = elRect.bottom - viewportHeightPx + verticalCenterPointPx;
    } else {
      return;
    }
  }

  /**
   * Handles when a tree item is clicked.
   */
  private handleItemClick(e: MouseEvent) {
    const el = e.target as HTMLSelectElement | null;
    if (!el) return;
    this.setFocusedIndex(this.visibleItems.indexOf(el));
    if (el.hasAttribute('aria-expanded')) {
      this.toggleItemExpandedState(el);
    }
    this.closeInactiveDocNavGroups(el);
  }

  /**
   * Closes inactive top level nav groups when a new tree item clicked.
   */
  private closeInactiveDocNavGroups(el: HTMLElement) {
    if (el.hasAttribute('aria-expanded')) {
      const level = el.getAttribute('aria-level');
      document.querySelectorAll(`[aria-level="${level}"]`).forEach(nav => {
        if (nav.getAttribute('aria-expanded') === 'true' && nav !== el) {
          nav.setAttribute('aria-expanded', 'false');
        }
      });
      this.updateVisibleItems();
      this.focusedIndex = this.visibleItems.indexOf(el);
    }
  }

  /**
   * Handles when a key is pressed when the component is in focus.
   */
  handleKeyDown(e: KeyboardEvent) {
    const targetEl = e.target as HTMLElement | null;

    switch (e.key) {
      case Key.ASTERISK:
        if (targetEl) {
          this.expandAllSiblingItems(targetEl);
        }
        e.stopPropagation();
        e.preventDefault();
        return;

      // Global keyboard shortcuts.
      // TODO(golang.org/issue/40246): consolidate keyboard shortcut handling
      // to avoid this duplication.
      case Key.FORWARD_SLASH:
      case Key.QUESTION_MARK:
        return;

      case Key.DOWN:
        this.focusNextItem();
        break;

      case Key.UP:
        this.focusPreviousItem();
        break;

      case Key.LEFT:
        if (targetEl?.getAttribute('aria-expanded') === 'true') {
          this.collapseItem(targetEl);
        } else {
          this.focusParentItem(targetEl);
        }
        break;

      case Key.RIGHT: {
        switch (targetEl?.getAttribute('aria-expanded')) {
          case 'false':
            this.expandItem(targetEl);
            break;
          case 'true':
            // Select the first child.
            this.focusNextItem();
            break;
        }
        break;
      }

      case Key.HOME:
        this.setFocusedIndex(0);
        break;

      case Key.END:
        this.setFocusedIndex(this.visibleItems.length - 1);
        break;

      case Key.ENTER:
        if (targetEl?.tagName === 'A') {
          // Enter triggers desired behavior by itself.
          return;
        }
      // Fall through for non-anchor items to be handled the same as when
      // the space key is pressed.
      // eslint-disable-next-line no-fallthrough
      case Key.SPACE:
        targetEl?.click();
        break;

      default:
        // Could be a typeahead search.
        this.handleSearch(e);
        return;
    }
    e.preventDefault();
    e.stopPropagation();
  }

  /**
   * Handles when a key event isn’t matched by shortcut handling, indicating
   * that the user may be attempting a typeahead search.
   */
  private handleSearch(e: KeyboardEvent) {
    if (
      e.metaKey ||
      e.altKey ||
      e.ctrlKey ||
      e.isComposing ||
      e.key.length > 1 ||
      !e.key.match(/\S/)
    ) {
      return;
    }

    // KeyDown events should be within one second of each other to be considered
    // part of the same typeahead search string.
    const MAX_TYPEAHEAD_THRESHOLD_MS = 1000;
    if (e.timeStamp - this.lastKeyDownTimeStamp > MAX_TYPEAHEAD_THRESHOLD_MS) {
      this.searchString = '';
    }
    this.lastKeyDownTimeStamp = e.timeStamp;
    this.searchString += e.key.toLocaleLowerCase();
    const focusedElementText = this.visibleItems[
      this.focusedIndex
    ].textContent?.toLocaleLowerCase();
    if (this.searchString.length === 1 || !focusedElementText?.startsWith(this.searchString)) {
      this.focusNextItemWithPrefix(this.searchString);
    }
    e.stopPropagation();
    e.preventDefault();
  }

  /**
   * Focuses on the next visible tree item (after the currently focused element,
   * wrapping the tree) that has a prefix equal to the given search string.
   */
  focusNextItemWithPrefix(prefix: string) {
    let i = this.focusedIndex + 1;
    if (i > this.visibleItems.length - 1) {
      i = 0;
    }
    while (i !== this.focusedIndex) {
      if (this.visibleItems[i].textContent?.toLocaleLowerCase().startsWith(prefix)) {
        this.setFocusedIndex(i);
        return;
      }
      if (i >= this.visibleItems.length - 1) {
        i = 0;
      } else {
        i++;
      }
    }
  }

  private toggleItemExpandedState(el: HTMLElement) {
    el.getAttribute('aria-expanded') === 'true' ? this.collapseItem(el) : this.expandItem(el);
  }

  private focusPreviousItem() {
    this.setFocusedIndex(Math.max(0, this.focusedIndex - 1));
  }

  private focusNextItem() {
    this.setFocusedIndex(Math.min(this.visibleItems.length - 1, this.focusedIndex + 1));
  }

  private collapseItem(el: HTMLElement) {
    el.setAttribute('aria-expanded', 'false');
    this.updateVisibleItems();
  }

  private expandItem(el: HTMLElement) {
    el.setAttribute('aria-expanded', 'true');
    this.updateVisibleItems();
  }

  private focusParentItem(el: HTMLElement | null) {
    const owningItemEl = this.owningItem(el);
    if (owningItemEl) {
      this.setFocusedIndex(this.visibleItems.indexOf(owningItemEl));
    }
  }

  /**
   * @returnThe first parent item that “owns” the group that el is a member of,
   * or null if there is none.
   */
  owningItem(el: HTMLElement | null) {
    const groupEl = el?.closest(`[role='group']`);
    if (!groupEl) {
      return null;
    }
    return groupEl.parentElement?.querySelector<HTMLElement>(`[aria-owns='${groupEl.id}']`);
  }

  /**
   * Updates which items are visible (not a child of a collapsed item).
   */
  private updateVisibleItems() {
    const allEls = Array.from(this.el.querySelectorAll<HTMLElement>(`[role='treeitem']`));
    const hiddenEls = Array.from(
      this.el.querySelectorAll(`[aria-expanded='false'] + [role='group'] [role='treeitem']`)
    );
    this.visibleItems = allEls.filter(el => !hiddenEls.includes(el));
  }
}

/**
 * Primary controller for the documentation page, handling coordination between
 * the navigation and content components. This class ensures that any
 * documentation elements in view are properly shown/highlighted in the
 * navigation components.
 *
 * Since navigation is essentially handled by anchor tags with fragment IDs as
 * hrefs, the fragment ID (referenced in this code as simply “ID”) is used to
 * look up both navigation and content nodes.
 */
class DocPageController {
  private navController?: DocNavTreeController;
  private mobileNavController?: MobileNavController;
  /**
   * Instantiates the controller, setting up the navigation controller (both
   * desktop and mobile), and event listeners. This should only be called once.
   */
  constructor(
    sideNavEl: HTMLElement | null,
    mobileNavEl: HTMLElement | null,
    private contentEl: HTMLElement | null
  ) {
    if (!sideNavEl || !contentEl) {
      console.warn('Unable to find all elements needed for navigation');
      return;
    }

    this.navController = new DocNavTreeController(sideNavEl);

    if (mobileNavEl) {
      this.mobileNavController = new MobileNavController(mobileNavEl);
    }
    window.addEventListener('hashchange', () => this.handleHashChange());

    this.updateSelectedIdFromWindowHash();
  }

  /**
   * Handles when the location hash changes.
   */
  private handleHashChange() {
    this.updateSelectedIdFromWindowHash();
  }

  private updateSelectedIdFromWindowHash() {
    const targetId = this.targetIdFromLocationHash();
    this.navController?.setSelectedId(targetId);
    if (this.mobileNavController) {
      this.mobileNavController.setSelectedId(targetId);
    }
    if (targetId !== '') {
      const targetEl = this.contentEl?.querySelector<HTMLElement>(`[id='${targetId}']`);
      if (targetEl) {
        targetEl.focus();
      }
    }
  }

  targetIdFromLocationHash() {
    return window.location.hash && window.location.hash.substr(1);
  }
}

/**
 * Controller for the navigation element used on smaller viewports. It utilizes
 * a native <select> element for interactivity and a styled <label> for
 * displaying the selected option.
 *
 * It presumes a fixed header and that the container for the control will be
 * sticky right below the header when scrolled enough.
 */
class MobileNavController {
  private selectEl: HTMLSelectElement | null;
  private labelTextEl: HTMLElement | null;
  private intersectionObserver: IntersectionObserver;

  constructor(private el: HTMLElement) {
    this.selectEl = el.querySelector<HTMLSelectElement>('select');
    this.labelTextEl = el.querySelector<HTMLElement>('.js-mobileNavSelectText');

    this.selectEl?.addEventListener('change', e => this.handleSelectChange(e));

    // We use a slight hack to detect if the mobile nav container is pinned to
    // the bottom of the site header. The root viewport of an IntersectionObserver
    // is inset by the header height plus one pixel to ensure that the container is
    // considered “out of view” when in a fixed position and can be styled appropriately.
    const ROOT_TOP_MARGIN = '-57px';

    this.intersectionObserver = new IntersectionObserver(
      entries => this.intersectionObserverCallback(entries),
      {
        rootMargin: `${ROOT_TOP_MARGIN} 0px 0px 0px`,
        threshold: 1.0,
      }
    );
    this.intersectionObserver.observe(this.el);
  }

  setSelectedId(id: string) {
    if (!this.selectEl) return;
    this.selectEl.value = id;
    this.updateLabelText();
  }

  private updateLabelText() {
    if (!this.labelTextEl || !this.selectEl) return;
    const selectedIndex = this.selectEl?.selectedIndex;
    if (selectedIndex === -1) {
      this.labelTextEl.textContent = '';
      return;
    }
    this.labelTextEl.textContent = this.selectEl.options[selectedIndex].textContent;
  }

  private handleSelectChange(e: Event) {
    window.location.hash = `#${(e.target as HTMLSelectElement).value}`;
    this.updateLabelText();
  }

  private intersectionObserverCallback(entries: IntersectionObserverEntry[]) {
    const SHADOW_CSS_CLASS = 'DocNavMobile--withShadow';
    entries.forEach(entry => {
      // entry.isIntersecting isn’t reliable on Firefox.
      const fullyInView = entry.intersectionRatio === 1.0;
      entry.target.classList.toggle(SHADOW_CSS_CLASS, !fullyInView);
    });
  }
}

new DocPageController(
  document.querySelector('.js-tree'),
  document.querySelector('.js-mobileNav'),
  document.querySelector('.js-unitDetailsContent')
);
