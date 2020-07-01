/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * Holds an Element with a corresponding index. Intended to allow for
 * operations that require quick access to DOM node ordering.
 * @property {!number} index
 * @property {!Element} element
 */
class IndexedElement {
  /**
   * @param {!number} index
   * @param {!Element} element
   */
  constructor(index, element) {
    this.index = index;
    this.element = element;
  }
}

/**
 * CSS classes used by DocNavTreeController.
 * @private @enum {string}
 */
const DocNavClass = {
  NODE: 'DocNav-node',
  NODE_SELECTED: 'DocNav-node--selected',
};

/**
 * The navigation tree component of the documentation page.
 */
class DocNavTreeController {
  /**
   * Instantiates and indexes all nodes in the navigation tree, calling the
   * given callback function on each node it indexes.
   * @param {!Element} el
   * @param {!function(string): void} callback
   */
  constructor(el, callback) {
    /** @private {!Element} */
    this._el = el;

    /** @private {Element} */
    this._selectedEl = null;

    /**
     * Map of content element IDs to an IndexedElement wrapping the corresponding
     * node in the navigation tree.
     * @private {!Object<string, IndexedElement>}
     */
    this._idToIndexedElement = {};

    this.indexNodes(callback);
  }

  /**
   * @param {!string} id
   * @return {IndexedElement|undefined} The indexed node with the given ID.
   */
  getIndexedElementById(id) {
    return this._idToIndexedElement[id];
  }

  /**
   * Selects the navigation node with the given ID. If no ID is provided, then
   * all nodes are unselected and nothing further is done.
   * @param {!string=} opt_id
   * @return {undefined}
   */
  setSelectedId(opt_id) {
    if (this._selectedEl) {
      this._selectedEl.classList.remove(DocNavClass.NODE_SELECTED);
      this._selectedEl = null;
    }
    if (!opt_id) {
      return;
    }
    const iEl = this.getIndexedElementById(opt_id);
    if (!iEl) {
      return;
    }
    this._selectedEl = iEl.element;
    if (!this._selectedEl) {
      return;
    }
    this._selectedEl.classList.add(DocNavClass.NODE_SELECTED);
  }

  /**
   * Queries the DOM, indexing each navigation node by its corresponding
   * target ID and invoking the given callback. This method is meant to be
   * called just once during class instantiation.
   * @param {!function(string): void} callback
   * @return {undefined}
   * @private
   */
  indexNodes(callback) {
    this._el.querySelectorAll(`a[href^='#']`).forEach((el, index) => {
      if (!el.hash) {
        return;
      }
      const nodeEl = el.closest(`.${DocNavClass.NODE}`);
      if (!nodeEl) {
        return;
      }
      const elId = el.hash.substr(1);
      this._idToIndexedElement[elId] = new IndexedElement(index, nodeEl);
      callback(elId);
    });
  }
}

/**
 * Primary controller for the documentation page, handling coordination between
 * the navigation and content components. This class ensures that any
 * documentation elements in view are properly shown/highlighted in the
 * navigation component.
 *
 * Since navigation is essentially handled by anchor tags with fragment IDs as
 * hrefs, the fragment ID (referenced in this code as simply “ID”) is used to
 * look up both navigation and content nodes.
 */
class DocPageController {
  /**
   * Instantiates the controller, setting up the navigation controller,
   * intersection observers, and event listeners. This should only be called
   * once.
   * @param {Element} navEl
   * @param {Element} contentEl
   */
  constructor(navEl, contentEl) {
    if (!navEl || !contentEl) {
      throw new Error('Must provide navigation and content elements.');
    }

    /* @private {!Element} */
    this._contentEl = contentEl;

    /**
     * A set of content node IDs currently in view. These should always be kept
     * in document order.
     * @private {!Array<string>}
     */
    this._nodeIdsInView = [];

    /**
     * The target element with an ID matching the current URL fragment.
     * @private {Element}
     */
    this._targetEl = null;

    // Since there is a sticky header at the top of the page, the root viewport
    // of the IntersectionObserver is inset to ensure that elements underneath
    // the header are not considered visible.
    const PRIMARY_ROOT_TOP_MARGIN = '-105px';

    /**
     * The IntersectionObserver that observes all content nodes other than the
     * target element.
     * @private {!IntersectionObserver}
     */
    this._primaryIntersectionObserver = new IntersectionObserver(
      (entries, observer) => this.intersectionObserverCallback(entries, observer),
      {
        rootMargin: `${PRIMARY_ROOT_TOP_MARGIN} 0px 0px 0px`,
        threshold: 1.0,
      }
    );

    // The target element is styled with a large upper rectangle to account for
    // the sticky header. In this case, the root margin is adjusted to behave
    // equally to the primary IntersectionObserver.
    const TARGET_ROOT_TOP_MARGIN = '17px';

    /**
     * The IntersectionObserver that observes only the target element. This is
     * required because the target element is intentionally styled with a much
     * larger bounding rectangle to position it below the sticky header. To
     * adjust to this, an additional IntersectionObserver is created with a
     * reduced top root margin.
     * @private {!IntersectionObserver}
     */
    this._targetIntersectionObserver = new IntersectionObserver(
      (entries, observer) => this.intersectionObserverCallback(entries, observer),
      {
        rootMargin: `${TARGET_ROOT_TOP_MARGIN} 0px 0px 0px`,
        threshold: 1.0,
      }
    );

    window.addEventListener('hashchange', e =>
      this.handleHashChange(/** @type {!HashChangeEvent} */ (e))
    );
    this._navController = new DocNavTreeController(navEl, id => this.handleNavTreeNodeIndex(id));
  }

