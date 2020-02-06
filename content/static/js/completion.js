// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

document.addEventListener('DOMContentLoaded', function() {
  // To implement autocomplete we use autoComplete.js, but override the
  // navigation controller to be more accessible.
  //
  // Accessibility requirements are based on:
  // https://www.w3.org/TR/wai-aria-practices/examples/combobox/aria1.1pattern/listbox-combo.html
  // This defines the interaction of three elements: the 'combobox', which is
  // the parent of both the text input, and the completion list.
  //
  // The autoComplete.js library assumes a single element in its query
  // selector, unfortunately, so we can't express our autocompletion behavior
  // in terms of classes rather than IDs. See also
  // https://github.com/TarekRaafat/autoComplete.js/issues/82
  const completeInput = document.querySelector('#AutoComplete');
  const parentForm = document.querySelector('#AutoComplete-parent');
  const hideCompletion = () => {
    parentForm.setAttribute('aria-expanded', false);
    // Without removing aria-activedescendant screenreaders will get confused.
    completeInput.removeAttribute('aria-activedescendant');
  };
  const showCompletion = () => {
    parentForm.setAttribute('aria-expanded', true);
  };
  // Accessibility: hide and show the completion dialog on blur and focus.
  completeInput.addEventListener('blur', hideCompletion);
  completeInput.addEventListener('focus', showCompletion);

  /**
   * See https://tarekraafat.github.io/autoComplete.js/#/?id=api-configuration
   * for some documentation of the autoComplete.js API. In particular, it
   * allows overriding the resultsList navigation controller.
   *
   * This controller is based on the default implementation here:
   * https://github.com/TarekRaafat/autoComplete.js/blob/v7.2.0/src/views/autoCompleteView.js#L120
   *
   * The primary changes are:
   * + set aria-expanded, aria-selected, and aria-activedescendant attributes
   *   where necessary
   * + use aria attributes for styling, rather than classes
   * + add id attributes to the list elements
   * + hide completion on blur, or escape
   * + simplify the code a bit
   *
   * This function is evaluated each time the result list is assembled (i.e.
   * potentially each keypress).
   *
   * @param input is the text input element.
   * @param resultsList is the ul element being constructed.
   * @param sendFeedback is the data feedback function invoked on user
   *   selection. "feedback" is the term used for this in autoComplete.js, so
   *   we preserve this for consistency.
   * @param resultsValues is the processed data values from the data source.
   */
  const navigation = (evt, input, resultsList, sendFeedback, resultsValues) => {
    const lis = Array.from(resultsList.childNodes);
    let liSelected = undefined;
    const highlightSelection = next => {
      // Clear the existing selected item.
      if (liSelected) {
        liSelected.removeAttribute('aria-selected');
      }
      liSelected = next;
      liSelected.setAttribute('aria-selected', 'true');
      const idx = lis.findIndex(li => li === liSelected);
      input.setAttribute('aria-activedescendant', 'AutoComplete-item-' + idx);
    };
    const onSelection = (event, elem) => {
      sendFeedback({
        event: event,
        query: input.value,
        matches: resultsValues.matches,
        results: resultsValues.list.map(record => record.value),
        selection: resultsValues.list.find(
          value => value.index === Number(elem.getAttribute('data-id'))
        )
      });
      hideCompletion();
    };
    input.onkeydown = event => {
      const keys = {
        ENTER: 13,
        ESCAPE: 27,
        ARROW_UP: 38,
        ARROW_DOWN: 40
      };
      let next = undefined; // the next item to highlight
      if (lis.length > 0) {
        switch (event.keyCode) {
          case keys.ARROW_UP:
            showCompletion();
            // Show the last completion item if none are currently selected, or
            // we're at the start of the list.
            next = lis[lis.length - 1];
            if (liSelected && liSelected.previousSibling) {
              next = liSelected.previousSibling;
            }
            highlightSelection(next);
            // If we don't preventDefault here, up and down arrows cause us to
            // also jump to the start or end of the text input.
            event.preventDefault();
            break;
          case keys.ARROW_DOWN:
            showCompletion();
            // Show the first completion item if none are currently selected,
            // or if we're at the end of the list.
            next = lis[0];
            if (liSelected && liSelected.nextSibling) {
              next = liSelected.nextSibling;
            }
            highlightSelection(next);
            // See note above for why this is necessary.
            event.preventDefault();
            break;
          case keys.ENTER:
            if (liSelected) {
              event.preventDefault();
              onSelection(event, liSelected);
            }
            break;
          case keys.ESCAPE:
            // From the aria guide, escape should both hide completion and
            // clear the input. Arguably it might be better to leave the input
            // untouched, which is what Google search does.
            hideCompletion();
            input.value = '';
            break;
          default:
            // Because we've hidden the completion ourselves on escape, we need
            // to show it on any other character.
            showCompletion();
        }
      }
    };
    lis.forEach((selection, i) => {
      // Completion items need id attributes so that they can be referenced by
      // aria-activedescendant.
      selection.setAttribute('id', 'AutoComplete-item-' + i);
      selection.onmousedown = event => {
        onSelection(event, event.currentTarget);
        event.preventDefault();
      };
    });
  };

  new autoComplete({
    data: {
      src: async () => {
        const query = completeInput.value;
        const source = await fetch(`/autocomplete?q=${query}`);
        return await source.json();
      },
      // The string we're completing is stored in the 'PackagePath' field of
      // the returned JSON array elements.
      key: ['PackagePath'],
      cache: false
    },
    threshold: 1, // minimum number of characters before rendering results
    debounce: 100, // in milliseconds
    resultsList: {
      render: true,
      container: source => {
        source.setAttribute('id', 'AutoComplete-list');
        source.classList.add('AutoComplete-list');
        source.setAttribute('role', 'listbox');
      },
      destination: document.querySelector('#AutoComplete-parent'),
      position: 'beforeend',
      element: 'ul',
      navigation: navigation
    },
    highlight: true,
    selector: '#AutoComplete',
    onSelection: feedback => {
      if (feedback.selection.value.PackagePath) {
        // Navigate directly to the package.
        // TODO (b/149016238): update ARIA attributes to reflect this.
        window.location.href = "/" + feedback.selection.value.PackagePath;
      }
    }
  });
});
