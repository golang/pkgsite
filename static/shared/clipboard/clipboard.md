## Clipboard

---

Use an aria-label on buttons to provide context about the data being copied.

### Button {#clipboard-button}

```html
<button
  class="go-Button go-Button--inline go-Clipboard js-clipboard"
  data-to-copy="hello, world!"
  aria-label="Copy _DATA_ to Clipboard"
  data-gtmc="clipboard button"
>
  <img
    class="go-Icon"
    height="24"
    width="24"
    src="/static/shared/icon/content_copy_gm_grey_24dp.svg"
    alt=""
  />
</button>
```

```html
<button
  class="go-Button go-Button--inverted go-Clipboard js-clipboard"
  data-to-copy="hello, world!"
  aria-label="Copy _DATA_ to Clipboard"
  data-gtmc="clipboard button"
>
  <img
    class="go-Icon"
    height="24"
    width="24"
    src="/static/shared/icon/content_copy_gm_grey_24dp.svg"
    alt=""
  />
</button>
```

### Input {#clipboard-input}

```html
<div class="go-InputGroup">
  <input class="go-Input" value="hello, world!" readonly />
  <button
    class="go-Button go-Button--inverted go-Clipboard js-clipboard"
    aria-label="Copy to Clipboard"
    data-gtmc="clipboard button"
  >
    <img
      class="go-Icon"
      height="24"
      width="24"
      src="/static/shared/icon/content_copy_gm_grey_24dp.svg"
      alt=""
    />
  </button>
</div>
```
