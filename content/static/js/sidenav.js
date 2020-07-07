/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
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
   * Instantiates a navigation tree.
   * @param {!Element} el
   */
  constructor(el) {
    /** @private {!Element} */
    this._el = el;

    /**
     * The currently selected element.
     * @private {Element}
     */
    this._selectedEl = null;

    /**
     * The index of the currently focused item. Used when navigating the tree
     * using the keyboard.
     * @private {number}
     */
    this._focusedIndex = 0;

    /**
     * The elements currently visible (not within a collapsed node of the tree).
     * @private {!Array<!Element>}
     */
    this._visibleItems = [];

    /**
     * The current search string.
     * @private {string}
     */
    this._searchString = '';

    /**
     * The timestamp of the last keydown event. Used to track whether to use the
     * current search string.
     * @private {number}
     */
    this._lastKeyDownTimeStamp = -Infinity;

    this.addEventListeners();
    this.updateVisibleItems();
    this.initialize();
  }

  /**
   * Initializes the tree. Should be called only once.
   * @private
   */
  initialize() {
    this._el.querySelectorAll(`[role='treeitem']`).forEach((el, i) => {
      el.addEventListener('click', e => this.handleItemClick(/** @type {!MouseEvent} */ (e)));
    });

    // TODO: remove once safehtml supports aria-owns with dynamic values.
    this._el.querySelectorAll('[data-aria-owns]').forEach(el => {
      el.setAttribute('aria-owns', el.getAttribute('data-aria-owns'));
    });
  }

  /**
   * @private
   */
  addEventListeners() {
    this._el.addEventListener('keydown', e =>
      this.handleKeyDown(/** @type {!KeyboardEvent} */ (e))
    );
  }

  /**
   * Sets the visible item with the given index with the proper tabindex and
   * focuses it.
   * @param {!number} index
   * @return {undefined}
   */
  setFocusedIndex(index) {
    if (index === this._focusedIndex) {
      return;
    }

    let itemEl = this._visibleItems[this._focusedIndex];
    itemEl.setAttribute('tabindex', '-1');

    itemEl = this._visibleItems[index];
    itemEl.setAttribute('tabindex', '0');
    itemEl.focus();

    this._focusedIndex = index;
  }

  /**
   * Marks the navigation node with the given ID as selected. If no ID is
   * provided, the first visible item in the tree is used.
   * @param {!string=} opt_id
   * @return {undefined}
   */
  setSelectedId(opt_id) {
    if (this._selectedEl) {
      this._selectedEl.removeAttribute('aria-selected');
      this._selectedEl = null;
    }
    if (opt_id) {
      this._selectedEl = this._el.querySelector(`[role='treeitem'][href='#${opt_id}']`);
    } else if (this._visibleItems.length > 0) {
      this._selectedEl = this._visibleItems[0];
    }

    if (!this._selectedEl) {
      return;
    }
    this._selectedEl.setAttribute('aria-selected', 'true');
    this.expandAllParents(this._selectedEl);
    this.scrollElementIntoView(this._selectedEl);
  }

  /**
   * Expands all sibling items of the given element.
   * @param {!Element} el
   * @private
   */
  expandAllSiblingItems(el) {
    const level = el.getAttribute('aria-level');
    this._el.querySelectorAll(`[aria-level='${level}'][aria-expanded='false']`).forEach(el => {
      el.setAttribute('aria-expanded', 'true');
    });
    this.updateVisibleItems();
    this._focusedIndex = this._visibleItems.indexOf(el);
  }

  /**
   * Expands all parent items of the given element.
   * @param {!Element} el
   * @private
   */
  expandAllParents(el) {
    if (!this._visibleItems.includes(el)) {
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
   * @param {!Element} el
   * @private
   */
  scrollElementIntoView(el) {
    const STICKY_HEADER_HEIGHT_PX = 105;
    const viewportHeightPx = document.documentElement.clientHeight;
    const elRect = el.getBoundingClientRect();
    const verticalCenterPointPx = (viewportHeightPx - STICKY_HEADER_HEIGHT_PX) / 2;
    if (elRect.top < STICKY_HEADER_HEIGHT_PX) {
      // Element is occluded at top of view by header or by being offscreen.
      this._el.scrollTop -=
        STICKY_HEADER_HEIGHT_PX - elRect.top - elRect.height + verticalCenterPointPx;
    } else if (elRect.bottom > viewportHeightPx) {
      // Element is below viewport.
      this._el.scrollTop = elRect.bottom - viewportHeightPx + verticalCenterPointPx;
    } else {
      return;
    }
  }

  /**
   * Handles when a tree item is clicked.
   * @param {!MouseEvent} e
   * @private
   */
  handleItemClick(e) {
    const el = /** @type {!Element} */ (e.target);
    this.setFocusedIndex(this._visibleItems.indexOf(el));
    if (el.hasAttribute('aria-expanded')) {
      this.toggleItemExpandedState(el);
    }
  }

  /**
   * Handles when a key is pressed when the component is in focus.
   * @param {!KeyboardEvent} e
   * @private
   */
  handleKeyDown(e) {
    const targetEl = /** @type {!Element} */ (e.target);

    switch (e.key) {
      case Key.ASTERISK:
        this.expandAllSiblingItems(targetEl);
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
        if (e.target.getAttribute('aria-expanded') === 'true') {
          this.collapseItem(targetEl);
        } else {
          this.focusParentItem(targetEl);
        }
        break;

      case Key.RIGHT: {
        switch (targetEl.getAttribute('aria-expanded')) {
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
        this.setFocusedIndex(this._visibleItems.length - 1);
        break;

      case Key.ENTER:
        if (targetEl.tagName === 'A') {
          // Enter triggers desired behavior by itself.
          return;
        }
      // Fall through for non-anchor items to be handled the same as when
      // the space key is pressed.
      case Key.SPACE:
        targetEl.click();
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
   * @param {!KeyboardEvent} e
   * @private
   */
  handleSearch(e) {
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
    if (e.timeStamp - this._lastKeyDownTimeStamp > MAX_TYPEAHEAD_THRESHOLD_MS) {
      this._searchString = '';
    }
    this._lastKeyDownTimeStamp = e.timeStamp;
    this._searchString += e.key.toLocaleLowerCase();
    const focusedElementText = this._visibleItems[
      this._focusedIndex
    ].textContent.toLocaleLowerCase();
    if (this._searchString.length === 1 || !focusedElementText.startsWith(this._searchString)) {
      this.focusNextItemWithPrefix(this._searchString);
    }
    e.stopPropagation();
    e.preventDefault();
  }

  /**
   * Focuses on the next visible tree item (after the currently focused element,
   * wrapping the tree) that has a prefix equal to the given search string.
   * @param {string} prefix
   */
  focusNextItemWithPrefix(prefix) {
    let i = this._focusedIndex + 1;
    if (i > this._visibleItems.length - 1) {
      i = 0;
    }
    while (i !== this._focusedIndex) {
      if (this._visibleItems[i].textContent.toLocaleLowerCase().startsWith(prefix)) {
        this.setFocusedIndex(i);
        return;
      }
      if (i >= this._visibleItems.length - 1) {
        i = 0;
      } else {
        i++;
      }
    }
  }

  /**
   * @param {!Element} el
   * @private
   */
  toggleItemExpandedState(el) {
    el.getAttribute('aria-expanded') === 'true' ? this.collapseItem(el) : this.expandItem(el);
  }

  /**
   * @private
   */
  focusPreviousItem() {
    this.setFocusedIndex(Math.max(0, this._focusedIndex - 1));
  }

  /**
   * @private
   */
  focusNextItem() {
    this.setFocusedIndex(Math.min(this._visibleItems.length - 1, this._focusedIndex + 1));
  }

  /**
   * @param {!Element} el
   * @private
   */
  collapseItem(el) {
    el.setAttribute('aria-expanded', 'false');
    this.updateVisibleItems();
  }

  /**
   * @param {!Element} el
   * @private
   */
  expandItem(el) {
    el.setAttribute('aria-expanded', 'true');
    this.updateVisibleItems();
  }

  /**
   * @param {!Element} el
   * @private
   */
  focusParentItem(el) {
    const owningItemEl = this.owningItem(el);
    if (owningItemEl) {
      this.setFocusedIndex(this._visibleItems.indexOf(owningItemEl));
    }
  }

  /**
   * @param {!Element} el
   * @return {Element} The first parent item that “owns” the group that el is a member of,
   * or null if there is none.
   */
  owningItem(el) {
    const groupEl = el.closest(`[role='group']`);
    if (!groupEl) {
      return null;
    }
    return groupEl.parentElement.querySelector(`[aria-owns='${groupEl.id}']`);
  }

  /**
   * Updates which items are visible (not a child of a collapsed item).
   * @private
   */
  updateVisibleItems() {
    const allEls = Array.from(this._el.querySelectorAll(`[role='treeitem']`));
    const hiddenEls = Array.from(
      this._el.querySelectorAll(`[aria-expanded='false'] + [role='group'] [role='treeitem']`)
    );
    this._visibleItems = allEls.filter(el => !hiddenEls.includes(el));
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
  /**
   * Instantiates the controller, setting up the navigation controller (both
   * desktop and mobile), and event listeners. This should only be called once.
   * @param {Element} sideNavEl
   * @param {Element} mobileNavEl
   * @param {Element} contentEl
   */
  constructor(sideNavEl, mobileNavEl, contentEl) {
    if (!sideNavEl || !mobileNavEl || !contentEl) {
      console.warn('Unable to find all elements needed for navigation');
      return;
    }

    /**
     * @type {!Element}
     * @private
     */
    this._contentEl = contentEl;

    window.addEventListener('hashchange', e =>
      this.handleHashChange(/** @type {!HashChangeEvent} */ (e))
    );

    /**
     * @type {!DocNavTreeController}
     * @private
     */
    this._navController = new DocNavTreeController(sideNavEl);

    /**
     * @type {!MobileNavController}
     * @private
     */
    this._mobileNavController = new MobileNavController(mobileNavEl);

    this.updateSelectedIdFromWindowHash();
  }

  /**
   * Handles when the location hash changes.
   * @param {!HashChangeEvent} e
   * @private
   */
  handleHashChange(e) {
    this.updateSelectedIdFromWindowHash();
  }

  /**
   * @private
   */
  updateSelectedIdFromWindowHash() {
    const targetId = this.targetIdFromLocationHash();
    this._navController.setSelectedId(targetId);
    this._mobileNavController.setSelectedId(targetId);
    if (targetId !== '') {
      const targetEl = this._contentEl.querySelector(`[id='${targetId}']`);
      targetEl.focus();
    }
  }

  /**
   * @return {!string}
   */
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
  /**
   * @param {!Element} el
   */
  constructor(el) {
    /**
     * @type {!Element}
     * @private
     */
    this._el = /** @type {!Element} */ (el);

    /**
     * @type {!HTMLSelectElement}
     * @private
     */
    this._selectEl = /** @type {!HTMLSelectElement} */ (el.querySelector('select'));

    /**
     * @type {!Element}
     * @private
     */
    this._labelTextEl = /** @type {!Element} */ (el.querySelector('.js-mobileNavSelectText'));

    this._selectEl.addEventListener('change', e =>
      this.handleSelectChange(/** @type {!Event} */ (e))
    );

    // We use a slight hack to detect if the mobile nav container is pinned to
    // the bottom of the site header. The root viewport of an IntersectionObserver
    // is inset by the header height plus one pixel to ensure that the container is
    // considered “out of view” when in a fixed position and can be styled appropriately.
    const ROOT_TOP_MARGIN = '-65px';

    this._intersectionObserver = new IntersectionObserver(
      (entries, observer) => this.intersectionObserverCallback(entries, observer),
      {
        rootMargin: `${ROOT_TOP_MARGIN} 0px 0px 0px`,
        threshold: 1.0,
      }
    );
    this._intersectionObserver.observe(this._el);
  }

  /**
   * @param {string} id
   */
  setSelectedId(id) {
    this._selectEl.value = id;
    this.updateLabelText();
  }

  /**
   * @private
   */
  updateLabelText() {
    const selectedIndex = this._selectEl.selectedIndex;
    if (selectedIndex === -1) {
      this._labelTextEl.textContent = '';
      return;
    }
    this._labelTextEl.textContent = this._selectEl.options[selectedIndex].textContent;
  }

  /**
   * @param {!Event} e
   * @private
   */
  handleSelectChange(e) {
    window.location.hash = `#${e.target.value}`;
    this.updateLabelText();
  }

  /**
   * @param {!Array<IntersectionObserverEntry>} entries
   * @param {!IntersectionObserver} observer
   * @private
   */
  intersectionObserverCallback(entries, observer) {
    const SHADOW_CSS_CLASS = 'DocNavMobile--withShadow';
    entries.forEach(entry => {
      // entry.isIntersecting isn’t reliable on Firefox.
      const fullyInView = entry.intersectionRatio === 1.0;
      entry.target.classList.toggle(SHADOW_CSS_CLASS, !fullyInView);
    });
  }
}

new DocPageController(
  document.querySelector('.js-sideNav'),
  document.querySelector('.js-mobileNav'),
  document.querySelector('.js-docContent')
);
