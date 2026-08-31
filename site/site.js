document.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-copy]");
  if (!button) return;

  await navigator.clipboard.writeText(button.dataset.copy);
  button.textContent = "Copied";
  button.setAttribute("aria-label", "Command copied");
  setTimeout(() => {
    button.textContent = "Copy";
    button.setAttribute("aria-label", "Copy command");
  }, 1500);
});

document.querySelectorAll("[data-slideshow]").forEach((slideshow) => {
  const slides = [...slideshow.querySelectorAll("[data-slide]")];
  const captions = [...slideshow.querySelectorAll("[data-caption]")];
  const controls = [...slideshow.querySelectorAll("[data-slide-to]")];
  const toggle = slideshow.querySelector("[data-slide-toggle]");
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  let active = 0;
  let paused = reducedMotion;
  let timer;

  const show = (index) => {
    active = index;
    slides.forEach((slide, position) => {
      const current = position === active;
      slide.classList.toggle("is-active", current);
      slide.setAttribute("aria-hidden", String(!current));
      captions[position].classList.toggle("is-active", current);
      captions[position].setAttribute("aria-hidden", String(!current));
      controls[position].setAttribute("aria-pressed", String(current));
    });
  };

  const stop = () => clearInterval(timer);
  const start = () => {
    stop();
    if (!paused) timer = setInterval(() => show((active + 1) % slides.length), 6000);
  };

  controls.forEach((control) => control.addEventListener("click", () => {
    stop();
    show(Number(control.dataset.slideTo));
    start();
  }));
  toggle.addEventListener("click", () => {
    paused = !paused;
    toggle.textContent = paused ? "Play" : "Pause";
    toggle.setAttribute("aria-label", paused ? "Play slideshow" : "Pause slideshow");
    toggle.setAttribute("aria-pressed", String(paused));
    start();
  });
  if (reducedMotion) {
    toggle.textContent = "Play";
    toggle.setAttribute("aria-label", "Play slideshow");
    toggle.setAttribute("aria-pressed", "true");
  }
  start();
});

const sectionLinks = [...document.querySelectorAll("[data-scroll-link]")];
const sections = [...document.querySelectorAll("[data-scroll-section]")];

if (sectionLinks.length && sections.length) {
  const updateCurrentSection = () => {
    const threshold = window.innerHeight * 0.35;
    const current = sections.reduce((active, section) =>
      section.getBoundingClientRect().top <= threshold ? section : active, sections[0]);

    sectionLinks.forEach((link) => {
      if (link.hash === `#${current.id}`) link.setAttribute("aria-current", "location");
      else link.removeAttribute("aria-current");
    });
  };

  document.addEventListener("scroll", updateCurrentSection, { passive: true });
  updateCurrentSection();
}
