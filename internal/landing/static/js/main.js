(function () {
  "use strict";

  var header = document.querySelector("[data-header]");
  var toggle = document.querySelector("[data-nav-toggle]");
  var nav = document.getElementById("primary-nav");

  function onScroll() {
    if (header) header.classList.toggle("is-scrolled", window.scrollY > 8);
  }

  onScroll();
  window.addEventListener("scroll", onScroll, { passive: true });

  function closeNav() {
    if (!toggle || !nav) return;
    toggle.setAttribute("aria-expanded", "false");
    nav.classList.remove("is-open");
    document.body.classList.remove("nav-open");
  }

  if (toggle && nav) {
    toggle.addEventListener("click", function () {
      var open = toggle.getAttribute("aria-expanded") === "true";
      toggle.setAttribute("aria-expanded", String(!open));
      nav.classList.toggle("is-open", !open);
      document.body.classList.toggle("nav-open", !open);
    });

    nav.addEventListener("click", function (event) {
      if (event.target.closest("a")) closeNav();
    });

    window.addEventListener("resize", function () {
      if (window.innerWidth > 880) closeNav();
    });
  }

  document.addEventListener("htmx:beforeRequest", function (event) {
    var el = event.detail && event.detail.elt;
    if (!el || !el.classList || !el.classList.contains("contact-form")) return;
    var button = el.querySelector('button[type="submit"]');
    if (button) button.disabled = true;
  });

  document.addEventListener("htmx:afterRequest", function (event) {
    var el = event.detail && event.detail.elt;
    if (!el || !el.classList || !el.classList.contains("contact-form")) return;
    var button = el.querySelector('button[type="submit"]');
    if (button) button.disabled = false;
  });

  function renderCaptchas(root) {
    if (!window.turnstile || !root || !root.querySelectorAll) return;
    root.querySelectorAll(".cf-turnstile").forEach(function (el) {
      if (el.getAttribute("data-rendered") === "1") return;
      el.setAttribute("data-rendered", "1");
      window.turnstile.render(el, { sitekey: el.getAttribute("data-sitekey") });
    });
  }

  document.addEventListener("htmx:afterSwap", function (event) {
    renderCaptchas(event.detail && event.detail.target);
  });

  document.addEventListener("htmx:afterSwap", function (event) {
    var target = event.target;
    if (!target || !target.querySelector) return;
    var focusable = target.querySelector("input, textarea, button");
    if (focusable && event.detail && event.detail.requestConfig && event.detail.requestConfig.verb === "post") {
      focusable.focus({ preventScroll: true });
    }
  });
})();
