const assert = require("node:assert/strict");
const fs = require("node:fs");
const test = require("node:test");
const vm = require("node:vm");

test("scrolling the workspace updates the current vault link", () => {
  const listeners = {};
  const makeLink = (hash) => {
    const result = { hash, current: false };
    result.setAttribute = (_, value) => { result.current = value === "location"; };
    result.removeAttribute = () => { result.current = false; };
    return result;
  };
  const links = ["#readme", "#browse", "#commands"].map(makeLink);
  const sections = [
    { id: "readme", top: 0 },
    { id: "browse", top: 700 },
    { id: "commands", top: 1400 },
  ];
  sections.forEach((section) => {
    section.getBoundingClientRect = () => ({ top: section.top });
  });

  vm.runInNewContext(fs.readFileSync(`${__dirname}/site.js`, "utf8"), {
    clearInterval,
    document: {
      addEventListener: (name, handler) => { listeners[name] = handler; },
      querySelectorAll: (selector) => selector === "[data-scroll-link]" ? links
        : selector === "[data-scroll-section]" ? sections : [],
    },
    navigator: { clipboard: { writeText: async () => {} } },
    setInterval,
    setTimeout,
    window: { innerHeight: 1000, matchMedia: () => ({ matches: true }) },
  });

  assert.equal(links[0].current, true);
  sections[0].top = -900;
  sections[1].top = 100;
  sections[2].top = 800;
  listeners.scroll();
  assert.deepEqual(links.map(({ current }) => current), [false, true, false]);
});
