/*! PagePilot Reveal highlight adapter. Classic browser script. */
(function (global) {
  "use strict";

  function each(list, callback) {
    Array.prototype.forEach.call(list || [], callback);
  }

  function getRevealRoot(reveal) {
    if (reveal && typeof reveal.getRevealElement === "function") {
      return reveal.getRevealElement();
    }
    return document;
  }

  function prepareBlock(block) {
    if (!block) {
      return;
    }

    block.classList.add("hljs");

    var parent = block.parentNode;
    if (parent && parent.classList) {
      parent.classList.add("code-wrapper");
    }

    var template = block.querySelector('script[type="text/template"]');
    if (template) {
      block.textContent = template.innerHTML;
    }

    var hljs = global.hljs;
    if (hljs && typeof hljs.highlightElement === "function") {
      hljs.highlightElement(block);
    }
  }

  var RevealHighlight = {
    id: "highlight",

    init: function (reveal) {
      var root = getRevealRoot(reveal);
      each(root.querySelectorAll("pre code"), prepareBlock);

      if (reveal && typeof reveal.on === "function") {
        reveal.on("slidechanged", function (event) {
          if (!event || !event.currentSlide) {
            return;
          }
          each(event.currentSlide.querySelectorAll("pre code"), prepareBlock);
        });
      }
    }
  };

  global.RevealHighlight = RevealHighlight;
})(typeof window !== "undefined" ? window : globalThis);
