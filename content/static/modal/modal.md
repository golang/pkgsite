## Modals

---

The size modifer class is optional. The base modal will grow to fit the inner content.

### Small {#modal-small}

```html
<dialog id="example-modal-id1" class="go-Modal go-Modal--sm js-modal">
  <form action="#modals">
    <div class="go-Modal-header">
      <h2>Small Modal</h2>
      <button
        class="go-Button go-Button--inline"
        type="button"
        data-modal-close
        data-gtmc="modal button"
        aria-label="Close"
      >
        <img
          class="go-Icon"
          height="24"
          width="24"
          src="/static/icon/close_gm_grey_24dp.svg"
          alt=""
        />
      </button>
    </div>
    <div class="go-Modal-body">
      <p>Hello, world!</p>
    </div>
    <div class="go-Modal-actions">
      <button class="go-Button" data-modal-close data-gtmc="modal button">Submit</button>
    </div>
  </form>
</dialog>
<button class="go-Button" aria-controls="example-modal-id1" data-gtmc="modal button">Open</button>
```

### Medium {#modal-medium}

```html
<dialog id="example-modal-id2" class="go-Modal go-Modal--md js-modal">
  <form action="#modals">
    <div class="go-Modal-header">
      <h2>Medium Modal</h2>
      <button
        class="go-Button go-Button--inline"
        type="button"
        data-modal-close
        data-gtmc="modal button"
        aria-label="Close"
      >
        <img
          class="go-Icon"
          height="24"
          width="24"
          src="/static/icon/close_gm_grey_24dp.svg"
          alt=""
        />
      </button>
    </div>
    <div class="go-Modal-body">
      <p>Hello, world!</p>
    </div>
    <div class="go-Modal-actions">
      <button class="go-Button" data-modal-close data-gtmc="modal button">Submit</button>
    </div>
  </form>
</dialog>
<button class="go-Button" aria-controls="example-modal-id2">Open</button>
```

### Large {#modal-large}

```html
<dialog id="example-modal-id3" class="go-Modal go-Modal--lg js-modal">
  <form action="#modals">
    <div class="go-Modal-header">
      <h2>Large Modal</h2>
      <button
        class="go-Button go-Button--inline"
        type="button"
        data-modal-close
        data-gtmc="modal button"
        aria-label="Close"
      >
        <img
          class="go-Icon"
          height="24"
          width="24"
          src="/static/icon/close_gm_grey_24dp.svg"
          alt=""
        />
      </button>
    </div>
    <div class="go-Modal-body">
      <p>Hello, world!</p>
    </div>
    <div class="go-Modal-actions">
      <button class="go-Button" data-modal-close data-gtmc="modal button">Submit</button>
    </div>
  </form>
</dialog>
<button class="go-Button" aria-controls="example-modal-id3" data-gtmc="modal button">Open</button>
```
