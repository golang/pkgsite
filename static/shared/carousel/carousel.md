## Carousel

---

### With Heading

```html
<section class="go-Carousel js-carousel">
  <h5>Search Tips</h2>
  <ul>
    <li class="go-Carousel-slide">
      <p>Search for a package by name, for example “logrus”</p>
    </li>
    <li class="go-Carousel-slide" aria-hidden>
      <p>Search for a symbol by name, for example "sql.DB"</p>
    </li>
    <li class="go-Carousel-slide" aria-hidden>
      <p>Search for a symbol in a path, for example "#error golang.org/x"</p>
    </li>
  </ul>
</section>
```

### Aria Label

Use aria-label to create an accessible label that is visually hidden.

```html
<section class="go-Carousel js-carousel" aria-label="Search Tips Carousel">
  <ul>
    <li class="go-Carousel-slide">
      <p>Search for a package by name, for example “logrus”</p>
    </li>
    <li class="go-Carousel-slide" aria-hidden>
      <p>Search for a symbol by name, for example "sql.DB"</p>
    </li>
    <li class="go-Carousel-slide" aria-hidden>
      <p>Search for a symbol in a path, for example "#error golang.org/x"</p>
    </li>
  </ul>
</section>
```