  /**
   * Handles when the navigation tree indexes a node, indicating that this
   * component should keep track of the corresponding content node’s visibility.
   * @param {!string} id
   * @return {undefined}
   * @private
   */
  handleNavTreeNodeIndex(id) {
    const targetId = this.targetIdFromLocationHash();
    if (id === targetId) {
      this._targetEl = this._contentEl.querySelector(`[id='${id}']:target`);
      if (this._targetEl) {
        this._targetIntersectionObserver.observe(/** @type {!Element} */ (this._targetEl));
      }
      return;
    }
    const el = this._contentEl.querySelector(`[id='${id}']:not(:target)`);
    if (el) {
      this._primaryIntersectionObserver.observe(/** @type {!Element} */ (el));
    }
  }

  /**
   * Updates which content nodes are in view and ensures the topmost content
   * node fully in view is selected in the navigation component.
   * @param {!Array<IntersectionObserverEntry>} entries
   * @param {!IntersectionObserver} observer
   */
  intersectionObserverCallback(entries, observer) {
    entries.forEach(entry => {
      // TODO: remove this check once this is no longer an experiment and
      // hidden elements are not observed as a result.
      if (entry.boundingClientRect.width === 0 || entry.boundingClientRect.height === 0) {
        return;
      }

      let /** number */ idx = -1;
      for (const [i, nodeId] of this._nodeIdsInView.entries()) {
        if (nodeId === entry.target.id) {
          idx = /** @type {number} */ (i);
          break;
        }
      }

      // entry.isIntersecting isn’t reliable on Firefox.
      const fullyInView = entry.intersectionRatio === 1.0;
      if (idx !== -1 && !fullyInView) {
        this._nodeIdsInView.splice(idx, 1);
      } else if (idx === -1 && fullyInView) {
        this._nodeIdsInView.push(entry.target.id);
      }
    });

    this._nodeIdsInView = this._nodeIdsInView.sort((n1, n2) => {
      return (
        this._navController.getIndexedElementById(n1).index -
        this._navController.getIndexedElementById(n2).index
      );
    });
    this._navController.setSelectedId(this._nodeIdsInView[0]);
  }

  /**
   * Handles when the location hash changes, indicating that the target element
   * must be updated.
   * @param {!HashChangeEvent} e
   * @private
   */
  handleHashChange(e) {
    if (this._targetEl) {
      this._targetIntersectionObserver.unobserve(/** @type {!Element} */ (this._targetEl));
      this._primaryIntersectionObserver.observe(/** @type {!Element} */ (this._targetEl));
    }

    const targetId = this.targetIdFromLocationHash();
    if (!targetId) {
      return;
    }
    const targetEl = this._contentEl.querySelector(`[id='${targetId}']:target`);
    if (targetEl) {
      this._targetEl = targetEl;
      this._primaryIntersectionObserver.unobserve(/** @type {!Element} */ (this._targetEl));
      this._targetIntersectionObserver.observe(/** @type {!Element} */ (this._targetEl));
    }
  }

  /**
   * @return {!string}
   */
  targetIdFromLocationHash() {
    return window.location.hash && window.location.hash.substr(1);
  }
}

new DocPageController(
  document.querySelector('.js-sideNav'),
  document.querySelector('.js-docContent')
);
